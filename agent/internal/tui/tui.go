package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/agent"
)

type FocusMode int

const (
	FocusTUI FocusMode = iota
	FocusPTY
)

type Model struct {
	Cfg       *config.Config
	Ag        *agent.Agent
	Workspace string
	Version   string

	width  int
	height int

	textarea textarea.Model
	viewport viewport.Model
	messages []Message

	history    []string
	historyIdx int

	popupOpen      bool
	popupItems     []SlashCommand
	popupSelected  int

	focus     FocusMode
	ptyActive bool
	ptyCmd    string
	ptyOutput string

	thinking bool
	startedAt time.Time
}

func New(cfg *config.Config, ag *agent.Agent, workspace, version string) *Model {
	ta := textarea.New()
	ta.Placeholder = "Спроси Allan..."
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.Prompt = ""
	ta.CharLimit = 8192
	ta.Focus()
	ta.KeyMap.InsertNewline.SetEnabled(false)

	vp := viewport.New(80, 20)
	vp.MouseWheelEnabled = true

	m := &Model{
		Cfg:       cfg,
		Ag:        ag,
		Workspace: workspace,
		Version:   version,
		textarea:  ta,
		viewport:  vp,
		startedAt: time.Now(),
	}
	m.welcome()
	return m
}

func (m *Model) welcome() {
	m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf(
		"Allan %s — backend: %s, model: %s, workspace: %s",
		m.Version, m.Ag.Backend.Name(), m.Cfg.Backend.Model, m.Workspace,
	)})
	m.appendMsg(Message{Kind: MsgSys, Text: "Введите /help для списка команд. Tab для автодополнения."})
}

func (m *Model) Init() tea.Cmd {
	return textarea.Blink
}

type agentEventMsg struct{ ev agent.Event }
type agentDoneMsg struct{}
type tickMsg time.Time

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
	case tea.KeyMsg:
		if cmd := m.handleKey(msg); cmd != nil {
			return m, cmd
		}
	case agentEventMsg:
		m.handleAgentEvent(msg.ev)
	case agentDoneMsg:
		m.thinking = false
		m.recalcLayout()
	case tickMsg:
		// future PTY tick
	}

	if !m.thinking && m.focus == FocusTUI {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyCtrlC:
		return tea.Quit
	case tea.KeyEsc:
		if m.popupOpen {
			m.popupOpen = false
			return nil
		}
	case tea.KeyTab:
		if m.popupOpen {
			if len(m.popupItems) > 0 {
				m.textarea.SetValue(m.popupItems[m.popupSelected].Name + " ")
				m.textarea.SetCursor(len(m.popupItems[m.popupSelected].Name) + 1)
				m.popupOpen = false
			}
			return nil
		}
		// PTY focus toggle
		if m.ptyActive {
			if m.focus == FocusTUI {
				m.focus = FocusPTY
			} else {
				m.focus = FocusTUI
			}
			return nil
		}
	case tea.KeyUp:
		if m.popupOpen {
			if m.popupSelected > 0 {
				m.popupSelected--
			}
			return nil
		}
		if !m.thinking && len(m.history) > 0 {
			if m.historyIdx > 0 {
				m.historyIdx--
				m.textarea.SetValue(m.history[m.historyIdx])
			}
			return nil
		}
	case tea.KeyDown:
		if m.popupOpen {
			if m.popupSelected < len(m.popupItems)-1 {
				m.popupSelected++
			}
			return nil
		}
		if !m.thinking && len(m.history) > 0 {
			if m.historyIdx < len(m.history)-1 {
				m.historyIdx++
				m.textarea.SetValue(m.history[m.historyIdx])
			} else {
				m.historyIdx = len(m.history)
				m.textarea.SetValue("")
			}
			return nil
		}
	case tea.KeyEnter:
		if m.thinking {
			return nil
		}
		val := strings.TrimSpace(m.textarea.Value())
		if val == "" {
			return nil
		}
		m.textarea.SetValue("")
		m.popupOpen = false
		m.history = append(m.history, val)
		m.historyIdx = len(m.history)
		return m.submit(val)
	}
	// Track input for popup
	if !m.thinking {
		val := m.textarea.Value()
		if strings.HasPrefix(val, "/") && !strings.Contains(val, " ") {
			matches := MatchSlash(val)
			if len(matches) > 0 {
				m.popupOpen = true
				m.popupItems = matches
				if m.popupSelected >= len(matches) {
					m.popupSelected = 0
				}
			} else {
				m.popupOpen = false
			}
		} else {
			m.popupOpen = false
		}
	}
	return nil
}

