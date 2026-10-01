// Package serve exposes the agent over HTTP so a remote client (the phone
// app, the site) can drive it without a terminal.
//
// Design notes:
//   - The agent stays on the machine that owns the tools, keys and memory; the
//     worker only answers requests, it never dials out.
//   - Every request must carry the shared worker token. The token is the only
//     thing that makes this process safe to expose, so it is compared in
//     constant time and the listener is expected to be bound to a private
//     interface.
//   - Sessions are namespaced per client id, so one client can never read or
//     cancel another client's conversation even if the front end has a bug.
package serve

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/agent"
	"github.com/keytron/allan/agent/internal/backend"
	"github.com/keytron/allan/agent/internal/bootstrap"
	"github.com/keytron/allan/agent/internal/link"
	"github.com/keytron/allan/agent/internal/memory"
	"github.com/keytron/allan/agent/internal/tools"
)

// Options configures the worker.
type Options struct {
	Addr     string
	Token    string
	Version  string
	MaxTurns int // how many sessions to keep before the oldest idle one is dropped
	// Build makes a fresh runtime per session. Tests and the TUI reuse the
	// default, which builds from the local config.
	Build func(ctx context.Context) (*bootstrap.Runtime, error)
}

// Server is the HTTP worker.
type Server struct {
	opts  Options
	token string
	build func(ctx context.Context) (*bootstrap.Runtime, error)

	mu       sync.Mutex
	sessions map[string]*session
}

type session struct {
	id       string // namespaced: "<client>|<session>"
	client   string
	rt       *bootstrap.Runtime
	runMu    sync.Mutex // one turn at a time per session
	cancel   context.CancelFunc
	busy     bool
	created  time.Time
	lastUsed time.Time
	turns    int
}

// New builds a worker. An empty token is refused: an unauthenticated agent
// that can run shell commands must never start.
func New(opts Options) (*Server, error) {
	if strings.TrimSpace(opts.Token) == "" {
		return nil, errors.New("serve: пустой токен — укажите --token, ALLAN_WORKER_TOKEN или подключитесь через allan connect")
	}
	if opts.Build == nil {
		opts.Build = func(ctx context.Context) (*bootstrap.Runtime, error) {
			return bootstrap.Build(ctx, bootstrap.Options{PickLocalModel: true})
		}
	}
	if opts.MaxTurns <= 0 {
		opts.MaxTurns = 32
	}
	return &Server{opts: opts, token: opts.Token, build: opts.Build, sessions: map[string]*session{}}, nil
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/v1/state", s.auth(s.handleState))
	mux.HandleFunc("/v1/chat", s.auth(s.handleChat))
	mux.HandleFunc("/v1/stop", s.auth(s.handleStop))
	mux.HandleFunc("/v1/history", s.auth(s.handleHistory))
	mux.HandleFunc("/v1/session/new", s.auth(s.handleSessionNew))
	mux.HandleFunc("/v1/session/clear", s.auth(s.handleSessionClear))
	mux.HandleFunc("/v1/session/delete", s.auth(s.handleSessionDelete))
	mux.HandleFunc("/v1/models", s.auth(s.handleModels))
	mux.HandleFunc("/v1/model", s.auth(s.handleModel))
	mux.HandleFunc("/v1/skills", s.auth(s.handleSkills))
	return withCORS(mux)
}

// Run serves until ctx is done.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.opts.Addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// A turn may run bash for minutes, so no write timeout.
		IdleTimeout: 5 * time.Minute,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
		s.closeAll()
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.sessions {
		if sess.rt != nil {
			sess.rt.Close(context.Background())
		}
	}
	s.sessions = map[string]*session{}
}

// ---- auth ----

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := bearerToken(r)
		if got == "" {
			writeErr(w, http.StatusUnauthorized, "нет токена")
			return
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			writeErr(w, http.StatusForbidden, "токен не подходит")
			return
		}
		next(w, r)
	}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return strings.TrimSpace(r.Header.Get("X-Allan-Token"))
}

// clientID is the front-end's user identity. The worker trusts it only for
// namespacing; authentication is the token's job.
func clientID(r *http.Request) string {
	id := strings.TrimSpace(r.Header.Get("X-Allan-Client"))
	if id == "" {
		return "default"
	}
	if len(id) > 128 {
		id = id[:128]
	}
	return id
}

