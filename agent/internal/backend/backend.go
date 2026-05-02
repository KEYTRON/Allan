package backend

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/keytron/allan/agent/config"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role        `json:"role"`
	Content    string      `json:"content"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	Name       string      `json:"name,omitempty"`
}

type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type Response struct {
	Content   string
	ToolCalls []ToolCall
	TokensIn  int
	TokensOut int
	Stop      string
}

type Token struct {
	Text     string
	ToolCall *ToolCall
	Done     bool
	Err      error
}

type Backend interface {
	Name() string
	Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error)
	Stream(ctx context.Context, messages []Message, tools []ToolDef, out chan<- Token) error
	Models(ctx context.Context) ([]string, error)
	Health(ctx context.Context) error
}

func New(cfg *config.Config) (Backend, error) {
	switch cfg.Backend.Type {
	case "anthropic":
		return NewAnthropic(cfg.Backend.APIKey, cfg.Backend.Model, cfg.Backend.BaseURL), nil
	case "openai":
		base := cfg.Backend.BaseURL
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		return NewOpenAILike("openai", base, cfg.Backend.APIKey, cfg.Backend.Model), nil
	case "ollama":
		base := cfg.Backend.BaseURL
		if base == "" {
			base = "http://localhost:11434/v1"
		}
		return NewOpenAILike("ollama", base, "", cfg.Backend.Model), nil
	case "llamacpp":
		base := cfg.Backend.BaseURL
		if base == "" {
			base = "http://localhost:8080/v1"
		}
		return NewOpenAILike("llamacpp", base, "", cfg.Backend.Model), nil
	case "lmstudio":
		base := cfg.Backend.BaseURL
		if base == "" {
			base = "http://localhost:1234/v1"
		}
		return NewOpenAILike("lmstudio", base, "", cfg.Backend.Model), nil
	default:
		return nil, fmt.Errorf("unknown backend type: %s", cfg.Backend.Type)
	}
}

// Detect tries to find an available local backend.
// Returns the type name on success, or empty string if none.
func Detect(ctx context.Context) string {
	candidates := []struct {
		name string
		url  string
	}{
		{"ollama", "http://localhost:11434/v1/models"},
		{"llamacpp", "http://localhost:8080/v1/models"},
		{"lmstudio", "http://localhost:1234/v1/models"},
	}
	client := &http.Client{Timeout: 600 * time.Millisecond}
	for _, c := range candidates {
		req, err := http.NewRequestWithContext(ctx, "GET", c.url, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return c.name
			}
		}
	}
	return ""
}
