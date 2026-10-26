package backend

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestOpenRouterPKCEFlow(t *testing.T) {
	p, err := NewPKCE()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(p.Verifier))
	if p.Challenge != base64.RawURLEncoding.EncodeToString(sum[:]) {
		t.Fatal("challenge is not S256 of the verifier")
	}

	link, _ := url.Parse(OpenRouterAuthLink(p, "http://localhost:3000/callback", "Allan pc"))
	q := link.Query()
	if q.Get("code_challenge") != p.Challenge || q.Get("code_challenge_method") != "S256" ||
		q.Get("callback_url") != "http://localhost:3000/callback" || q.Get("key_label") != "Allan pc" {
		t.Fatalf("auth link: %s", link)
	}
	if headless, _ := url.Parse(OpenRouterAuthLink(p, "", "")); headless.Query().Has("callback_url") {
		t.Fatal("headless link must not carry a callback")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["code"] != "c1" || in["code_verifier"] != p.Verifier || in["code_challenge_method"] != "S256" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"message":"bad code"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"key":"sk-or-v1-test"}`))
	}))
	defer srv.Close()
	old := OpenRouterExchangeURL
	OpenRouterExchangeURL = srv.URL
	defer func() { OpenRouterExchangeURL = old }()

	if key, err := ExchangeOpenRouterCode(context.Background(), "c1", p); err != nil || key != "sk-or-v1-test" {
		t.Fatalf("exchange: %q %v", key, err)
	}
	if _, err := ExchangeOpenRouterCode(context.Background(), "wrong", p); err == nil {
		t.Fatal("a refused code must be an error")
	}
}

func TestCodeFromInput(t *testing.T) {
	for in, want := range map[string]string{
		"  abc123 ": "abc123",
		"http://localhost:3000/callback?code=xyz": "xyz",
	} {
		if got := CodeFromInput(in); got != want {
			t.Errorf("CodeFromInput(%q) = %q, want %q", in, got, want)
		}
	}
}
