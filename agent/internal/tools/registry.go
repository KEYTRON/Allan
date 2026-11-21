package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/keytron/allan/agent/internal/backend"
)

type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Run(ctx context.Context, params map[string]any) (string, error)
}

// Grouped — инструмент из группы (MCP-сервер): по группам строится каталог
// отложенных инструментов.
type Grouped interface {
	Group() string
	// GroupNote — одна строка о группе в целом (может быть пустой).
	GroupNote() string
}

// LoadPolicy — когда схема инструмента уходит модели.
type LoadPolicy int

const (
	// LoadAlways — в каждом запросе (встроенные инструменты).
	LoadAlways LoadPolicy = iota
	// LoadAuto — сразу, пока все такие инструменты вместе укладываются в
	// порог; иначе откладываются до load_tools.
	LoadAuto
	// LoadDeferred — только после load_tools (или первого прямого вызова).
	LoadDeferred
)

// DefaultAutoTokens — порог для LoadAuto, в оценочных токенах схем.
const DefaultAutoTokens = 4000

type Registry struct {
	mu     sync.RWMutex
	tools  map[string]Tool
	policy map[string]LoadPolicy
	loaded map[string]bool
	// AutoTokens — порог для LoadAuto; 0 — DefaultAutoTokens, <0 — всегда сразу.
	AutoTokens int
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}, policy: map[string]LoadPolicy{}, loaded: map[string]bool{}}
}

func (r *Registry) Register(t Tool) { r.RegisterWith(t, LoadAlways) }

func (r *Registry) RegisterWith(t Tool, p LoadPolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
	r.policy[t.Name()] = p
}

// Unregister убирает инструменты (например, выключенного MCP-сервера).
func (r *Registry) Unregister(names ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range names {
		delete(r.tools, n)
		delete(r.policy, n)
		delete(r.loaded, n)
	}
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// EstimateTokens — грубая оценка, сколько токенов модели стоит определение
// инструмента (≈4 символа JSON на токен).
func EstimateTokens(t Tool) int {
	b, _ := json.Marshal(map[string]any{"name": t.Name(), "description": t.Description(), "parameters": t.Schema()})
	return len(b)/4 + 1
}

// split делит инструменты на отданные модели сейчас и отложенные.
func (r *Registry) split() (active, deferred []Tool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	limit := r.AutoTokens
	if limit == 0 {
		limit = DefaultAutoTokens
	}
	autoTotal := 0
	for name, t := range r.tools {
		if r.policy[name] == LoadAuto && !r.loaded[name] {
			autoTotal += EstimateTokens(t)
		}
	}
	autoFits := limit < 0 || autoTotal <= limit
	for name, t := range r.tools {
		switch {
		case r.policy[name] == LoadAlways, r.loaded[name],
			r.policy[name] == LoadAuto && autoFits:
			active = append(active, t)
		default:
			deferred = append(deferred, t)
		}
	}
	byName := func(s []Tool) {
		sort.Slice(s, func(i, j int) bool { return s[i].Name() < s[j].Name() })
	}
	byName(active)
	byName(deferred)
	return active, deferred
}

// Deferred — инструменты, которых модель сейчас не видит.
func (r *Registry) Deferred() []Tool {
	_, d := r.split()
	return d
}

// IsActive сообщает, видит ли модель схему инструмента сейчас.
func (r *Registry) IsActive(name string) bool {
	for _, t := range r.Deferred() {
		if t.Name() == name {
			return false
		}
	}
	_, ok := r.Get(name)
	return ok
}

// ToolDefs — определения для запроса к модели: активные инструменты и, если
// что-то отложено, load_tools с каталогом отложенного. Вызывать на каждом
// шаге: после load_tools набор меняется.
func (r *Registry) ToolDefs() []backend.ToolDef {
	active, deferred := r.split()
	defs := make([]backend.ToolDef, 0, len(active)+1)
	for _, t := range active {
		defs = append(defs, backend.ToolDef{Name: t.Name(), Description: t.Description(), Parameters: t.Schema()})
	}
	if len(deferred) > 0 {
		defs = append(defs, loaderDef(deferred))
	}
	return defs
}

// MarkLoaded делает отложенные инструменты видимыми до ResetLoaded.
func (r *Registry) MarkLoaded(names ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range names {
		if _, ok := r.tools[n]; ok {
			r.loaded[n] = true
		}
	}
}

// ResetLoaded снова прячет подгруженное (новый разговор, /clear).
func (r *Registry) ResetLoaded() {
	r.mu.Lock()
	r.loaded = map[string]bool{}
	r.mu.Unlock()
}

func (r *Registry) Run(ctx context.Context, name string, params map[string]any) (string, error) {
	if name == LoaderName {
		return r.runLoader(params)
	}
	t, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("tool %s not found", name)
	}
	// модель знала имя из каталога и позвала напрямую — схема ей больше не
	// секрет, пусть дальше видит её
	r.MarkLoaded(name)
	return t.Run(ctx, params)
}
