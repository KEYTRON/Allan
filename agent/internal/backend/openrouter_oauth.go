package backend

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// OpenRouter OAuth PKCE: the user signs in on openrouter.ai and Allan gets a
// key for their account without copying it from the dashboard.
var (
	OpenRouterAuthURL     = "https://openrouter.ai/auth"
	OpenRouterExchangeURL = "https://openrouter.ai/api/v1/auth/keys"
)

// PKCE is one sign-in attempt: the verifier stays here, the challenge goes to
// the browser.
type PKCE struct {
	Verifier  string
	Challenge string
}

func NewPKCE() (PKCE, error) {
	buf := make([]byte, 48)
	if _, err := rand.Read(buf); err != nil {
		return PKCE{}, err
	}
	v := base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(v))
	return PKCE{Verifier: v, Challenge: base64.RawURLEncoding.EncodeToString(sum[:])}, nil
}

// OpenRouterAuthLink builds the sign-in page address. Without a callback
// OpenRouter shows the code on the page for the user to paste (headless mode).
func OpenRouterAuthLink(p PKCE, callback, label string) string {
	q := url.Values{}
	if callback != "" {
		q.Set("callback_url", callback)
	}
	q.Set("code_challenge", p.Challenge)
	q.Set("code_challenge_method", "S256")
	if label != "" {
		q.Set("key_label", label)
	}
	return OpenRouterAuthURL + "?" + q.Encode()
}

// CodeFromInput accepts either the bare code or the whole address OpenRouter
// redirected to (pasted when the browser could not reach this machine).
func CodeFromInput(s string) string {
	s = strings.TrimSpace(s)
	if u, err := url.Parse(s); err == nil && u.Scheme != "" {
		return u.Query().Get("code")
	}
	return s
}

// ExchangeOpenRouterCode trades the one-time code for an API key.
func ExchangeOpenRouterCode(ctx context.Context, code string, p PKCE) (string, error) {
	body, _ := json.Marshal(map[string]string{"code": code, "code_verifier": p.Verifier, "code_challenge_method": "S256"})
	req, err := http.NewRequestWithContext(ctx, "POST", OpenRouterExchangeURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Key   string `json:"key"`
		Error any    `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 400 || out.Key == "" {
		return "", fmt.Errorf("OpenRouter: HTTP %d %v", resp.StatusCode, out.Error)
	}
	return out.Key, nil
}
