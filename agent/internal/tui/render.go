package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type MsgKind int

const (
	MsgYou MsgKind = iota
	MsgAllan
	MsgTool
	MsgSys
	MsgWarn
	MsgError
	MsgToolBlock
)

type Message struct {
	Kind   MsgKind
	Text   string
	Tool   string
	Args   string
	Status string // "running" | "done" | "error"
}

func badge(k MsgKind) string {
	switch k {
	case MsgYou:
		return StyleBadgeYou.Render("[you]")
	case MsgAllan:
		return StyleBadgeAllan.Render("[allan]")
	case MsgTool:
		return StyleBadgeTool.Render("[tool]")
	case MsgSys:
		return StyleBadgeSys.Render("[sys]")
	case MsgWarn:
		return StyleBadgeWarn.Render("[warn]")
	case MsgError:
		return StyleBadgeError.Render("[error]")
	}
	return ""
}

func renderMessage(m Message, width int) string {
	switch m.Kind {
	case MsgToolBlock:
		return renderToolBlock(m, width)
	default:
		b := badge(m.Kind)
		return wordWrap(b+" "+m.Text, width)
	}
}

func renderToolBlock(m Message, width int) string {
	var icon, status string
	switch m.Status {
	case "done":
		icon = "⚙"
		status = lipgloss.NewStyle().Foreground(ColorGreen).Render("✓ done")
	case "error":
		icon = "⚙"
		status = lipgloss.NewStyle().Foreground(ColorRed).Render("✗ error")
	default:
		icon = "⚙"
		status = lipgloss.NewStyle().Foreground(ColorYellow).Render("… running")
	}
	header := fmt.Sprintf("%s %s  %s", icon, m.Tool, status)
	body := strings.TrimRight(m.Text, "\n")
	if body == "" {
		body = "(no output)"
	}
	if m.Args != "" {
		header += "\n  " + lipgloss.NewStyle().Foreground(ColorMuted).Render("args: "+truncate(m.Args, width-10))
	}
	bodyLimited := limitLines(body, 12, width-4)
	full := header + "\n" + lipgloss.NewStyle().Foreground(ColorMuted).Render("output:") + "\n" + bodyLimited
	return StyleToolBlock.Render(full)
}

func wordWrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	var sb strings.Builder
	for _, line := range strings.Split(s, "\n") {
		for len(line) > width {
			cut := width
			if i := strings.LastIndex(line[:width], " "); i > width/2 {
				cut = i
			}
			sb.WriteString(line[:cut])
			sb.WriteString("\n")
			line = strings.TrimLeft(line[cut:], " ")
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func limitLines(s string, n int, width int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = append(lines[:n], fmt.Sprintf("… (+%d more lines)", len(lines)-n))
	}
	if width > 0 {
		for i, l := range lines {
			if len(l) > width {
				lines[i] = l[:width-1] + "…"
			}
		}
	}
	return strings.Join(lines, "\n")
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	if n < 3 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