func (m *Model) submit(input string) tea.Cmd {
	m.appendMsg(Message{Kind: MsgYou, Text: input})

	if strings.HasPrefix(input, "/") {
		m.handleSlash(input)
		return nil
	}

	m.thinking = true
	events := make(chan agent.Event, 32)
	ctx := context.Background()
	go m.Ag.Run(ctx, input, events)

	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return agentDoneMsg{}
		}
		go m.drainEvents(events)
		return agentEventMsg{ev: ev}
	}
}

func (m *Model) drainEvents(events <-chan agent.Event) {
	for ev := range events {
		// Forward via program send through channel — handled at run time
		teaProg := globalProgram
		if teaProg != nil {
			teaProg.Send(agentEventMsg{ev: ev})
		}
	}
	if globalProgram != nil {
		globalProgram.Send(agentDoneMsg{})
	}
}

func (m *Model) handleAgentEvent(ev agent.Event) {
	switch ev.Kind {
	case "tool_call":
		args, _ := json.Marshal(ev.Args)
		m.appendMsg(Message{
			Kind:   MsgToolBlock,
			Tool:   ev.Tool,
			Args:   string(args),
			Status: "running",
			Text:   "(running…)",
		})
	case "tool_result":
		// Update last tool block
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Kind == MsgToolBlock && m.messages[i].Tool == ev.Tool && m.messages[i].Status == "running" {
				m.messages[i].Text = ev.Result
				if ev.IsError {
					m.messages[i].Status = "error"
				} else {
					m.messages[i].Status = "done"
				}
				break
			}
		}
	case "final":
		if ev.Text != "" {
			m.appendMsg(Message{Kind: MsgAllan, Text: ev.Text})
		}
	case "warn":
		m.appendMsg(Message{Kind: MsgWarn, Text: ev.Text})
	case "skill_saved":
		if ev.Skill != nil {
			m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf("Навык сохранён: %q (/skills чтобы посмотреть)", ev.Skill.Name)})
		}
	}
	m.recalcLayout()
}

func (m *Model) handleSlash(input string) {
	parts := strings.Fields(input)
	cmd := parts[0]
	args := parts[1:]
	switch cmd {
	case "/help":
		var sb strings.Builder
		for _, c := range SlashCommands {
			sb.WriteString(fmt.Sprintf("%-10s %s\n", c.Name, c.Help))
		}
		m.appendMsg(Message{Kind: MsgSys, Text: strings.TrimRight(sb.String(), "\n")})
	case "/quit", "/exit":
		globalProgram.Send(tea.Quit())
	case "/clear":
		m.messages = nil
		m.Ag.ClearHistory()
		m.welcome()
	case "/tools":
		var sb strings.Builder
		for _, t := range m.Ag.Tools.List() {
			sb.WriteString(fmt.Sprintf("• %s — %s\n", t.Name(), t.Description()))
		}
		m.appendMsg(Message{Kind: MsgSys, Text: strings.TrimRight(sb.String(), "\n")})
	case "/config":
		out := fmt.Sprintf("backend=%s model=%s workspace=%s\nmax_tool_calls=%d tool_timeout=%d scratchpad=%v",
			m.Cfg.Backend.Type, m.Cfg.Backend.Model, m.Workspace,
			m.Cfg.Agent.MaxToolCalls, m.Cfg.Agent.ToolTimeout, m.Cfg.Agent.ScratchpadEnabled)
		m.appendMsg(Message{Kind: MsgSys, Text: out})
	case "/plan":
		text := m.Ag.Scratch.Render()
		if text == "" {
			text = "(scratchpad пуст)"
		}
		m.appendMsg(Message{Kind: MsgSys, Text: text})
	case "/memory":
		if m.Ag.Memory == nil {
			m.appendMsg(Message{Kind: MsgWarn, Text: "Память отключена"})
		} else {
			m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf("Сессия: %s, тулзов: %d, токенов in/out: %d/%d",
				m.Ag.SessionID, m.Ag.Stats.ToolCalls, m.Ag.Stats.TokensIn, m.Ag.Stats.TokensOut)})
		}
	case "/skills":
		m.handleSkills(args)
	case "/model":
		if len(args) == 0 {
			m.appendMsg(Message{Kind: MsgSys, Text: "Текущая модель: " + m.Cfg.Backend.Model})
			return
		}
		m.Cfg.Backend.Model = args[0]
		_ = config.Save(m.Cfg)
		m.appendMsg(Message{Kind: MsgSys, Text: "Модель сменена на " + args[0] + " (потребуется перезапуск для смены backend)"})
	case "/backend":
		if len(args) == 0 {
			m.appendMsg(Message{Kind: MsgSys, Text: "Текущий бэкенд: " + m.Cfg.Backend.Type})
			return
		}
		m.appendMsg(Message{Kind: MsgWarn, Text: "Смена бэкенда требует перезапуска. Установлено в конфиге: " + args[0]})
		m.Cfg.Backend.Type = args[0]
		_ = config.Save(m.Cfg)
	default:
		m.appendMsg(Message{Kind: MsgWarn, Text: "Неизвестная команда: " + cmd})
	}
}

