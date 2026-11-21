package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/i18n"
	"github.com/keytron/allan/agent/internal/mcp"
)

// mcpStatusText — /mcp list: какие серверы подключены, сколько весят их
// схемы и что модель видит прямо сейчас.
func (m *Model) mcpStatusText() string {
	if len(m.Cfg.MCP) == 0 {
		return i18n.S("MCP-серверов нет. Добавить: allan mcp add <имя> <команда|url>, или allan mcp import claude")
	}
	hidden := m.mcpHidden()
	var sb strings.Builder
	sb.WriteString(i18n.S("MCP-серверы (настройка: allan mcp, ~/.allan/config.toml)\n"))
	for _, st := range mcp.Shared(m.Cfg).Status() {
		line := fmt.Sprintf("  %s %-20s %-5s %-8s %s", mcpIcon(st.State), st.Name, st.Transport, st.Load, i18n.S(string(st.State)))
		if st.State == mcp.StateReady {
			line += fmt.Sprintf(i18n.S(" · инструментов: %d, ~%d токенов"), len(st.Tools), st.Tokens)
			if n := hidden[st.Name]; n > 0 {
				line += fmt.Sprintf(i18n.S(" · скрыто до load_tools: %d"), n)
			}
		}
		if st.Error != "" {
			line += " · " + st.Error
		}
		sb.WriteString(line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// mcpHidden — сколько инструментов каждого сервера модель сейчас не видит.
func (m *Model) mcpHidden() map[string]int {
	hidden := map[string]int{}
	if m.Ag != nil && m.Ag.Tools != nil {
		for _, t := range m.Ag.Tools.Deferred() {
			if g, ok := t.(interface{ Group() string }); ok {
				hidden[g.Group()]++
			}
		}
	}
	return hidden
}

func mcpIcon(s mcp.State) string {
	switch s {
	case mcp.StateReady:
		return "●"
	case mcp.StateConnecting:
		return "◌"
	case mcp.StateFailed:
		return "✗"
	}
	return "○"
}

// handleMCPSlash: /mcp — список серверов с управлением, /mcp list — текстом.
func (m *Model) handleMCPSlash(args []string) tea.Cmd {
	if len(args) > 0 && (args[0] == "list" || args[0] == "ls") {
		m.appendMsg(Message{Kind: MsgSys, Text: m.mcpStatusText()})
		return nil
	}
	if len(m.Cfg.MCP) == 0 {
		m.appendMsg(Message{Kind: MsgSys, Text: m.mcpStatusText()})
		return nil
	}
	m.openMCPPicker("")
	return nil
}

// mcpGroups — порядок групп в списке: сначала то, что требует внимания.
var mcpGroups = []mcp.State{mcp.StateFailed, mcp.StateConnecting, mcp.StateReady, mcp.StateDisabled}

// openMCPPicker показывает серверы по группам состояния; курсор встаёт на focus.
func (m *Model) openMCPPicker(focus string) {
	hidden := m.mcpHidden()
	byState := map[mcp.State][]mcp.ServerStatus{}
	for _, st := range mcp.Shared(m.Cfg).Status() {
		byState[st.State] = append(byState[st.State], st)
	}
	var items []pickItem
	for _, state := range mcpGroups {
		group := map[mcp.State]string{
			mcp.StateFailed: i18n.S("Ошибки"), mcp.StateConnecting: i18n.S("Подключаются"),
			mcp.StateReady: i18n.S("Готовы"), mcp.StateDisabled: i18n.S("Выключены"),
		}[state]
		for _, st := range byState[state] {
			detail := st.Transport + " · " + st.Load
			switch st.State {
			case mcp.StateReady:
				detail += fmt.Sprintf(i18n.S(" · %d инстр. · ~%d ток."), len(st.Tools), st.Tokens)
				if n := hidden[st.Name]; n > 0 {
					detail += fmt.Sprintf(i18n.S(" · скрыто %d"), n)
				}
			case mcp.StateFailed:
				detail += " · " + shorten(st.Error, 70)
			}
			items = append(items, pickItem{
				Label: mcpIcon(st.State) + " " + st.Name, Group: group, Detail: detail,
				Current: st.Name == focus, Dim: st.State == mcp.StateDisabled, Value: st.Name,
			})
		}
	}
	m.openPicker(newPicker(i18n.S("MCP-серверы · Enter — управление"), items, func(it pickItem) tea.Cmd {
		m.openMCPServerPicker(it.Value.(string))
		return nil
	}))
}

type mcpAction struct {
	kind string // toggle | reconnect | load | tools | error | remove | back
	arg  string
}

// openMCPServerPicker — действия над одним сервером.
func (m *Model) openMCPServerPicker(name string) {
	var st mcp.ServerStatus
	for _, s := range mcp.Shared(m.Cfg).Status() {
		if s.Name == name {
			st = s
		}
	}
	sc := m.Cfg.MCP[name]
	where := sc.URL
	if where == "" {
		where = strings.TrimSpace(sc.Command + " " + strings.Join(sc.Args, " "))
	}
	var items []pickItem
	if sc.Disabled {
		items = append(items, pickItem{Label: i18n.S("Включить"), Detail: shorten(where, 60), Value: mcpAction{kind: "toggle"}})
	} else {
		items = append(items,
			pickItem{Label: i18n.S("Выключить"), Detail: i18n.S("инструменты пропадут у модели"), Value: mcpAction{kind: "toggle"}},
			pickItem{Label: i18n.S("Переподключить"), Detail: shorten(where, 60), Value: mcpAction{kind: "reconnect"}},
		)
	}
	if len(st.Tools) > 0 {
		items = append(items, pickItem{Label: fmt.Sprintf(i18n.S("Инструменты (%d)"), len(st.Tools)),
			Detail: fmt.Sprintf(i18n.S("~%d токенов схем"), st.Tokens), Value: mcpAction{kind: "tools"}})
	}
	if st.Error != "" {
		items = append(items, pickItem{Label: i18n.S("Показать ошибку целиком"), Detail: shorten(st.Error, 60), Value: mcpAction{kind: "error"}})
	}
	loads := []struct{ v, help string }{
		{"auto", i18n.S("сразу, пока все такие серверы лёгкие, иначе по требованию")},
		{"always", i18n.S("схемы в каждом запросе")},
		{"deferred", i18n.S("только по требованию через load_tools")},
	}
	for _, l := range loads {
		items = append(items, pickItem{Label: i18n.S("Загрузка: ") + l.v, Group: i18n.S("Когда модель видит инструменты"),
			Detail: l.help, Current: st.Load == l.v, Value: mcpAction{kind: "load", arg: l.v}})
	}
	items = append(items,
		pickItem{Label: i18n.S("Удалить из конфига"), Group: " ", Detail: "~/.allan/config.toml", Value: mcpAction{kind: "remove"}},
		pickItem{Label: i18n.S("← К списку серверов"), Group: " ", Value: mcpAction{kind: "back"}},
	)
	p := newPicker(mcpIcon(st.State)+" "+name, items, func(it pickItem) tea.Cmd {
		return m.mcpDo(name, it.Value.(mcpAction))
	})
	p.selected = 0 // ● у режима загрузки — пометка, курсор начинает с первого действия
	m.openPicker(p)
}

// mcpDo выполняет действие, сохраняет конфиг и перенастраивает только этот сервер.
func (m *Model) mcpDo(name string, a mcpAction) tea.Cmd {
	sc, ok := m.Cfg.MCP[name]
	if !ok && a.kind != "back" {
		return nil
	}
	switch a.kind {
	case "back":
		m.openMCPPicker(name)
		return nil
	case "tools":
		var st mcp.ServerStatus
		for _, s := range mcp.Shared(m.Cfg).Status() {
			if s.Name == name {
				st = s
			}
		}
		names := append([]string{}, st.Tools...)
		sort.Strings(names)
		m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf(i18n.S("%s — инструменты (у модели: %s):\n  "), name, mcp.ToolName(name, "<имя>")) +
			strings.Join(names, ", ")})
		return nil
	case "error":
		for _, s := range mcp.Shared(m.Cfg).Status() {
			if s.Name == name {
				m.appendMsg(Message{Kind: MsgWarn, Text: name + ": " + s.Error})
			}
		}
		return nil
	case "toggle":
		sc.Disabled = !sc.Disabled
	case "load":
		sc.Load = a.arg
	case "remove":
		delete(m.Cfg.MCP, name)
	}
	if a.kind != "remove" {
		m.Cfg.MCP[name] = sc
	}
	if a.kind != "reconnect" {
		if err := config.Save(m.Cfg); err != nil {
			m.appendMsg(Message{Kind: MsgWarn, Text: i18n.S("Не удалось сохранить конфиг: ") + err.Error()})
			return nil
		}
	}
	mcp.Reconfigure(m.Cfg, name)
	switch {
	case a.kind == "remove":
		m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf(i18n.S("MCP %s удалён из конфига"), name)})
		return nil
	case a.kind == "load" && sc.Disabled:
		m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf(i18n.S("MCP %s: загрузка %s (сервер выключен)"), name, a.arg)})
		return nil
	case sc.Disabled:
		m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf(i18n.S("MCP %s выключен, его инструменты убраны"), name)})
		return nil
	}
	m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf(i18n.S("MCP %s: подключаюсь…"), name)})
	return waitMCP(m.Cfg, name)
}

