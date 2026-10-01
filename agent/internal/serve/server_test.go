package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/agent"
	"github.com/keytron/allan/agent/internal/backend"
	"github.com/keytron/allan/agent/internal/bootstrap"
	"github.com/keytron/allan/agent/internal/tools"
)

// stubBackend answers with a fixed reply, optionally asking for one tool call
// first, so the SSE stream can be checked end to end without a real LLM.
type stubBackend struct {
	toolFirst bool
	turns     int
}

func (s *stubBackend) Name() string { return "stub" }

func (s *stubBackend) Chat(ctx context.Context, msgs []backend.Message, defs []backend.ToolDef) (*backend.Response, error) {
	s.turns++
	if s.toolFirst && s.turns == 1 {
		return &backend.Response{
			ToolCalls: []backend.ToolCall{{
				ID:        "call-1",
				Name:      "list_files",
				Arguments: map[string]any{"path": "."},
			}},
		}, nil
	}
	return &backend.Response{Content: "готово: " + lastUser(msgs)}, nil
}

func (s *stubBackend) Stream(ctx context.Context, msgs []backend.Message, defs []backend.ToolDef, out chan<- backend.Token) error {
	resp, err := s.Chat(ctx, msgs, defs)
	if err != nil {
		return err
	}
	for _, w := range strings.SplitAfter(resp.Content, " ") {
		out <- backend.Token{Text: w}
	}
	out <- backend.Token{Done: true}
	return nil
}

func (s *stubBackend) Models(ctx context.Context) ([]string, error) {
	return []string{"stub-1", "stub-2"}, nil
}
func (s *stubBackend) Health(ctx context.Context) error { return nil }

func lastUser(msgs []backend.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == backend.RoleUser {
			return msgs[i].Content
		}
	}
	return ""
}

func newTestServer(t *testing.T, token string, be backend.Backend) *Server {
	t.Helper()
	build := func(ctx context.Context) (*bootstrap.Runtime, error) {
		cfg := config.Default()
		cfg.Backend.Type = "ollama"
		cfg.Backend.Model = "stub"
		cfg.Memory.Enabled = false
		cfg.Memory.ChromaDBURL = ""
		reg := tools.NewRegistry()
		reg.Register(&echoTool{})
		ag := agent.New(cfg, be, reg, nil, nil, nil, t.TempDir())
		return &bootstrap.Runtime{
			Cfg:       cfg,
			Agent:     ag,
			Registry:  reg,
			Workspace: ag.Workspace,
		}, nil
	}
	srv, err := New(Options{Addr: "127.0.0.1:0", Token: token, Version: "test", Build: build})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv
}

type echoTool struct{}

func (e *echoTool) Name() string        { return "list_files" }
func (e *echoTool) Description() string { return "тестовая тулза" }
func (e *echoTool) Schema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (e *echoTool) Run(ctx context.Context, p map[string]any) (string, error) { return "a\nb\nc", nil }

func post(t *testing.T, h http.Handler, path, token, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestNewRefusesEmptyToken(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("воркер без токена должен не запускаться")
	}
}