func (m *Model) handleSkills(args []string) {
	if m.Ag.Skills == nil {
		m.appendMsg(Message{Kind: MsgWarn, Text: "Skills engine не инициализирован"})
		return
	}
	ctx := context.Background()
	if len(args) == 0 {
		skills, err := m.Ag.Skills.List(ctx)
		if err != nil {
			m.appendMsg(Message{Kind: MsgError, Text: err.Error()})
			return
		}
		if len(skills) == 0 {
			m.appendMsg(Message{Kind: MsgSys, Text: "Навыков пока нет."})
			return
		}
		var sb strings.Builder
		for _, s := range skills {
			sb.WriteString(fmt.Sprintf("• %s — %s (used: %d)  [id: %s]\n", s.Name, s.Description, s.UsedCount, s.ID[:8]))
		}
		m.appendMsg(Message{Kind: MsgSys, Text: strings.TrimRight(sb.String(), "\n")})
		return
	}
	switch args[0] {
	case "show":
		if len(args) < 2 {
			m.appendMsg(Message{Kind: MsgWarn, Text: "/skills show <id>"})
			return
		}
		s, err := m.Ag.Skills.Get(ctx, args[1])
		if err != nil {
			m.appendMsg(Message{Kind: MsgError, Text: err.Error()})
			return
		}
		out := fmt.Sprintf("Name: %s\nDescription: %s\nTrigger: %s\nSolution: %s\nTools: %s\nUsed: %d",
			s.Name, s.Description, s.Trigger, s.Solution, strings.Join(s.ToolSequence, " → "), s.UsedCount)
		m.appendMsg(Message{Kind: MsgSys, Text: out})
	case "delete":
		if len(args) < 2 {
			m.appendMsg(Message{Kind: MsgWarn, Text: "/skills delete <id>"})
			return
		}
		if err := m.Ag.Skills.Delete(ctx, args[1]); err != nil {
			m.appendMsg(Message{Kind: MsgError, Text: err.Error()})
			return
		}
		m.appendMsg(Message{Kind: MsgSys, Text: "Навык удалён."})
	case "export":
		out, err := m.Ag.Skills.Export(ctx)
		if err != nil {
			m.appendMsg(Message{Kind: MsgError, Text: err.Error()})
			return
		}
		m.appendMsg(Message{Kind: MsgSys, Text: out})
	default:
		m.appendMsg(Message{Kind: MsgWarn, Text: "Подкоманды: show, delete, export"})
	}
}

func (m *Model) appendMsg(msg Message) {
	m.messages = append(m.messages, msg)
	m.recalcLayout()
}

