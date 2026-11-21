package mcp

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/tools"
)

// Тестовый бинарник сам становится stdio MCP-сервером, если его так запустили:
// менеджер подключается к нему как к любому внешнему серверу.
func TestMain(m *testing.M) {
	if os.Getenv("ALLAN_MCP_TEST_SERVER") == "1" {
		runTestServer()
		return
	}
	os.Exit(m.Run())
}

type echoIn struct {
	Text string `json:"text" jsonschema:"что вернуть"`
}

func runTestServer() {
	s := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	sdk.AddTool(s, &sdk.Tool{Name: "echo", Description: "Возвращает текст"},
		func(ctx context.Context, req *sdk.CallToolRequest, in echoIn) (*sdk.CallToolResult, any, error) {
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "эхо: " + in.Text}}}, nil, nil
		})
	sdk.AddTool(s, &sdk.Tool{Name: "fail", Description: "Всегда ошибка"},
		func(ctx context.Context, req *sdk.CallToolRequest, in echoIn) (*sdk.CallToolResult, any, error) {
			return nil, nil, errors.New("сломалось")
		})
	_ = s.Run(context.Background(), &sdk.StdioTransport{})
}

type sink struct {
	mu sync.Mutex
	r  *tools.Registry
}

func (s *sink) RegisterWith(t tools.Tool, p tools.LoadPolicy) {
	s.mu.Lock()
	s.r.RegisterWith(t, p)
	s.mu.Unlock()
}

func TestStdioServerToolsReachAgent(t *testing.T) {
	cfg := &config.Config{MCP: map[string]config.MCPServer{
		"demo":  {Command: os.Args[0], Env: map[string]string{"ALLAN_MCP_TEST_SERVER": "1"}},
		"only":  {Command: os.Args[0], Env: map[string]string{"ALLAN_MCP_TEST_SERVER": "1"}, Tools: []string{"echo"}},
		"off":   {Command: os.Args[0], Disabled: true},
		"broke": {Command: "/nonexistent/allan-mcp-server"},
	}}
	m := newManager(cfg)
	defer m.Close()
	reg := tools.NewRegistry()
	m.Attach(&sink{r: reg})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	m.Wait(ctx)

	states := map[string]ServerStatus{}
	for _, st := range m.Status() {
		states[st.Name] = st
	}
	if states["demo"].State != StateReady || len(states["demo"].Tools) != 2 {
		t.Fatalf("demo: %+v", states["demo"])
	}
	if len(states["only"].Tools) != 1 {
		t.Fatalf("allow list not applied: %+v", states["only"])
	}
	if states["off"].State != StateDisabled {
		t.Fatalf("off: %+v", states["off"])
	}
	if states["broke"].State != StateFailed || states["broke"].Error == "" {
		t.Fatalf("broke: %+v", states["broke"])
	}

	out, err := reg.Run(ctx, "mcp__demo__echo", map[string]any{"text": "привет"})
	if err != nil || out != "эхо: привет" {
		t.Fatalf("echo: %q %v", out, err)
	}
	if _, err := reg.Run(ctx, "mcp__demo__fail", map[string]any{"text": "x"}); err == nil || !strings.Contains(err.Error(), "сломалось") {
		t.Fatalf("fail should return the server error, got %v", err)
	}
	if _, ok := reg.Get("mcp__only__fail"); ok {
		t.Fatal("tool outside the allow list must not be registered")
	}
	var def map[string]any
	for _, d := range reg.ToolDefs() {
		if d.Name == "mcp__demo__echo" {
			def = d.Parameters
		}
	}
	if def["type"] != "object" {
		t.Fatalf("schema: %v", def)
	}
}

func TestToolNameIsProviderSafe(t *testing.T) {
	if got := ToolName("my server", "do.thing"); got != "mcp__my_server__do_thing" {
		t.Fatal(got)
	}
	long := ToolName(strings.Repeat("s", 40), strings.Repeat("t", 40))
	if len(long) > 64 {
		t.Fatalf("too long: %d", len(long))
	}
	if long == ToolName(strings.Repeat("s", 40), strings.Repeat("t", 41)) {
		t.Fatal("truncated names must stay distinct")
	}
}

func (s *sink) Unregister(names ...string) {
	s.mu.Lock()
	s.r.Unregister(names...)
	s.mu.Unlock()
}

// Сервер выключают и включают на лету: инструменты уходят из реестра и
// возвращаются, а соседние серверы не переподключаются.
func TestSetTogglesOneServer(t *testing.T) {
	srv := config.MCPServer{Command: os.Args[0], Env: map[string]string{"ALLAN_MCP_TEST_SERVER": "1"}}
	cfg := &config.Config{MCP: map[string]config.MCPServer{"a": srv, "b": srv}}
	m := newManager(cfg)
	defer m.Close()
	reg := tools.NewRegistry()
	m.Attach(&sink{r: reg})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	m.Wait(ctx)
	sessionB := m.servers["b"].session

	off := srv
	off.Disabled = true
	m.Set("a", off)
	if _, ok := reg.Get("mcp__a__echo"); ok {
		t.Fatal("tools of a disabled server must leave the registry")
	}
	m.Set("a", srv)
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, ok := reg.Get("mcp__a__echo"); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("a did not come back: %+v", m.Status())
		}
		time.Sleep(50 * time.Millisecond)
	}
	if m.servers["b"].session != sessionB {
		t.Fatal("b must keep its connection")
	}
	if _, err := reg.Run(ctx, "mcp__a__echo", map[string]any{"text": "снова"}); err != nil {
		t.Fatalf("echo after re-enable: %v", err)
	}
}
