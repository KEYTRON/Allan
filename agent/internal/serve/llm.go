package serve

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/backend"
)

// The site can use this machine's own models (Ollama, llama.cpp, LM Studio,
// custom OpenAI-compatible endpoints such as Ollama on another PC) as plain
// chat models, not only through the agent. GET /v1/llm lists them;
// /v1/llm/<provider>/<path> is a reverse proxy to that provider's OpenAI API
// with the key from this machine, so the key never leaves it.
//
// Public clouds are left out on purpose: the site reaches those with the
// user's own BYOK keys anyway, and proxying them here would only add a hop.

type llmTarget struct {
	name, typ, base, key string
	local                bool
}

// openAICompatible are backend types that speak /chat/completions.
var openAICompatible = map[string]bool{"openai": true, "ollama": true, "llamacpp": true, "lmstudio": true}

func llmTargets(cfg *config.Config) []llmTarget {
	seen := map[string]bool{}
	var out []llmTarget
	add := func(name string) {
		name = backend.NormalizeProvider(name)
		if name == "" || seen[name] || name == "keytron" { // keytron is the site itself: no loops
			return
		}
		seen[name] = true
		t := llmTarget{name: name, typ: backend.ProviderType(name), base: backend.DefaultBaseURL(name), local: backend.IsLocalProvider(name)}
		if p, ok := cfg.Providers[name]; ok {
			if p.Type != "" {
				t.typ = p.Type
			}
			if p.BaseURL != "" {
				t.base = p.BaseURL
			}
			t.key = p.APIKey
		}
		known := backend.IsKnownProvider(name)
		if !openAICompatible[t.typ] || t.base == "" || (known && !t.local) {
			return
		}
		out = append(out, t)
	}
	for name := range cfg.Providers {
		add(name)
	}
	for _, name := range []string{"ollama", "llamacpp", "lmstudio"} {
		add(name)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func (s *Server) handleLLM(w http.ResponseWriter, r *http.Request) {
	// only the config is needed here: a full agent runtime per request would be waste
	cfg, _, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/llm")
	if rest == "" || rest == "/" {
		s.llmCatalog(w, r, cfg)
		return
	}
	name, path, _ := strings.Cut(strings.TrimPrefix(rest, "/"), "/")
	var target *llmTarget
	for _, t := range llmTargets(cfg) {
		if t.name == backend.NormalizeProvider(name) {
			target = &t
			break
		}
	}
	if target == nil {
		writeErr(w, http.StatusNotFound, "нет такой локальной модели-провайдера: "+name)
		return
	}
	base, err := url.Parse(strings.TrimRight(target.base, "/"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme, pr.Out.URL.Host = base.Scheme, base.Host
			pr.Out.URL.Path = base.Path + "/" + path
			pr.Out.URL.RawQuery = r.URL.RawQuery
			pr.Out.Host = base.Host
			pr.Out.Header.Del("Authorization") // the worker token stays here
			pr.Out.Header.Del("X-Allan-Client")
			pr.Out.Header.Del("Cookie")
			if target.key != "" {
				pr.Out.Header.Set("Authorization", "Bearer "+target.key)
			}
		},
		FlushInterval: -1, // streamed completions go out chunk by chunk
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeErr(w, http.StatusBadGateway, target.name+": "+err.Error())
		},
	}
	proxy.ServeHTTP(w, r)
}

func (s *Server) llmCatalog(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
	targets := llmTargets(cfg)
	type entry struct {
		Provider string   `json:"provider"`
		Type     string   `json:"type"`
		Local    bool     `json:"local"`
		Models   []string `json:"models"`
		Error    string   `json:"error,omitempty"`
	}
	out := make([]entry, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t llmTarget) {
			defer wg.Done()
			out[i] = entry{Provider: t.name, Type: t.typ, Local: t.local}
			ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
			defer cancel()
			be, err := backend.ForProvider(t.name, cfg, "")
			if err == nil {
				out[i].Models, err = be.Models(ctx)
			}
			if err != nil {
				out[i].Error = err.Error()
			}
		}(i, t)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"providers": out})
}
