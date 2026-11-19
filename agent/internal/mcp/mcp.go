// Package mcp подключает внешние MCP-серверы (Model Context Protocol) и отдаёт
// их инструменты агенту как обычные инструменты Allan.
//
// Серверы описываются в config.toml ([mcp.<имя>]). Подключение общее на весь
// процесс: allan serve собирает агента на каждую сессию, а поднимать
// stdio-серверы заново каждый раз было бы долго и расточительно. Подключение
// идёт в фоне: TUI стартует сразу, инструменты появляются по мере готовности.
package mcp

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/tools"
)

// Version уходит серверу в initialize.
var Version = "dev"

const (
	connectTimeout = 30 * time.Second
	defaultCallTO  = 120 * time.Second
	maxOutput      = 100_000 // символов результата в контекст модели
	legacyProtocol = "2025-11-25"
)

// State — состояние одного сервера.
type State string

const (
	StateConnecting State = "подключается"
	StateReady      State = "готов"
	StateFailed     State = "ошибка"
	StateDisabled   State = "выключен"
)

// ServerStatus — что показать в /mcp и allan mcp list.
type ServerStatus struct {
	Name      string
	Transport string // stdio | http | sse
	State     State
	Error     string
	Tools     []string // имена инструментов на сервере
	Load      string   // auto | always | deferred
	Tokens    int      // оценка: сколько стоят схемы всех его инструментов
}

// Registrar — то, куда менеджер кладёт инструменты (tools.Registry).
type Registrar interface {
	RegisterWith(t tools.Tool, p tools.LoadPolicy)
}

// Policy переводит load из конфига в политику реестра.
func Policy(load string) tools.LoadPolicy {
	switch strings.ToLower(strings.TrimSpace(load)) {
	case "always":
		return tools.LoadAlways
	case "deferred", "lazy", "on-demand":
		return tools.LoadDeferred
	default:
		return tools.LoadAuto
	}
}

type server struct {
	name    string
	cfg     config.MCPServer
	session *sdk.ClientSession
	state   State
	err     string
	tools   []*Tool
}

// Manager держит подключения ко всем серверам процесса.
type Manager struct {
	mu      sync.Mutex
	servers map[string]*server
	sinks   []func(*Tool)
	ready   chan struct{}
}

var (
	sharedMu  sync.Mutex
	shared    *Manager
	sharedKey string
)

// Shared возвращает менеджер процесса для этой конфигурации, при первом вызове
// запуская подключение в фоне. Если набор серверов в конфиге поменялся,
// старые подключения закрываются и поднимаются новые.
func Shared(cfg *config.Config) *Manager {
	key := configKey(cfg)
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if shared != nil && sharedKey == key {
		return shared
	}
	if shared != nil {
		go shared.Close()
	}
	shared = newManager(cfg)
	sharedKey = key
	return shared
}

// Shutdown закрывает общий менеджер (завершает stdio-серверы).
func Shutdown() {
	sharedMu.Lock()
	m := shared
	shared, sharedKey = nil, ""
	sharedMu.Unlock()
	if m != nil {
		m.Close()
	}
}

func configKey(cfg *config.Config) string {
	if cfg == nil || len(cfg.MCP) == 0 {
		return ""
	}
	b, _ := json.Marshal(cfg.MCP)
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:])
}

func newManager(cfg *config.Config) *Manager {
	m := &Manager{servers: map[string]*server{}, ready: make(chan struct{})}
	if cfg == nil {
		close(m.ready)
		return m
	}
	var wg sync.WaitGroup
	for name, sc := range cfg.MCP {
		s := &server{name: name, cfg: sc, state: StateConnecting}
		if sc.Disabled {
			s.state = StateDisabled
		}
		m.servers[name] = s
		if sc.Disabled {
			continue
		}
		wg.Add(1)
		go func(s *server) {
			defer wg.Done()
			m.connect(s)
		}(s)
	}
	go func() { wg.Wait(); close(m.ready) }()
	return m
}

// Attach отдаёт инструменты в реестр: уже готовые сразу, остальные — когда
// их сервер подключится. Реестр потокобезопасный, агент видит новые
// инструменты со следующего запроса к модели.
func (m *Manager) Attach(r Registrar) {
	m.mu.Lock()
	m.sinks = append(m.sinks, func(t *Tool) { r.RegisterWith(t, t.policy) })
	var ready []*Tool
	for _, s := range m.servers {
		ready = append(ready, s.tools...)
	}
	m.mu.Unlock()
	for _, t := range ready {
		r.RegisterWith(t, t.policy)
	}
}

// Wait ждёт окончания первых попыток подключения (для CLI: allan mcp list).
func (m *Manager) Wait(ctx context.Context) {
	select {
	case <-m.ready:
	case <-ctx.Done():
	}
}