// ---- handlers ----

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	busy := 0
	for _, sess := range s.sessions {
		if sess.busy {
			busy++
		}
	}
	total := len(s.sessions)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"version":  s.opts.Version,
		"pid":      1,
		"sessions": total,
		"busy":     busy,
		"uptime":   time.Since(startedAt).Round(time.Second).String(),
		"host":     hostLabel(),
	})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	rt, err := s.runtimeFor(r)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	sess := &session{rt: rt}
	s.mu.Lock()
	list := make([]map[string]any, 0, len(s.sessions))
	for _, x := range s.sessions {
		list = append(list, map[string]any{
			"session":   x.id,
			"client":    x.client,
			"busy":      x.busy,
			"created":   x.created,
			"last_used": x.lastUsed,
		})
	}
	s.mu.Unlock()
	providers := []map[string]any{}
	for name, p := range sess.rt.Cfg.Providers {
		providers = append(providers, map[string]any{
			"provider": name,
			"type":     p.Type,
			"base_url": p.BaseURL,
			"has_key":  p.APIKey != "",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":   s.opts.Version,
		"backend":   sess.rt.Cfg.Backend.Type,
		"model":     sess.rt.Cfg.Backend.Model,
		"workspace": sess.rt.Workspace,
		"agent":     sess.rt.Agent.SessionID,
		"machine":   link.MachineID(),
		"name":      link.MachineName(),
		"tools":     toolNames(sess.rt.Registry),
		"providers": providers,
		"memory":    sess.rt.Memory != nil,
		"skills":    sess.rt.Skills != nil,
		"sessions":  list,
		"platform":  hostLabel(),
	})
}

