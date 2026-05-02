package pty

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	gopty "github.com/aymanbagabas/go-pty"
)

type Session struct {
	pty        gopty.Pty
	cmd        *gopty.Cmd
	output     []byte
	mu         sync.Mutex
	done       chan struct{}
	patterns   []string
	lastChange time.Time
	command    string
	closed     bool
}

func DefaultPatterns() []string {
	return []string{
		"password", "Password", "пароль",
		"(yes/no)", "Passphrase", "[Y/n]", "[y/N]",
		"Press any key", "Enter ",
	}
}

func Start(ctx context.Context, shell, command string, patterns []string) (*Session, error) {
	pty, err := gopty.New()
	if err != nil {
		return nil, err
	}
	cmd := pty.CommandContext(ctx, shell, "-c", command)
	if err := cmd.Start(); err != nil {
		pty.Close()
		return nil, err
	}
	if patterns == nil {
		patterns = DefaultPatterns()
	}
	s := &Session{
		pty:        pty,
		cmd:        cmd,
		done:       make(chan struct{}),
		patterns:   patterns,
		lastChange: time.Now(),
		command:    command,
	}
	go s.readLoop()
	go s.waitLoop()
	return s, nil
}

func (s *Session) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.output = append(s.output, buf[:n]...)
			s.lastChange = time.Now()
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) waitLoop() {
	_ = s.cmd.Wait()
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	close(s.done)
}

func (s *Session) Output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.output)
}

func (s *Session) Write(p []byte) (int, error) {
	return s.pty.Write(p)
}

func (s *Session) Resize(rows, cols uint16) error {
	return s.pty.Resize(int(cols), int(rows))
}

func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	return s.pty.Close()
}

func (s *Session) Done() <-chan struct{} { return s.done }

// IsWaitingForInput returns true if PTY hasn't produced new output for >200ms
// AND the last line matches one of the interactive patterns.
func (s *Session) IsWaitingForInput() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.lastChange) < 200*time.Millisecond {
		return false
	}
	last := lastNonEmptyLine(s.output)
	low := strings.ToLower(last)
	for _, p := range s.patterns {
		if strings.Contains(low, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

func lastNonEmptyLine(data []byte) string {
	s := string(data)
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimRight(lines[i], "\r\n ")
		if l != "" {
			return l
		}
	}
	return ""
}

// CopyTo copies output to a writer. Used for fullscreen mode.
func (s *Session) CopyTo(w io.Writer) {
	s.mu.Lock()
	out := make([]byte, len(s.output))
	copy(out, s.output)
	s.mu.Unlock()
	_, _ = w.Write(out)
}

func IsFullscreenApp(command string) bool {
	command = strings.TrimSpace(command)
	apps := []string{"vim", "nvim", "nano", "htop", "top", "less", "more", "man", "ncdu", "tmux", "screen"}
	for _, a := range apps {
		if command == a || strings.HasPrefix(command, a+" ") {
			return true
		}
	}
	return false
}
