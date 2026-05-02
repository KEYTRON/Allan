package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ReadFileTool struct {
	Workspace string
	OnRead    func(path, content string)
}

func NewReadFileTool(workspace string) *ReadFileTool {
	return &ReadFileTool{Workspace: workspace}
}

func (r *ReadFileTool) Name() string { return "read_file" }

func (r *ReadFileTool) Description() string {
	return "Read a file from the filesystem. Returns the content with line numbers. For large files, supports start_line/end_line ranges."
}

func (r *ReadFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":       map[string]any{"type": "string", "description": "File path (absolute or relative to workspace)"},
			"start_line": map[string]any{"type": "integer", "description": "First line to read (1-indexed)"},
			"end_line":   map[string]any{"type": "integer", "description": "Last line to read (inclusive)"},
		},
		"required": []string{"path"},
	}
}

func (r *ReadFileTool) resolve(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(r.Workspace, p)
}

const maxFileSize = 50 * 1024

func (r *ReadFileTool) Run(ctx context.Context, params map[string]any) (string, error) {
	path, _ := params["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	full := r.resolve(path)
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	startLine := 1
	endLine := len(lines)
	if v, ok := params["start_line"].(float64); ok {
		startLine = int(v)
	}
	if v, ok := params["end_line"].(float64); ok {
		endLine = int(v)
	}
	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine > endLine {
		return "", fmt.Errorf("start_line > end_line")
	}

	truncated := false
	if info.Size() > maxFileSize && (params["start_line"] == nil && params["end_line"] == nil) {
		// Return only first N lines
		approxLines := 0
		var size int64
		for i, l := range lines {
			size += int64(len(l)) + 1
			if size > maxFileSize {
				approxLines = i
				break
			}
		}
		if approxLines > 0 {
			endLine = approxLines
			truncated = true
		}
	}

	var sb strings.Builder
	if truncated {
		sb.WriteString(fmt.Sprintf("[warn] file %s is %d bytes; showing first %d of %d lines\n", path, info.Size(), endLine, len(lines)))
	}
	for i := startLine - 1; i < endLine && i < len(lines); i++ {
		sb.WriteString(fmt.Sprintf("%5d  %s\n", i+1, lines[i]))
	}
	out := sb.String()
	if r.OnRead != nil {
		r.OnRead(full, string(data))
	}
	return out, nil
}
