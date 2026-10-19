package serve

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A fake Ollama-on-another-PC endpoint configured as a custom provider must
// be listed and proxied with this machine's key, never with the worker token.
func TestLLMCatalogAndProxy(t *testing.T) {
	var gotAuth, gotPath, gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Write([]byte(`{"data":[{"id":"qwen3:8b"},{"id":"gemma3:4b"}]}`))
		default:
			gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"))
		}
	}))
	defer upstream.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ALLAN_SECRETS", "file")
	dir := filepath.Join(home, ".allan")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`
[backend]
  type = "box"
  model = "qwen3:8b"
[providers.box]
  type = "openai"
  base_url = "`+upstream.URL+`/v1"
[providers.openrouter]
  type = "openai"
  base_url = "https://openrouter.ai/api/v1"
[providers.keytron]
  type = "openai"
  base_url = "https://keytron-prime.org/api/ai/v1"
`), 0o600)
	os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(`{"box":"box-key"}`), 0o600)

	srv, err := New(Options{Token: "worker-token"})
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()

	req := httptest.NewRequest("GET", "/v1/llm", nil)
	req.Header.Set("Authorization", "Bearer worker-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var cat struct {
		Providers []struct {
			Provider string
			Models   []string
		}
	}
	json.Unmarshal(rec.Body.Bytes(), &cat)
	names := map[string][]string{}
	for _, p := range cat.Providers {
		names[p.Provider] = p.Models
	}
	if got := names["box"]; len(got) != 2 {
		t.Fatalf("box models = %v (body %s)", got, rec.Body.String())
	}
	if _, ok := names["openrouter"]; ok {
		t.Error("public cloud providers must not be proxied")
	}
	if _, ok := names["keytron"]; ok {
		t.Error("the site itself must not be offered back to the site")
	}

	req = httptest.NewRequest("POST", "/v1/llm/box/chat/completions", strings.NewReader(`{"model":"qwen3:8b","stream":true}`))
	req.Header.Set("Authorization", "Bearer worker-token")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"hi"`) {
		t.Fatalf("proxy: %d %s", rec.Code, rec.Body.String())
	}
	if gotAuth != "Bearer box-key" || gotPath != "/v1/chat/completions" || !strings.Contains(gotBody, "qwen3:8b") {
		t.Errorf("upstream saw auth=%q path=%q body=%q", gotAuth, gotPath, gotBody)
	}

	req = httptest.NewRequest("POST", "/v1/llm/box/chat/completions", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: %d", rec.Code)
	}
}