type chatRequest struct {
	Session    string `json:"session"`
	Text       string `json:"text"`
	NewSession bool   `json:"new_session"`
	Limit      int    `json:"limit"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeErr(w, http.StatusBadRequest, "пустое сообщение")
		return
	}
	sess, err := s.sessionFor(r, req.Session, req.NewSession || req.Session == "")
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if !sess.runMu.TryLock() {
		writeErr(w, http.StatusConflict, "в этой сессии уже идёт запрос")
		return
	}
	defer sess.runMu.Unlock()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "стриминг не поддерживается")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	s.setCancel(sess, cancel)
	defer s.setCancel(sess, nil)

	events := make(chan agent.Event, 32)
	go sess.rt.Agent.Run(ctx, text, events)

	s.send(w, flusher, "start", map[string]any{
		"session": sess.id,
		"agent":   sess.rt.Agent.SessionID,
		"model":   sess.rt.Cfg.Backend.Model,
	})
	for ev := range events {
		s.send(w, flusher, ev.Kind, eventPayload(sess, ev))
		if ev.Kind == "final" {
			s.send(w, flusher, "done", map[string]any{
				"session":    sess.id,
				"tool_calls": sess.rt.Agent.Stats.ToolCalls,
			})
		}
	}
	flusher.Flush()
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	sess, err := s.lookupExisting(r, req.Session)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	s.mu.Lock()
	cancel := sess.cancel
	busy := sess.busy
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	writeJSON(w, http.StatusOK, map[string]any{"stopped": busy, "session": sess.id})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	limit := req.Limit
	if limit <= 0 || limit > 500 {
		limit = 40
	}
	// A conversation survives a worker restart in SQLite, so fall back to the
	// database instead of failing when the session is not in memory.
	id := sessionIDWith(r, req.Session)
	if id == "" {
		id = "default"
	}
	key := namespace(clientID(r), id)
	msgs := []map[string]any{}
	if sess, err := s.lookupExisting(r, req.Session); err == nil {
		if sess.rt.Memory != nil {
			if last, err := sess.rt.Memory.SessionMessages(r.Context(), sess.id, limit); err == nil {
				msgs = convMessages(last)
			}
		} else {
			msgs = inMemoryMessages(sess, limit)
		}
	} else if rt, berr := s.build(r.Context()); berr == nil && rt.Memory != nil {
		if last, err := rt.Memory.SessionMessages(r.Context(), key, limit); err == nil {
			msgs = convMessages(last)
			rt.Close(r.Context())
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": key, "messages": msgs})
}

func convMessages(in []memory.ConvMessage) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, m := range in {
		out = append(out, map[string]any{"role": m.Role, "content": m.Content, "at": m.Timestamp})
	}
	return out
}

func inMemoryMessages(sess *session, limit int) []map[string]any {
	out := []map[string]any{}
	hist := sess.rt.Agent.History
	if len(hist) > limit {
		hist = hist[len(hist)-limit:]
	}
	for _, m := range hist {
		if m.Role == backend.RoleUser || m.Role == backend.RoleAssistant {
			out = append(out, map[string]any{"role": string(m.Role), "content": m.Content})
		}
	}
	return out
}

func (s *Server) handleSessionNew(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessionFor(r, "", true)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": sess.id, "created": true})
}

func (s *Server) handleSessionClear(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	sess, err := s.lookupExisting(r, req.Session)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	sess.runMu.Lock()
	sess.rt.Agent.ClearHistory()
	sess.runMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"session": sess.id, "cleared": true})
}

func (s *Server) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	client := clientID(r)
	id := sessionID(r)
	if id == "" {
		writeErr(w, http.StatusBadRequest, "нужен session")
		return
	}
	key := namespace(client, id)
	s.mu.Lock()
	sess, ok := s.sessions[key]
	if ok {
		delete(s.sessions, key)
	}
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"deleted": false, "session": key})
		return
	}
	sess.runMu.Lock()
	if sess.cancel != nil {
		sess.cancel()
	}
	sess.rt.Close(context.Background())
	sess.runMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "session": key})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	rt, err := s.runtimeFor(r)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if provider == "" {
		provider = rt.Cfg.Backend.Type
	}
	be, err := backend.ForProvider(provider, rt.Cfg, rt.Cfg.Backend.Model)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	models, err := be.Models(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": provider, "models": models})
}

type modelRequest struct {
	Session  string `json:"session"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func (s *Server) handleModel(w http.ResponseWriter, r *http.Request) {
	var req modelRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Model == "" {
		writeErr(w, http.StatusBadRequest, "нужен model")
		return
	}
	provider := backend.NormalizeProvider(req.Provider)
	if provider == "" {
		provider = "openai"
	}
	rt, err := s.runtimeFor(r)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	be, err := backend.ForProvider(provider, rt.Cfg, req.Model)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rt.Cfg.Backend.Type = provider
	rt.Cfg.Backend.Model = req.Model
	if p, ok := rt.Cfg.Providers[provider]; ok {
		rt.Cfg.Backend.BaseURL = p.BaseURL
		rt.Cfg.Backend.APIKey = p.APIKey
	} else {
		rt.Cfg.Backend.BaseURL = backend.DefaultBaseURL(provider)
		if backend.IsLocalProvider(provider) {
			rt.Cfg.Backend.APIKey = ""
		}
	}
	if err := config.Save(rt.Cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.mu.Lock()
	for _, sess := range s.sessions {
		sess.runMu.Lock()
		sess.rt.Agent.Backend = be
		sess.runMu.Unlock()
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"provider": provider, "model": req.Model})
}

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	rt, err := s.runtimeFor(r)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	out := []map[string]any{}
	if rt.Skills != nil {
		if list, err := rt.Skills.List(context.Background()); err == nil {
			for _, sk := range list {
				out = append(out, map[string]any{
					"name": sk.Name, "description": sk.Description,
					"trigger": sk.Trigger, "used": sk.UsedCount,
				})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": out})
}

// ---- sessions ----

// sessionID reads the session from the query string or the header. The id in
// a JSON body takes priority: that is what the phone app sends.
func sessionID(r *http.Request) string {
	return sessionIDWith(r, "")
}

func sessionIDWith(r *http.Request, fromBody string) string {
	if v := strings.TrimSpace(fromBody); v != "" {
		return v
	}
	if v := r.URL.Query().Get("session"); v != "" {
		return v
	}
	return strings.TrimSpace(r.Header.Get("X-Allan-Session"))
}

func namespace(client, id string) string { return client + "|" + id }

// sessionFor returns the session named by the request, creating it when asked.
// A new session gets a fresh agent runtime so a user's conversation cannot leak
// into the next one.
func (s *Server) sessionFor(r *http.Request, wantID string, fresh bool) (*session, error) {
	client := clientID(r)
	id := sessionIDWith(r, wantID)
	if fresh || id == "" {
		id = shortID()
	}
	key := namespace(client, id)

	s.mu.Lock()
	if !fresh {
		if sess, ok := s.sessions[key]; ok {
			sess.lastUsed = time.Now()
			s.mu.Unlock()
			return sess, nil
		}
	}
	s.mu.Unlock()

	rt, err := s.build(r.Context())
	if err != nil {
		return nil, fmt.Errorf("не удалось собрать агента: %w", err)
	}
	// Give the agent the caller's session id so SQLite history lines up.
	rt.Agent.SessionID = key
	if rt.Memory != nil {
		_ = rt.Memory.StartSession(context.Background(), &memory.Session{
			ID:        key,
			StartedAt: time.Now(),
			Backend:   rt.Cfg.Backend.Type,
			Model:     rt.Cfg.Backend.Model,
		})
	}

	sess := &session{
		id:       key,
		client:   client,
		rt:       rt,
		created:  time.Now(),
		lastUsed: time.Now(),
		turns:    1,
	}
	s.mu.Lock()
	s.sessions[key] = sess
	s.evictLocked()
	s.mu.Unlock()
	return sess, nil
}

// lookupExisting returns the session named by the request without creating
// one. Endpoints that change or report on a conversation use it, so a typo in
// a session id cannot silently spawn an empty session.
func (s *Server) lookupExisting(r *http.Request, wantID string) (*session, error) {
	key := namespace(clientID(r), sessionIDWith(r, wantID))
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[key]
	if !ok {
		return nil, fmt.Errorf("сессия %s не найдена", key)
	}
	sess.lastUsed = time.Now()
	return sess, nil
}

// runtimeFor gives a request a runtime without keeping a session around:
// used by read-only endpoints (state, models, skills).
func (s *Server) runtimeFor(r *http.Request) (*bootstrap.Runtime, error) {
	if sessionID(r) != "" {
		if sess, err := s.sessionFor(r, "", false); err == nil {
			return sess.rt, nil
		}
	}
	return s.build(r.Context())
}

// evictLocked drops the least recently used sessions above the limit. It must
// be called with s.mu held.
func (s *Server) evictLocked() {
	if len(s.sessions) <= s.opts.MaxTurns {
		return
	}
	type aged struct {
		key  string
		when time.Time
	}
	all := make([]aged, 0, len(s.sessions))
	for k, sess := range s.sessions {
		all = append(all, aged{k, sess.lastUsed})
	}
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].when.Before(all[j-1].when); j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	drop := len(s.sessions) - s.opts.MaxTurns
	for i := 0; i < drop; i++ {
		if sess, ok := s.sessions[all[i].key]; ok && !sess.busy {
			delete(s.sessions, all[i].key)
			go sess.rt.Close(context.Background())
		}
	}
}

func (s *Server) setCancel(sess *session, cancel context.CancelFunc) {
	s.mu.Lock()
	sess.cancel = cancel
	sess.busy = cancel != nil
	if cancel != nil {
		sess.lastUsed = time.Now()
	}
	s.mu.Unlock()
}

func (s *Server) send(w http.ResponseWriter, f http.Flusher, event string, payload map[string]any) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
	f.Flush()
}

func eventPayload(sess *session, ev agent.Event) map[string]any {
	out := map[string]any{
		"session": sess.id,
		"kind":    ev.Kind,
		"text":    ev.Text,
	}
	if ev.Tool != "" {
		out["tool"] = ev.Tool
		out["tool_id"] = ev.ToolID
		out["args"] = ev.Args
	}
	if ev.Result != "" {
		out["result"] = ev.Result
	}
	if ev.IsError {
		out["is_error"] = true
	}
	if ev.Skill != nil {
		out["skill"] = map[string]any{
			"name": ev.Skill.Name, "description": ev.Skill.Description, "trigger": ev.Skill.Trigger,
		}
	}
	return out
}

var startedAt = time.Now()

func hostLabel() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return host + " " + runtime.GOOS + "/" + runtime.GOARCH
}

// decode reads a small JSON body. Unknown fields are ignored on purpose: a
// newer phone app must keep working against an older worker.
func decode(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("пустое тело запроса")
	}
	raw, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 4<<20))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return json.Unmarshal(raw, v)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func toolNames(reg *tools.Registry) []string {
	if reg == nil {
		return nil
	}
	list := reg.List()
	out := make([]string, 0, len(list))
	for _, t := range list {
		out = append(out, t.Name())
	}
	return out
}

func shortID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return uuid.NewString()[:16]
	}
	return hex.EncodeToString(b)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Allan-Client, X-Allan-Session")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