// Status — состояние серверов по имени.
func (m *Manager) Status() []ServerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ServerStatus, 0, len(m.servers))
	for _, s := range m.servers {
		st := ServerStatus{Name: s.name, Transport: transportOf(s.cfg), State: s.state, Error: s.err, Load: loadName(s.cfg.Load)}
		for _, t := range s.tools {
			st.Tools = append(st.Tools, t.remote)
			st.Tokens += tools.EstimateTokens(t)
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Close завершает все сессии (stdio-серверы получают EOF и выходят).
func (m *Manager) Close() {
	m.mu.Lock()
	var sessions []*sdk.ClientSession
	for _, s := range m.servers {
		if s.session != nil {
			sessions = append(sessions, s.session)
			s.session = nil
		}
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, cs := range sessions {
		wg.Add(1)
		go func(cs *sdk.ClientSession) { defer wg.Done(); _ = cs.Close() }(cs)
	}
	wg.Wait()
}

func loadName(load string) string {
	switch Policy(load) {
	case tools.LoadAlways:
		return "always"
	case tools.LoadDeferred:
		return "deferred"
	}
	return "auto"
}

func transportOf(c config.MCPServer) string {
	if c.URL == "" {
		return "stdio"
	}
	if strings.EqualFold(c.Transport, "sse") {
		return "sse"
	}
	return "http"
}

func (m *Manager) fail(s *server, err error) {
	m.mu.Lock()
	s.state = StateFailed
	s.err = err.Error()
	m.mu.Unlock()
}

func (m *Manager) connect(s *server) {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	tail := &tailBuf{}
	t, err := transportFor(s.cfg, tail)
	if err != nil {
		m.fail(s, err)
		return
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "allan", Version: Version}, nil)
	// Классический initialize: SDK по умолчанию сначала шлёт server/discover
	// (SEP-2575), а многие серверы (rmcp, старые TS) на незнакомом запросе
	// просто завершаются. Новые серверы initialize тоже принимают.
	cs, err := client.Connect(ctx, t, &sdk.ClientSessionOptions{ProtocolVersion: legacyProtocol})
	if err != nil {
		if msg := tail.String(); msg != "" {
			err = fmt.Errorf("%w; stderr: %s", err, msg)
		}
		m.fail(s, fmt.Errorf("подключение: %w", err))
		return
	}
	var list []*sdk.Tool
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			_ = cs.Close()
			m.fail(s, fmt.Errorf("список инструментов: %w", err))
			return
		}
		list = append(list, tool)
	}
	allow := map[string]bool{}
	for _, n := range s.cfg.Tools {
		allow[n] = true
	}
	callTO := defaultCallTO
	if s.cfg.Timeout > 0 {
		callTO = time.Duration(s.cfg.Timeout) * time.Second
	}
	note := ""
	if ir := cs.InitializeResult(); ir != nil {
		note = oneLine(ir.Instructions)
		if note == "" && ir.ServerInfo != nil {
			note = oneLine(ir.ServerInfo.Description)
		}
	}
	policy := Policy(s.cfg.Load)
	var made []*Tool
	for _, st := range list {
		if len(allow) > 0 && !allow[st.Name] {
			continue
		}
		made = append(made, &Tool{
			name:    ToolName(s.name, st.Name),
			remote:  st.Name,
			server:  s.name,
			desc:    st.Description,
			schema:  schemaOf(st.InputSchema),
			session: cs,
			timeout: callTO,
			note:    note,
			policy:  policy,
		})
	}
	m.mu.Lock()
	s.session = cs
	s.state = StateReady
	s.err = ""
	s.tools = made
	sinks := append([]func(*Tool){}, m.sinks...)
	m.mu.Unlock()
	for _, t := range made {
		for _, sink := range sinks {
			sink(t)
		}
	}
}

func expand(v string) string { return os.ExpandEnv(v) }

func transportFor(c config.MCPServer, tail *tailBuf) (sdk.Transport, error) {
	if c.URL == "" {
		if c.Command == "" {
			return nil, errors.New("нужен command (stdio) или url (HTTP)")
		}
		args := make([]string, len(c.Args))
		for i, a := range c.Args {
			args[i] = expand(a)
		}
		cmd := exec.Command(expand(c.Command), args...)
		cmd.Env = os.Environ()
		for k, v := range c.Env {
			cmd.Env = append(cmd.Env, k+"="+expand(v))
		}
		// stderr сервера не мешаем в TUI, но хвост держим для текста ошибки
		cmd.Stderr = tail
		return &sdk.CommandTransport{Command: cmd}, nil
	}
	hc := &http.Client{Transport: headerRT{base: http.DefaultTransport, headers: c.Headers}}
	if strings.EqualFold(c.Transport, "sse") {
		return &sdk.SSEClientTransport{Endpoint: expand(c.URL), HTTPClient: hc}, nil
	}
	return &sdk.StreamableClientTransport{Endpoint: expand(c.URL), HTTPClient: hc}, nil
}

