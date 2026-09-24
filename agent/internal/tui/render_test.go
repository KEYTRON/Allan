package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Cyrillic is two bytes per letter: wrapping must count display cells and
// never split a letter.
func TestWrapCyrillic(t *testing.T) {
	text := strings.Repeat("проверка переноса строк ", 10)
	out := wrap(text, 30)
	for _, line := range strings.Split(out, "\n") {
		if !utf8.ValidString(line) {
			t.Fatalf("broken UTF-8 in %q", line)
		}
		if w := lipgloss.Width(line); w > 30 {
			t.Fatalf("line width %d > 30: %q", w, line)
		}
	}
	if lines := strings.Count(out, "\n") + 1; lines > 10 {
		t.Fatalf("wrapped into %d lines, expected 10 (one 23-cell phrase per line)", lines)
	}
}

func TestToolArgsSummary(t *testing.T) {
	if got := toolArgsSummary(`{"command":"ls   -la\n/tmp"}`); got != "ls -la /tmp" {
		t.Fatalf("single arg: %q", got)
	}
	if got := toolArgsSummary(`{"path":"a.txt","content":"x"}`); got != "content=x, path=a.txt" {
		t.Fatalf("several args: %q", got)
	}
	if got := toolArgsSummary(`{}`); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

func TestMouseGarbageFilter(t *testing.T) {
	for _, s := range []string{"[<65;72;16M", "<64;52;17M", "65;52;17M", "[<65;72;16M[<65;52;17M", "[<0;10;5m"} {
		if !isMouseGarbage(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}) {
			t.Errorf("%q should be dropped", s)
		}
	}
	for _, s := range []string{"привет", "M", "1;2", "ls -la", "/model 4"} {
		if isMouseGarbage(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}) {
			t.Errorf("%q is real input", s)
		}
	}
}

func TestHumanError(t *testing.T) {
	in := `backend error: backend ollama error 402: {"error":{"message":"402 Payment Required: not free","type":"api_error"}}`
	want := "backend ollama error 402: 402 Payment Required: not free"
	if got := humanError(in); got != want {
		t.Fatalf("got %q", got)
	}
	if got := humanError("plain"); got != "plain" {
		t.Fatalf("got %q", got)
	}
}
