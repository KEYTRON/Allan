package backend

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGigaChatAuthAndFunctionCall(t *testing.T) {
	var oauthCalls, chatCalls int
	var lastReq gigaRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth":
			oauthCalls++
			if r.Header.Get("Authorization") != "Basic a2V5" || r.Header.Get("RqUID") == "" {
				t.Errorf("oauth headers: %v", r.Header)
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != "scope=GIGACHAT_API_PERS" {
				t.Errorf("oauth body %q", body)
			}
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "tok" + strings.Repeat("x", oauthCalls),
				"expires_at":   time.Now().Add(30 * time.Minute).UnixMilli(),
			})
		case "/chat/completions":
			chatCalls++
			if chatCalls == 1 { // first call: pretend the token expired
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.Header.Get("Authorization") != "Bearer tokxx" {
				t.Errorf("chat auth %q", r.Header.Get("Authorization"))
			}
			json.NewDecoder(r.Body).Decode(&lastReq)
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"","function_call":{"name":"bash","arguments":{"command":"ls"}},"functions_state_id":"fs-1"},"finish_reason":"function_call"}],"usage":{"prompt_tokens":10,"completion_tokens":3}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	g := NewGigaChat(srv.URL, "a2V5", "GigaChat-2")
	g.authURL = srv.URL + "/oauth"
	resp, err := g.Chat(context.Background(), []Message{
		{Role: RoleSystem, Content: "sys1"},
		{Role: RoleSystem, Content: "sys2"},
		{Role: RoleUser, Content: "list files"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "fs-0", Name: "read_file", Arguments: map[string]any{"path": "a"}}}},
		{Role: RoleTool, Name: "read_file", Content: "plain text result"},
	}, []ToolDef{{Name: "bash", Description: "run", Parameters: map[string]any{"type": "object"}}})
	if err != nil {
		t.Fatal(err)
	}
	if oauthCalls != 2 || chatCalls != 2 {
		t.Fatalf("oauth=%d chat=%d, want token refresh after 401", oauthCalls, chatCalls)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "bash" || resp.ToolCalls[0].ID != "fs-1" || resp.ToolCalls[0].Arguments["command"] != "ls" {
		t.Fatalf("tool call: %+v", resp.ToolCalls)
	}
	if resp.TokensIn != 10 || resp.TokensOut != 3 {
		t.Fatalf("usage %d/%d", resp.TokensIn, resp.TokensOut)
	}

	msgs := lastReq.Messages
	if msgs[0].Role != "system" || msgs[0].Content != "sys1\n\nsys2" || msgs[1].Role != "user" {
		t.Fatalf("system merge: %+v", msgs[:2])
	}
	if fc := msgs[2].FunctionCall; fc == nil || fc.Name != "read_file" || msgs[2].FunctionsStateID != "fs-0" {
		t.Fatalf("assistant function_call: %+v", msgs[2])
	}
	if msgs[3].Role != "function" || msgs[3].Name != "read_file" || msgs[3].Content != `{"result":"plain text result"}` {
		t.Fatalf("function result must be a JSON object: %+v", msgs[3])
	}
	if lastReq.FunctionCall != "auto" || len(lastReq.Functions) != 1 {
		t.Fatalf("functions: %+v", lastReq)
	}

	// The cached token is reused: no new OAuth call.
	if _, err := g.accessToken(context.Background()); err != nil || oauthCalls != 2 {
		t.Fatalf("token cache: calls=%d err=%v", oauthCalls, err)
	}
}

func TestGigaChatTrustsRussianRootCA(t *testing.T) {
	g := NewGigaChat("", "", "")
	tr := g.client.Transport.(*http.Transport)
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.RootCAs == nil {
		t.Fatal("no custom root pool")
	}
	if !strings.Contains(string(russianRootCA), "BEGIN CERTIFICATE") {
		t.Fatal("embedded CA missing")
	}
}