// mcpStateMsg приходит, когда сервер после перенастройки подключился или упал.
type mcpStateMsg struct{ st mcp.ServerStatus }

func waitMCP(cfg *config.Config, name string) tea.Cmd {
	return func() tea.Msg {
		deadline := time.Now().Add(40 * time.Second)
		for {
			for _, s := range mcp.Shared(cfg).Status() {
				if s.Name == name && (s.State != mcp.StateConnecting || time.Now().After(deadline)) {
					return mcpStateMsg{st: s}
				}
			}
			if time.Now().After(deadline) {
				return nil
			}
			time.Sleep(300 * time.Millisecond)
		}
	}
}

func (m *Model) onMCPState(msg mcpStateMsg) {
	st := msg.st
	switch st.State {
	case mcp.StateReady:
		m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf(i18n.S("MCP %s готов · инструментов: %d, ~%d токенов"), st.Name, len(st.Tools), st.Tokens)})
	case mcp.StateFailed:
		m.appendMsg(Message{Kind: MsgWarn, Text: fmt.Sprintf(i18n.S("MCP %s не подключился: %s"), st.Name, st.Error)})
	default:
		m.appendMsg(Message{Kind: MsgWarn, Text: fmt.Sprintf(i18n.S("MCP %s всё ещё подключается — проверить: /mcp"), st.Name)})
	}
}

func shorten(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
