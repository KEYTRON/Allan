package bootstrap

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/agent"
	"github.com/keytron/allan/agent/internal/backend"
	"github.com/keytron/allan/agent/internal/memory"
	"github.com/keytron/allan/agent/internal/tools"
)

// testRuntime builds a runtime on a throwaway SQLite database, with a stub
// backend so no network is involved.
func testRuntime(t *testing.T, be backend.Backend) *Runtime {
	t.Helper()
	return testRuntimeOn(t, be, filepath.Join(t.TempDir(), "memory.db"))
}

// testRuntimeOn reuses an existing database, which is what a second `allan
// --resume` run does: the same memory.db, a fresh process.
func testRuntimeOn(t *testing.T, be backend.Backend, db string) *Runtime {
	t.Helper()
	ctx := context.Background()
	mem, err := memory.Open(db)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	cfg := config.Default()
	cfg.Memory.Enabled = true
	cfg.Memory.DBPath = db
	cfg.Memory.ChromaDBURL = ""
	cfg.Backend.Type = "ollama"
	cfg.Backend.Model = "stub"
	reg := tools.NewRegistry()
	ag := agent.New(cfg, be, reg, mem, nil, nil, t.TempDir())
	// Build всегда открывает строку сессии при старте — повторяем это здесь.
	_ = mem.StartSession(ctx, &memory.Session{ID: ag.SessionID, StartedAt: time.Now(), Backend: "ollama", Model: "stub"})
	return &Runtime{Cfg: cfg, Agent: ag, Memory: mem, Registry: reg, Workspace: ag.Workspace}
}

func TestResumeLoadsConversationIntoAgentAndScreen(t *testing.T) {
	ctx := context.Background()
	be := &countingBackend{}
	db := filepath.Join(t.TempDir(), "memory.db")
	rt := testRuntimeOn(t, be, db)

	// Первая «сессия»: пишем в базу и закрываем её, как это делает выход из TUI.
	oldID := rt.Agent.SessionID
	_ = rt.Memory.AppendMessage(ctx, oldID, "user", "найди конфиг")
	_ = rt.Memory.AppendMessage(ctx, oldID, "assistant", "конфиг в ~/.allan/config.toml")
	rt.Close(ctx)

	// Вторая «сессия»: --resume должен подхватить именно её.
	rt2 := testRuntimeOn(t, be, db)
	if _, err := rt2.Resume(ctx, "", 50); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if rt2.Agent.SessionID != oldID {
		t.Fatalf("session id = %q, ожидали %q", rt2.Agent.SessionID, oldID)
	}
	if len(rt2.Agent.History) != 2 {
		t.Fatalf("в контексте модели %d сообщений, ожидали 2", len(rt2.Agent.History))
	}
	if rt2.Agent.History[0].Content != "найди конфиг" {
		t.Fatalf("история в неверном порядке: %+v", rt2.Agent.History[0])
	}
	rt2.Close(ctx)
}

func TestResumeByExplicitID(t *testing.T) {
	ctx := context.Background()
	rt := testRuntime(t, &countingBackend{})
	older := "11111111-1111-1111-1111-111111111111"
	newer := "22222222-2222-2222-2222-222222222222"
	for _, s := range []struct {
		id    string
		text  string
		older bool
	}{{older, "старая сессия", true}, {newer, "новая сессия", false}} {
		_ = rt.Memory.StartSession(ctx, &memory.Session{ID: s.id, StartedAt: time.Now(), Backend: "ollama", Model: "stub"})
		_ = rt.Memory.AppendMessage(ctx, s.id, "user", s.text)
		_ = rt.Memory.AppendMessage(ctx, s.id, "assistant", "ответ: "+s.text)
		_ = rt.Memory.EndSession(ctx, &memory.Session{ID: s.id})
		if !s.older {
			time.Sleep(2 * time.Millisecond)
		}
	}
	msgs, err := rt.Resume(ctx, older, 50)
	if err != nil {
		t.Fatalf("Resume по id: %v", err)
	}
	if rt.Agent.SessionID != older {
		t.Fatalf("session id = %q, ожидали %q", rt.Agent.SessionID, older)
	}
	if msgs[0].Content != "старая сессия" {
		t.Fatalf("загрузилась не та сессия: %q", msgs[0].Content)
	}
	rt.Close(ctx)
}

func TestResumeWithoutAnySession(t *testing.T) {
	rt := testRuntime(t, &countingBackend{})
	if _, err := rt.Resume(context.Background(), "", 50); err == nil {
		t.Fatal("без завершённых сессий --resume должен ругаться, а не молча начинать заново")
	}
	rt.Close(context.Background())
}

func TestResumeWithoutMemory(t *testing.T) {
	cfg := config.Default()
	cfg.Memory.Enabled = false
	cfg.Memory.ChromaDBURL = ""
	reg := tools.NewRegistry()
	ag := agent.New(cfg, &countingBackend{}, reg, nil, nil, nil, t.TempDir())
	rt := &Runtime{Cfg: cfg, Agent: ag, Registry: reg, Workspace: ag.Workspace}
	if _, err := rt.Resume(context.Background(), "", 10); err == nil {
		t.Fatal("без памяти --resume должен объяснить, что память отключена")
	}
}

// countingBackend is a backend that never calls the network.
type countingBackend struct{ calls int }

func (c *countingBackend) Name() string { return "stub" }
func (c *countingBackend) Chat(ctx context.Context, m []backend.Message, d []backend.ToolDef) (*backend.Response, error) {
	c.calls++
	return &backend.Response{Content: "ok"}, nil
}
func (c *countingBackend) Stream(ctx context.Context, m []backend.Message, d []backend.ToolDef, out chan<- backend.Token) error {
	out <- backend.Token{Text: "ok", Done: true}
	return nil
}
func (c *countingBackend) Models(ctx context.Context) ([]string, error) { return nil, nil }
func (c *countingBackend) Health(ctx context.Context) error             { return nil }
