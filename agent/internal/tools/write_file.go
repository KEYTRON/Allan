package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type WriteFileTool struct {
	Workspace string
}

func NewWriteFileTool(workspace string) *WriteFileTool {
	return &WriteFileTool{Workspace: workspace}
}

func (w *WriteFileTool) Name() string { return "write_file" }

func (w *WriteFileTool) Description() string {
	return "Write content to a file. Modes: 'create' (fail if exists), 'overwrite' (replace), 'append'. Creates intermediate directories automatically."
}

func (w *WriteFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path"},
			"content": map[string]any{"type": "string", "description": "Content to write"},
			"mode":    map[string]any{"type": "string", "enum": []string{"create", "overwrite", "append"}, "description": "Write mode"},
		},
		"required": []string{"path", "content"},
	}
}

func (w *WriteFileTool) resolve(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(w.Workspace, p)
}

func (w *WriteFileTool) Run(ctx context.Context, params map[string]any) (string, error) {
	path, _ := params["path"].(string)
	content, _ := params["content"].(string)
	mode, _ := params["mode"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if mode == "" {
		mode = "overwrite"
	}
	full := w.resolve(path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	switch mode {
	case "create":
		if _, err := os.Stat(full); err == nil {
			return "", fmt.Errorf("file %s already exists (mode=create)", path)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return "", err
		}
	case "overwrite":
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return "", err
		}
	case "append":
		f, err := os.OpenFile(full, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return "", err
		}
		defer f.Close()
		if _, err := f.WriteString(content); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("invalid mode: %s", mode)
	}
	return fmt.Sprintf("wrote %d bytes to %s (mode=%s)", len(content), path, mode), nil
}
