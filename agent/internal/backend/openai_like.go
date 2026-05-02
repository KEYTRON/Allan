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

type OpenAILike struct {
	name    string
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenAILike(name, baseURL, apiKey, model string) *OpenAILike {
	return &OpenAILike{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

func (o *OpenAILike) Name() string { return o.name }

type oaiMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []oaiToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

type oaiToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function oaiFuncCall    `json:"function"`
}

type oaiFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaiTool struct {
	Type     string         `json:"type"`
	Function oaiFunction    `json:"function"`
}

type oaiFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type oaiRequest struct {
	Model    string       `json:"model"`
	Messages []oaiMessage `json:"messages"`
	Tools    []oaiTool    `json:"tools,omitempty"`
	Stream   bool         `json:"stream,omitempty"`
}

type oaiResponse struct {
	Choices []struct {
		Message oaiMessage `json:"message"`
		Delta   oaiMessage `json:"delta"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type oaiModelsResp struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func toOAIMessages(messages []Message) []oaiMessage {
	out := make([]oaiMessage, 0, len(messages))
	for _, m := range messages {
		om := oaiMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		if m.Role == RoleTool {
			om.Role = "tool"
		}
		for _, tc := range m.ToolCalls {
			args, _ := json.Marshal(tc.Arguments)
			om.ToolCalls = append(om.ToolCalls, oaiToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: oaiFuncCall{
					Name:      tc.Name,
					Arguments: string(args),
				},
			})
		}
		out = append(out, om)
	}
	return out
}

func toOAITools(tools []ToolDef) []oaiTool {
	out := make([]oaiTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, oaiTool{
			Type: "function",
			Function: oaiFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return out
}

func (o *OpenAILike) authHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
}

func (o *OpenAILike) Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error) {
	body := oaiRequest{
		Model:    o.model,
		Messages: toOAIMessages(messages),
		Tools:    toOAITools(tools),
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	o.authHeaders(req)
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("backend %s error %d: %s", o.name, resp.StatusCode, string(data))
	}
	var or oaiResponse
	if err := json.Unmarshal(data, &or); err != nil {
		return nil, fmt.Errorf("decode response: %w (body: %s)", err, string(data))
	}
	if len(or.Choices) == 0 {
		return &Response{}, nil
	}
	msg := or.Choices[0].Message
	out := &Response{
		Content:   msg.Content,
		TokensIn:  or.Usage.PromptTokens,
		TokensOut: or.Usage.CompletionTokens,
	}
	for _, tc := range msg.ToolCalls {
		var args map[string]any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		if args == nil {
			args = map[string]any{}
		}
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}
	return out, nil
}

func (o *OpenAILike) Stream(ctx context.Context, messages []Message, tools []ToolDef, out chan<- Token) error {
	defer close(out)
	body := oaiRequest{
		Model:    o.model,
		Messages: toOAIMessages(messages),
		Tools:    toOAITools(tools),
		Stream:   true,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	o.authHeaders(req)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := o.client.Do(req)
	if err != nil {
		out <- Token{Err: err}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("backend %s stream error %d: %s", o.name, resp.StatusCode, string(data))
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
		if payload == "[DONE]" {
			out <- Token{Done: true}
			return nil
		}
		var chunk oaiResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			out <- Token{Text: delta.Content}
		}
	}
	out <- Token{Done: true}
	return scanner.Err()
}

func (o *OpenAILike) Models(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	o.authHeaders(req)
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(data))
	}
	var mr oaiModelsResp
	if err := json.Unmarshal(data, &mr); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(mr.Data))
	for _, m := range mr.Data {
		out = append(out, m.ID)
	}
	return out, nil
}

func (o *OpenAILike) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/models", nil)
	if err != nil {
		return err
	}
	o.authHeaders(req)
	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("backend unhealthy: %d", resp.StatusCode)
	}
	return nil
}
