package backend

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Anthropic struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func NewAnthropic(apiKey, model, baseURL string) *Anthropic {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &Anthropic{
		apiKey:  apiKey,
		model:   model,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

func (a *Anthropic) Name() string { return "anthropic" }

type anthBlock struct {
	Type    string         `json:"type"`
	Text    string         `json:"text,omitempty"`
	ID      string         `json:"id,omitempty"`
	Name    string         `json:"name,omitempty"`
	Input   map[string]any `json:"input,omitempty"`
	ToolUseID string       `json:"tool_use_id,omitempty"`
	Content string         `json:"content,omitempty"`
}

type anthMessage struct {
	Role    string      `json:"role"`
	Content []anthBlock `json:"content"`
}

type anthTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthRequest struct {
	Model     string        `json:"model"`
	System    string        `json:"system,omitempty"`
	Messages  []anthMessage `json:"messages"`
	Tools     []anthTool    `json:"tools,omitempty"`
	MaxTokens int           `json:"max_tokens"`
	Stream    bool          `json:"stream,omitempty"`
}

type anthResponse struct {
	Content    []anthBlock `json:"content"`
	StopReason string      `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func toAnthropicMessages(messages []Message) (system string, msgs []anthMessage) {
	for _, m := range messages {
		switch m.Role {
		case RoleSystem:
			if system != "" {
				system += "\n\n"
			}
			system += m.Content
		case RoleUser:
			msgs = append(msgs, anthMessage{
				Role:    "user",
				Content: []anthBlock{{Type: "text", Text: m.Content}},
			})
		case RoleAssistant:
			blocks := []anthBlock{}
			if m.Content != "" {
				blocks = append(blocks, anthBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, anthBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: tc.Arguments,
				})
			}
			msgs = append(msgs, anthMessage{Role: "assistant", Content: blocks})
		case RoleTool:
			msgs = append(msgs, anthMessage{
				Role: "user",
				Content: []anthBlock{{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   m.Content,
				}},
			})
		}
	}
	return
}

func toAnthropicTools(tools []ToolDef) []anthTool {
	out := make([]anthTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, anthTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}
	return out
}

func (a *Anthropic) headers(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
}

func (a *Anthropic) Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error) {
	system, msgs := toAnthropicMessages(messages)
	body := anthRequest{
		Model:     a.model,
		System:    system,
		Messages:  msgs,
		Tools:     toAnthropicTools(tools),
		MaxTokens: 4096,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+"/v1/messages", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	a.headers(req)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("anthropic error %d: %s", resp.StatusCode, string(data))
	}
	var ar anthResponse
	if err := json.Unmarshal(data, &ar); err != nil {
		return nil, err
	}
	out := &Response{
		Stop:      ar.StopReason,
		TokensIn:  ar.Usage.InputTokens,
		TokensOut: ar.Usage.OutputTokens,
	}
	for _, block := range ar.Content {
		switch block.Type {
		case "text":
			out.Content += block.Text
		case "tool_use":
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			})
		}
	}
	return out, nil
}

type anthStreamEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
	ContentBlock anthBlock `json:"content_block"`
}

func (a *Anthropic) Stream(ctx context.Context, messages []Message, tools []ToolDef, out chan<- Token) error {
	defer close(out)
	system, msgs := toAnthropicMessages(messages)
	body := anthRequest{
		Model:     a.model,
		System:    system,
		Messages:  msgs,
		Tools:     toAnthropicTools(tools),
		MaxTokens: 4096,
		Stream:    true,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+"/v1/messages", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	a.headers(req)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := a.client.Do(req)
	if err != nil {
		out <- Token{Err: err}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("anthropic stream error %d: %s", resp.StatusCode, string(data))
		out <- Token{Err: err}
		return err
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		var ev anthStreamEvent
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				out <- Token{Text: ev.Delta.Text}
			}
		case "message_stop":
			out <- Token{Done: true}
			return nil
		}
	}
	out <- Token{Done: true}
	return scanner.Err()
}

func (a *Anthropic) Models(ctx context.Context) ([]string, error) {
	return []string{
		"claude-opus-4-7",
		"claude-sonnet-4-6",
		"claude-haiku-4-5-20251001",
		"claude-3-5-sonnet-latest",
		"claude-3-5-haiku-latest",
	}, nil
}

func (a *Anthropic) Health(ctx context.Context) error {
	if a.apiKey == "" {
		return fmt.Errorf("anthropic api key not set")
	}
	return nil
}