type headerRT struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h headerRT) RoundTrip(r *http.Request) (*http.Response, error) {
	if len(h.headers) > 0 {
		r = r.Clone(r.Context())
		for k, v := range h.headers {
			r.Header.Set(k, expand(v))
		}
	}
	return h.base.RoundTrip(r)
}

var nameUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// ToolName — имя инструмента для модели: mcp__<сервер>__<инструмент>.
// Провайдеры (OpenAI и совместимые) принимают только [a-zA-Z0-9_-] и до 64
// символов, поэтому лишнее заменяется, а длинное обрезается с хвостом-хешем.
func ToolName(server, tool string) string {
	n := "mcp__" + nameUnsafe.ReplaceAllString(server, "_") + "__" + nameUnsafe.ReplaceAllString(tool, "_")
	if len(n) <= 64 {
		return n
	}
	sum := sha1.Sum([]byte(n))
	return n[:55] + "_" + hex.EncodeToString(sum[:])[:8]
}

func schemaOf(v any) map[string]any {
	var m map[string]any
	switch x := v.(type) {
	case map[string]any:
		m = x
	case nil:
	default:
		b, err := json.Marshal(x)
		if err == nil {
			_ = json.Unmarshal(b, &m)
		}
	}
	if m == nil {
		m = map[string]any{}
	}
	if _, ok := m["type"]; !ok {
		m["type"] = "object"
	}
	if _, ok := m["properties"]; !ok {
		m["properties"] = map[string]any{}
	}
	return m
}

// Tool — инструмент MCP-сервера в виде инструмента Allan.
type Tool struct {
	name, remote, server, desc string
	schema                     map[string]any
	session                    *sdk.ClientSession
	timeout                    time.Duration
	note                       string
	policy                     tools.LoadPolicy
}

func (t *Tool) Group() string     { return t.server }
func (t *Tool) GroupNote() string { return t.note }

// oneLine — первое предложение, не длиннее 100 символов: для каталога.
func oneLine(s string) string {
	s = strings.TrimSpace(strings.SplitN(strings.TrimSpace(s), "\n", 2)[0])
	if i := strings.Index(s, ". "); i > 0 {
		s = s[:i]
	}
	if r := []rune(s); len(r) > 100 {
		s = string(r[:100]) + "…"
	}
	return s
}

func (t *Tool) Name() string { return t.name }

func (t *Tool) Description() string {
	d := strings.TrimSpace(t.desc)
	if d == "" {
		d = t.remote
	}
	return "[MCP " + t.server + "] " + d
}

func (t *Tool) Schema() map[string]any { return t.schema }

func (t *Tool) Run(ctx context.Context, params map[string]any) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	if params == nil {
		params = map[string]any{}
	}
	res, err := t.session.CallTool(ctx, &sdk.CallToolParams{Name: t.remote, Arguments: params})
	if err != nil {
		return "", fmt.Errorf("MCP %s/%s: %w", t.server, t.remote, err)
	}
	out := renderResult(res)
	if res.IsError {
		return "", fmt.Errorf("MCP %s/%s: %s", t.server, t.remote, out)
	}
	return out, nil
}

func renderResult(res *sdk.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		switch v := c.(type) {
		case *sdk.TextContent:
			parts = append(parts, v.Text)
		case *sdk.ImageContent:
			parts = append(parts, fmt.Sprintf("[изображение %s, %d байт]", v.MIMEType, len(v.Data)))
		case *sdk.AudioContent:
			parts = append(parts, fmt.Sprintf("[аудио %s, %d байт]", v.MIMEType, len(v.Data)))
		case *sdk.ResourceLink:
			parts = append(parts, "[ресурс] "+v.URI)
		case *sdk.EmbeddedResource:
			if v.Resource != nil {
				if v.Resource.Text != "" {
					parts = append(parts, v.Resource.Text)
				} else {
					parts = append(parts, "[ресурс] "+v.Resource.URI)
				}
			}
		default:
			if b, err := json.Marshal(c); err == nil {
				parts = append(parts, string(b))
			}
		}
	}
	if len(parts) == 0 && res.StructuredContent != nil {
		if b, err := json.MarshalIndent(res.StructuredContent, "", "  "); err == nil {
			parts = append(parts, string(b))
		}
	}
	out := strings.Join(parts, "\n")
	if r := []rune(out); len(r) > maxOutput {
		out = string(r[:maxOutput]) + fmt.Sprintf("\n… [обрезано: %d из %d символов]", maxOutput, len(r))
	}
	if out == "" {
		out = "(пустой результат)"
	}
	return out
}

// tailBuf хранит последние байты stderr сервера: причина, по которой он упал.
type tailBuf struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tailBuf) Write(p []byte) (int, error) {
	t.mu.Lock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > 2048 {
		t.buf = t.buf[len(t.buf)-2048:]
	}
	t.mu.Unlock()
	return len(p), nil
}

func (t *tailBuf) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := strings.Split(strings.TrimSpace(string(t.buf)), "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return strings.Join(lines, " | ")
}
