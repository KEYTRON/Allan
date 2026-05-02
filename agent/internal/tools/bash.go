package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type BashTool struct {
	Shell     string
	Workspace string
}

func NewBashTool(shell, workspace string) *BashTool {
	if shell == "" {
		shell = detectShell()
	}
	return &BashTool{Shell: shell, Workspace: workspace}
}

func detectShell() string {
	for _, sh := range []string{"bash", "sh"} {
		if _, err := exec.LookPath(sh); err == nil {
			return sh
		}
	}
	return "sh"
}

func (b *BashTool) Name() string { return "bash" }

func (b *BashTool) Description() string {
	return "Execute a shell command in the workspace. Returns combined stdout+stderr and exit code. Use for running scripts, file operations, git, build tools, etc."
}

func (b *BashTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds (default 30, max 600)",
			},
		},
		"required": []string{"command"},
	}
}

var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`rm\s+-rf?\s+/(\s|$)`),
	regexp.MustCompile(`rm\s+-rf?\s+/\*`),
	regexp.MustCompile(`:\(\)\{\s*:\|:\&\s*\}`),
	regexp.MustCompile(`\bdd\s+if=.*of=/dev/sd`),
	regexp.MustCompile(`mkfs\.\w+\s+/dev/`),
	regexp.MustCompile(`>\s*/dev/sd[a-z]`),
}

func IsDangerous(cmd string) bool {
	for _, p := range dangerousPatterns {
		if p.MatchString(cmd) {
			return true
		}
	}
	return false
}

func (b *BashTool) Run(ctx context.Context, params map[string]any) (string, error) {
	command, _ := params["command"].(string)
	if command == "" {
		return "", fmt.Errorf("command is required")
	}
	if IsDangerous(command) {
		return "", fmt.Errorf("refused: command matches dangerous pattern")
	}
	timeout := 30
	if t, ok := params["timeout_seconds"].(float64); ok {
		timeout = int(t)
	} else if t, ok := params["timeout_seconds"].(int); ok {
		timeout = t
	}
	if timeout <= 0 {
		timeout = 30
	}
	if timeout > 600 {
		timeout = 600
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cctx, b.Shell, "-c", command)
	if b.Workspace != "" {
		cmd.Dir = b.Workspace
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	out := strings.Builder{}
	if stdout.Len() > 0 {
		out.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if out.Len() > 0 {
			out.WriteString("\n")
		}
		out.WriteString("[stderr]\n")
		out.WriteString(stderr.String())
	}
	out.WriteString(fmt.Sprintf("\n[exit %d]", exitCode))
	if cctx.Err() == context.DeadlineExceeded {
		return out.String(), fmt.Errorf("command timed out after %ds", timeout)
	}
	if err != nil && exitCode == 0 {
		return out.String(), err
	}
	return out.String(), nil
}

// IsInteractive guesses whether a command might require interactive input.
func IsInteractive(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	for _, prefix := range []string{"sudo ", "ssh ", "vim ", "nvim ", "nano ", "htop", "top", "less ", "more ", "man ", "git rebase -i"} {
		if strings.HasPrefix(cmd, prefix) || cmd == strings.TrimSpace(prefix) {
			return true
		}
	}
	return false
}
