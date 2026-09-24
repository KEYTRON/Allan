package backend

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// GigaChat (Sber) differs from OpenAI-compatible APIs in three ways:
//   - the stored key is an "authorization key" (base64 of client_id:client_secret)
//     exchanged at the OAuth endpoint for an access token that lives 30 minutes;
//   - servers use a certificate chain of the Russian Ministry of Digital
//     Development, which is not in the Mozilla/OS trust stores;
//   - tools are passed as legacy "functions", the model calls one function per
//     turn and results come back as role "function" messages with JSON content.

const (
	GigaChatBaseURL = "https://gigachat.devices.sberbank.ru/api/v1"
	gigaChatAuthURL = "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"
	gigaChatScope   = "GIGACHAT_API_PERS" // personal accounts; B2B/CORP use other scopes
)

// Russian Trusted Root CA from gosuslugi.ru/crt,
// SHA-256 D2:6D:2D:02:31:B7:C3:9F:92:CC:73:85:12:BA:54:10:35:19:E4:40:5D:68:B5:BD:70:3E:97:88:CA:8E:CF:31.
// It is trusted only by the GigaChat client, on top of the system roots.
//
//go:embed certs/russian_trusted_root_ca.pem
var russianRootCA []byte

type GigaChat struct {
	baseURL string
	authURL string
	authKey string
	scope   string
	model   string
	client  *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func NewGigaChat(baseURL, authKey, model string) *GigaChat {
	if baseURL == "" {
		baseURL = GigaChatBaseURL
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	pool.AppendCertsFromPEM(russianRootCA)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return &GigaChat{
		baseURL: strings.TrimRight(baseURL, "/"),
		authURL: gigaChatAuthURL,
		authKey: strings.TrimSpace(authKey),
		scope:   gigaChatScope,
		model:   model,
		client:  &http.Client{Timeout: 5 * time.Minute, Transport: transport},
	}
}

func (g *GigaChat) Name() string { return "gigachat" }

// accessToken returns a cached token, refreshing it a minute before expiry.
func (g *GigaChat) accessToken(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.token != "" && time.Until(g.expiresAt) > time.Minute {
		return g.token, nil
	}
	if g.authKey == "" {
		return "", fmt.Errorf("нет ключа авторизации GigaChat: allan key set gigachat")
	}
	form := url.Values{"scope": {g.scope}}
	req, err := http.NewRequestWithContext(ctx, "POST", g.authURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Basic "+g.authKey)
	req.Header.Set("RqUID", uuid.NewString())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gigachat oauth: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("gigachat oauth %d: %s (нужен ключ авторизации из личного кабинета, а не Client Secret)", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresAt   int64  `json:"expires_at"` // unix milliseconds
		Tok         string `json:"tok"`
		Exp         int64  `json:"exp"`
	}
	if err := json.Unmarshal(data, &tok); err != nil {
		return "", fmt.Errorf("gigachat oauth: %w", err)
	}
	if tok.AccessToken == "" {
		tok.AccessToken, tok.ExpiresAt = tok.Tok, tok.Exp
	}
	g.token = tok.AccessToken
	g.expiresAt = time.UnixMilli(tok.ExpiresAt)
	if tok.ExpiresAt < 1e12 { // seconds, not milliseconds
		g.expiresAt = time.Unix(tok.ExpiresAt, 0)
	}
	return g.token, nil
}

// do sends an authorized request, retrying once with a fresh token on 401.
func (g *GigaChat) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := g.accessToken(ctx)
		if err != nil {
			return nil, err
		}
		var rd io.Reader
		if body != nil {
			buf, err := json.Marshal(body)
			if err != nil {
				return nil, err
			}
			rd = bytes.NewReader(buf)
		}
		req, err := http.NewRequestWithContext(ctx, method, g.baseURL+path, rd)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := g.client.Do(req)
		if err != nil {
			return nil, err
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			g.mu.Lock()
			g.token = ""
			g.mu.Unlock()
			continue
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("gigachat %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
		}
		return data, nil
	}
	return nil, fmt.Errorf("gigachat: повторная авторизация не помогла")
}

type gigaFunctionCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type gigaMessage struct {
	Role             string            `json:"role"`
	Content          string            `json:"content"`
	FunctionCall     *gigaFunctionCall `json:"function_call,omitempty"`
	Name             string            `json:"name,omitempty"`
	FunctionsStateID string            `json:"functions_state_id,omitempty"`
}

type gigaFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type gigaRequest struct {
	Model        string         `json:"model"`
	Messages     []gigaMessage  `json:"messages"`
	Functions    []gigaFunction `json:"functions,omitempty"`
	FunctionCall string         `json:"function_call,omitempty"`
}

type gigaResponse struct {
	Choices []struct {
		Message      gigaMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// toGigaMessages converts history. Tool results become role "function" with a
// JSON object as content (GigaChat rejects plain text there), and several
// system messages are merged because GigaChat accepts only the first one.
func toGigaMessages(messages []Message) []gigaMessage {
	out := make([]gigaMessage, 0, len(messages))
	var system []string
	for _, m := range messages {
		switch m.Role {
		case RoleSystem:
			system = append(system, m.Content)
		case RoleTool:
			content := m.Content
			if !json.Valid([]byte(content)) || !strings.HasPrefix(strings.TrimSpace(content), "{") {
				buf, _ := json.Marshal(map[string]string{"result": content})
				content = string(buf)
			}
			out = append(out, gigaMessage{Role: "function", Name: m.Name, Content: content})
		case RoleAssistant:
			gm := gigaMessage{Role: "assistant", Content: m.Content}
			if len(m.ToolCalls) > 0 {
				tc := m.ToolCalls[0] // GigaChat makes one call per turn
				gm.FunctionCall = &gigaFunctionCall{Name: tc.Name, Arguments: tc.Arguments}
				gm.FunctionsStateID = tc.ID
			}
			out = append(out, gm)
		default:
			out = append(out, gigaMessage{Role: string(m.Role), Content: m.Content})
		}
	}
	if len(system) > 0 {
		out = append([]gigaMessage{{Role: "system", Content: strings.Join(system, "\n\n")}}, out...)
	}
	return out
}

func (g *GigaChat) Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error) {
	req := gigaRequest{Model: g.model, Messages: toGigaMessages(messages)}
	for _, t := range tools {
		req.Functions = append(req.Functions, gigaFunction{Name: t.Name, Description: t.Description, Parameters: t.Parameters})
	}
	if len(req.Functions) > 0 {
		req.FunctionCall = "auto"
	}
	data, err := g.do(ctx, "POST", "/chat/completions", req)
	if err != nil {
		return nil, err
	}
	var gr gigaResponse
	if err := json.Unmarshal(data, &gr); err != nil {
		return nil, fmt.Errorf("gigachat: %w", err)
	}
	if len(gr.Choices) == 0 {
		return nil, fmt.Errorf("gigachat: пустой ответ")
	}
	ch := gr.Choices[0]
	out := &Response{
		Content:   ch.Message.Content,
		TokensIn:  gr.Usage.PromptTokens,
		TokensOut: gr.Usage.CompletionTokens,
		Stop:      ch.FinishReason,
	}
	if fc := ch.Message.FunctionCall; fc != nil && fc.Name != "" {
		id := ch.Message.FunctionsStateID
		if id == "" {
			id = uuid.NewString()
		}
		out.ToolCalls = []ToolCall{{ID: id, Name: fc.Name, Arguments: fc.Arguments}}
	}
	return out, nil
}

// Stream is not used by the agent loop; it delivers the whole reply at once.
func (g *GigaChat) Stream(ctx context.Context, messages []Message, tools []ToolDef, out chan<- Token) error {
	defer close(out)
	resp, err := g.Chat(ctx, messages, tools)
	if err != nil {
		out <- Token{Err: err, Done: true}
		return err
	}
	if resp.Content != "" {
		out <- Token{Text: resp.Content}
	}
	for i := range resp.ToolCalls {
		out <- Token{ToolCall: &resp.ToolCalls[i]}
	}
	out <- Token{Done: true}
	return nil
}

func (g *GigaChat) Models(ctx context.Context) ([]string, error) {
	data, err := g.do(ctx, "GET", "/models", nil)
	if err != nil {
		return nil, err
	}
	var mr struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &mr); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(mr.Data))
	for _, m := range mr.Data {
		out = append(out, m.ID)
	}
	return out, nil
}

func (g *GigaChat) Health(ctx context.Context) error {
	_, err := g.Models(ctx)
	return err
}
