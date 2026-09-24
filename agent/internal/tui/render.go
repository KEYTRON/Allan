package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
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
	MsgWelcome // start card, rendered from live state
)

type Message struct {
	Kind   MsgKind
	Text   string
	Tool   string
	Args   string
	Status string // "running" | "done" | "error"
}

// toolOutputLines is how many lines of tool output stay visible.
const toolOutputLines = 6

func renderMessage(m Message, width int) string {
	switch m.Kind {
	case MsgYou:
		return StyleUserBar.Width(width - 1).Render(wrap(m.Text, width-3))
	case MsgAllan:
		return prefixed(StyleDotAllan.Render("●"), renderMarkdown(m.Text, width-2), width)
	case MsgToolBlock:
		return renderToolBlock(m, width)
	case MsgWarn:
		return prefixed(StyleWarn.Render("▲"), StyleWarn.Render(wrap(m.Text, width-2)), width)
	case MsgError:
		return prefixed(StyleError.Render("✗"), StyleError.Render(wrap(m.Text, width-2)), width)
	default: // MsgSys, MsgTool
		return indent(StyleMuted.Render(wrap(m.Text, width-2)), "  ")
	}
}

// prefixed puts a one-cell marker before the first line and aligns the rest under it.
func prefixed(marker, body string, width int) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if i == 0 {
			lines[i] = marker + " " + l
		} else {
			lines[i] = "  " + l
		}
	}
	return strings.Join(lines, "\n")
}

func indent(s, pad string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

func renderToolBlock(m Message, width int) string {
	var status string
	switch m.Status {
	case "done":
		status = StyleOK.Render("✓")
	case "error":
		status = StyleError.Render("✗")
	default:
		status = StyleWarn.Render("…")
	}
	call := StyleDotTool.Render("⏺") + " " + StyleText.Bold(true).Render(m.Tool)
	if args := toolArgsSummary(m.Args); args != "" {
		call += StyleMuted.Render("(" + xansi.Truncate(args, maxInt(10, width-len(m.Tool)-8), "…") + ")")
	}
	call += " " + status

	body := strings.TrimRight(m.Text, "\n")
	switch {
	case m.Status == "running":
		return call
	case body == "":
		body = "(нет вывода)"
	}
	lines := strings.Split(body, "\n")
	extra := 0
	if len(lines) > toolOutputLines {
		extra = len(lines) - toolOutputLines
		lines = lines[:toolOutputLines]
	}
	out := make([]string, 0, len(lines)+1)
	for i, l := range lines {
		lead := "    "
		if i == 0 {
			lead = "  ⎿ "
		}
		out = append(out, StyleDim.Render(lead)+StyleMuted.Render(xansi.Truncate(l, maxInt(10, width-6), "…")))
	}
	if extra > 0 {
		out = append(out, StyleDim.Render(fmt.Sprintf("    … ещё %d строк", extra)))
	}
	return call + "\n" + strings.Join(out, "\n")
}

// toolArgsSummary turns {"command":"ls -la"} into `ls -la`: a single argument
// is shown bare, several as key=value pairs.
func toolArgsSummary(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || raw == "null" {
		return ""
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil || len(args) == 0 {
		return raw
	}
	if len(args) == 1 {
		for _, v := range args {
			return oneLine(fmt.Sprint(v))
		}
	}
	parts := make([]string, 0, len(args))
	for _, k := range sortedMapKeys(args) {
		parts = append(parts, k+"="+oneLine(fmt.Sprint(args[k])))
	}
	return strings.Join(parts, ", ")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

var (
	mdMu       sync.Mutex
	mdWidth    int
	mdRenderer *glamour.TermRenderer
)

// renderMarkdown renders assistant replies (code blocks, lists, headings).
// Falls back to plain wrapping if glamour fails.
func renderMarkdown(s string, width int) string {
	width = maxInt(20, width)
	mdMu.Lock()
	defer mdMu.Unlock()
	if mdRenderer == nil || mdWidth != width {
		r, err := glamour.NewTermRenderer(
			glamour.WithStyles(markdownStyle()),
			glamour.WithWordWrap(width),
			glamour.WithEmoji(),
		)
		if err != nil {
			return wrap(s, width)
		}
		mdRenderer, mdWidth = r, width
	}
	out, err := mdRenderer.Render(s)
	if err != nil {
		return wrap(s, width)
	}
	return trimBlankEdges(out)
}

// markdownStyle is glamour's dark theme without the document margin, so
// replies line up with the "●" marker instead of floating two columns right.
func markdownStyle() ansi.StyleConfig {
	st := styles.DarkStyleConfig
	zero := uint(0)
	st.Document.Margin = &zero
	st.Document.BlockPrefix = ""
	st.Document.BlockSuffix = ""
	return st
}

// trimBlankEdges drops the empty lines glamour adds around a document.
func trimBlankEdges(s string) string {
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(xansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(xansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// wrap word-wraps by display width (runes, wide chars), so Cyrillic and
// emoji wrap at the real edge and are never cut mid-character.
func wrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	return xansi.Wrap(s, width, "")
}

func truncate(s string, n int) string {
	if n <= 0 {
		return s
	}
	return xansi.Truncate(s, n, "…")
}

func padRight(s string, width int) string {
	return s + strings.Repeat(" ", maxInt(0, width-lipgloss.Width(s)))
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
