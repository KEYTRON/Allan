package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type VectorResult struct {
	ID       string
	Text     string
	Metadata map[string]any
	Score    float32
}

type Store interface {
	Add(ctx context.Context, collection, id, text string, metadata map[string]any) error
	Query(ctx context.Context, collection, query string, topK int) ([]VectorResult, error)
	Delete(ctx context.Context, collection, id string) error
	Healthy(ctx context.Context) bool
}

// ChromaStore implements Store via ChromaDB v2 HTTP API.
// Uses default tenant and database.
type ChromaStore struct {
	BaseURL    string
	Tenant     string
	Database   string
	Prefix     string
	client     *http.Client
	collIDs    map[string]string
}

func NewChroma(baseURL, prefix string) *ChromaStore {
	return &ChromaStore{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Tenant:   "default_tenant",
		Database: "default_database",
		Prefix:   prefix,
		client:   &http.Client{Timeout: 10 * time.Second},
		collIDs:  make(map[string]string),
	}
}

func (c *ChromaStore) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/api/v2/heartbeat", nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

func (c *ChromaStore) collName(name string) string {
	if c.Prefix == "" {
		return name
	}
	return c.Prefix + "_" + name
}

func (c *ChromaStore) collectionURL() string {
	return fmt.Sprintf("%s/api/v2/tenants/%s/databases/%s/collections", c.BaseURL, c.Tenant, c.Database)
}

func (c *ChromaStore) ensureCollection(ctx context.Context, name string) (string, error) {
	full := c.collName(name)
	if id, ok := c.collIDs[full]; ok {
		return id, nil
	}
	body, _ := json.Marshal(map[string]any{
		"name":          full,
		"get_or_create": true,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", c.collectionURL(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("chroma create collection: %d %s", resp.StatusCode, string(data))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	c.collIDs[full] = out.ID
	return out.ID, nil
}

func (c *ChromaStore) Add(ctx context.Context, collection, id, text string, metadata map[string]any) error {
	cid, err := c.ensureCollection(ctx, collection)
	if err != nil {
		return err
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	body, _ := json.Marshal(map[string]any{
		"ids":       []string{id},
		"documents": []string{text},
		"metadatas": []map[string]any{metadata},
	})
	url := fmt.Sprintf("%s/%s/add", c.collectionURL(), cid)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("chroma add: %d %s", resp.StatusCode, string(data))
	}
	return nil
}

func (c *ChromaStore) Query(ctx context.Context, collection, query string, topK int) ([]VectorResult, error) {
	cid, err := c.ensureCollection(ctx, collection)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]any{
		"query_texts": []string{query},
		"n_results":   topK,
	})
	url := fmt.Sprintf("%s/%s/query", c.collectionURL(), cid)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("chroma query: %d %s", resp.StatusCode, string(data))
	}
	var raw struct {
		IDs       [][]string         `json:"ids"`
		Documents [][]string         `json:"documents"`
		Metadatas [][]map[string]any `json:"metadatas"`
		Distances [][]float32        `json:"distances"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var out []VectorResult
	if len(raw.IDs) == 0 {
		return out, nil
	}
	for i := range raw.IDs[0] {
		r := VectorResult{ID: raw.IDs[0][i]}
		if i < len(raw.Documents[0]) {
			r.Text = raw.Documents[0][i]
		}
		if len(raw.Metadatas) > 0 && i < len(raw.Metadatas[0]) {
			r.Metadata = raw.Metadatas[0][i]
		}
		if len(raw.Distances) > 0 && i < len(raw.Distances[0]) {
			// Chroma returns distance (lower=better); convert to similarity score
			r.Score = 1 - raw.Distances[0][i]
		}
		out = append(out, r)
	}
	return out, nil
}

func (c *ChromaStore) Delete(ctx context.Context, collection, id string) error {
	cid, err := c.ensureCollection(ctx, collection)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"ids": []string{id}})
	url := fmt.Sprintf("%s/%s/delete", c.collectionURL(), cid)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// NoopStore is used when ChromaDB is unavailable.
type NoopStore struct{}

func (n *NoopStore) Add(context.Context, string, string, string, map[string]any) error {
	return nil
}
func (n *NoopStore) Query(context.Context, string, string, int) ([]VectorResult, error) {
	return nil, nil
}
func (n *NoopStore) Delete(context.Context, string, string) error { return nil }
func (n *NoopStore) Healthy(context.Context) bool                 { return false }