func (m *Model) recalcLayout() {
	if m.width < 20 || m.height < 10 {
		return
	}
	headerH := 4
	statusH := 1
	inputH := 4
	popupH := 0
	if m.popupOpen {
		popupH = len(m.popupItems) + 2
		if popupH > 8 {
			popupH = 8
		}
	}
	mainH := m.height - headerH - statusH - inputH - popupH
	if mainH < 5 {
		mainH = 5
	}
	m.viewport.Width = m.width
	m.viewport.Height = mainH

	var sb strings.Builder
	for _, msg := range m.messages {
		sb.WriteString(renderMessage(msg, m.width-2))
		sb.WriteString("\n\n")
	}
	if m.thinking {
		sb.WriteString(StyleBadgeAllan.Render("[allan]") + " " + lipgloss.NewStyle().Foreground(ColorMuted).Render("thinking…"))
		sb.WriteString("\n")
	}
	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
	m.textarea.SetWidth(m.width - 4)
}

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}
	header := m.renderHeader()
	body := lipgloss.NewStyle().Background(ColorBgMain).Foreground(ColorFg).Render(m.viewport.View())
	popup := ""
	if m.popupOpen {
		popup = m.renderPopup()
	}
	input := m.renderInput()
	status := m.renderStatus()
	parts := []string{header, body}
	if popup != "" {
		parts = append(parts, popup)
	}
	parts = append(parts, input, status)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *Model) renderHeader() string {
	logoLines := strings.Split(Logo, "\n")
	logoWidth := 0
	for _, l := range logoLines {
		if len(l) > logoWidth {
			logoWidth = len(l)
		}
	}
	infoText := fmt.Sprintf("Allan v%s   [backend: %s/%s]   %s",
		m.Version, m.Ag.Backend.Name(), m.Cfg.Backend.Model, m.Workspace)
	logoStyled := StyleLogo.Render(Logo)
	infoStyled := StyleHeaderInfo.Render(infoText)

	pad := strings.Repeat(" ", maxInt(0, m.width-logoWidth-len(infoText)-2))
	right := infoStyled
	headerLine := lipgloss.JoinHorizontal(lipgloss.Top, logoStyled, pad, right)
	return StyleHeader.Width(m.width).Render(headerLine)
}

func (m *Model) renderInput() string {
	prompt := StylePrompt.Render("› ")
	return prompt + m.textarea.View()
}

func (m *Model) renderPopup() string {
	var sb strings.Builder
	for i, c := range m.popupItems {
		line := fmt.Sprintf("%-12s %s", c.Name, c.Help)
		if i == m.popupSelected {
			line = lipgloss.NewStyle().Foreground(ColorBgHeader).Background(ColorCyan).Render(line)
		}
		sb.WriteString(line + "\n")
	}
	return StylePopup.Render(strings.TrimRight(sb.String(), "\n"))
}

func (m *Model) renderStatus() string {
	parts := []string{
		fmt.Sprintf("workspace %s", abbrevPath(m.Workspace)),
		fmt.Sprintf("backend %s", m.Ag.Backend.Name()),
		fmt.Sprintf("model %s", m.Cfg.Backend.Model),
		fmt.Sprintf("calls %d", m.Ag.Stats.ToolCalls),
		fmt.Sprintf("tokens %d", m.Ag.Stats.TokensIn+m.Ag.Stats.TokensOut),
	}
	if m.ptyActive {
		parts = append(parts, fmt.Sprintf("⚙ %s", m.ptyCmd))
		if m.focus == FocusPTY {
			parts = append(parts, "Tab: фокус Allan")
		} else {
			parts = append(parts, "Tab: фокус shell")
		}
	}
	left := strings.Join(parts, "  ·  ")
	right := "? /help"
	pad := strings.Repeat(" ", maxInt(1, m.width-len(left)-len(right)-2))
	return StyleStatus.Width(m.width).Render(left + pad + right)
}

func abbrevPath(p string) string {
	home := homeDir()
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

func homeDir() string {
	if h, err := osUserHomeDir(); err == nil {
		return h
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