func TestAuthRequired(t *testing.T) {
	srv := newTestServer(t, "secret-token", &stubBackend{})
	h := srv.Handler()

	if rec := post(t, h, "/v1/state", "", `{}`, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("без токена ждём 401, получили %d", rec.Code)
	}
	if rec := post(t, h, "/v1/state", "wrong", `{}`, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("с чужим токеном ждём 403, получили %d", rec.Code)
	}
	if rec := post(t, h, "/v1/state", "secret-token", `{}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("с верным токеном ждём 200, получили %d: %s", rec.Code, rec.Body.String())
	}
	// /health открыт: по нему nginx проверяет, что воркер жив.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/health ждём 200, получили %d", rec.Code)
	}
}

func TestChatStreamsEvents(t *testing.T) {
	srv := newTestServer(t, "secret-token", &stubBackend{toolFirst: true})
	h := srv.Handler()

	body := `{"session":"s1","text":"покажи файлы"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("X-Allan-Client", "user-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("чат: %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("ожидали SSE, получили %q", ct)
	}

	events := parseSSE(t, rec.Body.String())
	for _, want := range []string{"start", "tool_call", "tool_result", "final", "done"} {
		if !events[want] {
			t.Fatalf("в потоке нет события %q (получено %v)", want, keys(events))
		}
	}
	if !strings.Contains(rec.Body.String(), "готово: покажи файлы") {
		t.Fatal("финальный ответ не дошёл до клиента")
	}
}

func TestSessionsAreNamespacedPerClient(t *testing.T) {
	srv := newTestServer(t, "secret-token", &stubBackend{})
	h := srv.Handler()

	// Клиент A создаёт сессию и говорит в неё.
	rec := post(t, h, "/v1/session/new", "secret-token", `{}`, map[string]string{"X-Allan-Client": "a"})
	if rec.Code != http.StatusOK {
		t.Fatalf("создание сессии: %d", rec.Code)
	}
	var created struct {
		Session string `json:"session"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Session == "" {
		t.Fatal("сервер не вернул id сессии")
	}

	// Клиент B не должен найти чужую сессию по тому же id.
	rec = post(t, h, "/v1/session/clear", "secret-token", `{"session":"`+created.Session+`"}`,
		map[string]string{"X-Allan-Client": "b"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("чужую сессию чистить нельзя: получили %d", rec.Code)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	for _, sess := range srv.sessions {
		if sess.client != "a" {
			t.Fatalf("сессия создана с чужим client=%q", sess.client)
		}
	}
}

func TestChatKeepsSessionFromBody(t *testing.T) {
	srv := newTestServer(t, "secret-token", &stubBackend{})
	h := srv.Handler()

	// Первый запрос создаёт сессию, второй с тем же id в теле обязан её найти:
	// иначе история диалога не сохранялась бы между сообщениями.
	first := post(t, h, "/v1/chat", "secret-token", `{"session":"chat-1","text":"привет"}`,
		map[string]string{"X-Allan-Client": "u"})
	if first.Code != http.StatusOK {
		t.Fatalf("первый запрос: %d %s", first.Code, first.Body.String())
	}
	second := post(t, h, "/v1/chat", "secret-token", `{"session":"chat-1","text":"ещё раз"}`,
		map[string]string{"X-Allan-Client": "u"})
	if second.Code != http.StatusOK {
		t.Fatalf("второй запрос: %d %s", second.Code, second.Body.String())
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.sessions) != 1 {
		t.Fatalf("ожидалась одна сессия, их %d", len(srv.sessions))
	}
	for _, sess := range srv.sessions {
		if sess.id != "u|chat-1" {
			t.Fatalf("id сессии = %q, ожидали u|chat-1", sess.id)
		}
		if n := len(sess.rt.Agent.History); n < 4 {
			t.Fatalf("история не накоплена: %d сообщений", n)
		}
	}
}

func TestEmptyTokenRejectedByChat(t *testing.T) {
	srv := newTestServer(t, "secret-token", &stubBackend{})
	rec := post(t, srv.Handler(), "/v1/chat", "secret-token", `{"text":"  "}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("пустое сообщение: ждём 400, получили %d", rec.Code)
	}
}

func TestStopOnlyTouchesOwnSession(t *testing.T) {
	srv := newTestServer(t, "secret-token", &stubBackend{})
	h := srv.Handler()

	rec := post(t, h, "/v1/stop", "secret-token", `{"session":"s-42"}`,
		map[string]string{"X-Allan-Client": "u"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("stop на несуществующей сессии: ждём 404, получили %d", rec.Code)
	}

	created := mustSession(t, post(t, h, "/v1/session/new", "secret-token", `{}`,
		map[string]string{"X-Allan-Client": "u"}))
	rec = post(t, h, "/v1/stop", "secret-token", `{"session":"`+strings.TrimPrefix(created, "u|")+`"}`,
		map[string]string{"X-Allan-Client": "u"})
	if rec.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", rec.Code, rec.Body.String())
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.sessions) != 1 {
		t.Fatalf("ожидалась одна сессия, их %d", len(srv.sessions))
	}
	for _, sess := range srv.sessions {
		if sess.client != "u" {
			t.Fatalf("client=%q", sess.client)
		}
	}
}

func mustSession(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		Session string `json:"session"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Session
}

func TestSkillsEndpointWorks(t *testing.T) {
	srv := newTestServer(t, "secret-token", &stubBackend{})
	rec := post(t, srv.Handler(), "/v1/skills", "secret-token", `{}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("skills: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"skills"`) {
		t.Fatalf("неожиданный ответ: %s", rec.Body.String())
	}
}

func parseSSE(t *testing.T, body string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: ") {
			out[strings.TrimPrefix(line, "event: ")] = true
		}
	}
	return out
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
