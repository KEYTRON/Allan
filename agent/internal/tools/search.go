package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SearchTool struct {
	UserAgent string
	client    *http.Client
}

func NewSearchTool() *SearchTool {
	return &SearchTool{
		UserAgent: "Allan/0.1 (+https://github.com/keytron/allan)",
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *SearchTool) Name() string { return "search" }

func (s *SearchTool) Description() string {
	return "Search the web via DuckDuckGo Instant Answer API. Returns a list of results with title, url and snippet."
}

func (s *SearchTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":       map[string]any{"type": "string", "description": "Search query"},
			"max_results": map[string]any{"type": "integer", "description": "Max results (default 5)"},
		},
		"required": []string{"query"},
	}
}

type ddgResp struct {
	AbstractText string `json:"AbstractText"`
	AbstractURL  string `json:"AbstractURL"`
	Heading      string `json:"Heading"`
	Answer       string `json:"Answer"`
	RelatedTopics []struct {
		FirstURL string `json:"FirstURL"`
		Text     string `json:"Text"`
	} `json:"RelatedTopics"`
}

func (s *SearchTool) Run(ctx context.Context, params map[string]any) (string, error) {
	query, _ := params["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	max := 5
	if v, ok := params["max_results"].(float64); ok {
		max = int(v)
	}
	if max <= 0 {
		max = 5
	}

	u := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_redirect=1&no_html=1", url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", s.UserAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Sprintf("[warn] search unavailable: %v", err), nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Sprintf("[warn] search returned status %d", resp.StatusCode), nil
	}
	var dr ddgResp
	if err := json.Unmarshal(body, &dr); err != nil {
		return fmt.Sprintf("[warn] failed to parse search response: %v", err), nil
	}

	type result struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Snippet string `json:"snippet"`
	}
	results := []result{}
	if dr.AbstractText != "" {
		results = append(results, result{
			Title:   dr.Heading,
			URL:     dr.AbstractURL,
			Snippet: dr.AbstractText,
		})
	}
	if dr.Answer != "" {
		results = append(results, result{
			Title:   "Answer",
			URL:     "",
			Snippet: dr.Answer,
		})
	}
	for _, rt := range dr.RelatedTopics {
		if len(results) >= max {
			break
		}
		if rt.FirstURL == "" {
			continue
		}
		title := rt.Text
		if i := strings.Index(rt.Text, " - "); i > 0 {
			title = rt.Text[:i]
		}
		results = append(results, result{
			Title:   title,
			URL:     rt.FirstURL,
			Snippet: rt.Text,
		})
	}
	if len(results) == 0 {
		return fmt.Sprintf("[warn] no results found for: %s", query), nil
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	return string(out), nil
}
