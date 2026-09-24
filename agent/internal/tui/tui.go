package tui

import (
	"context"
	"regexp"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/agent"
	"github.com/keytron/allan/agent/internal/backend"
)

// bubbles/textarea draws the cursor over the placeholder's first byte,
// which garbles a leading Cyrillic letter, so it starts with ASCII.
const inputPlaceholder = "/ — команды, или просто напишите задачу"

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
	popupNavigated bool // arrows were used, so Enter takes the highlighted item
	picker         *picker
	cancelRun      context.CancelFunc
	quitArmedAt    time.Time // first Ctrl+C on an empty input; a second one quits
	modelCatalog  []modelChoice

	focus     FocusMode
	ptyActive bool
	ptyCmd    string
	ptyOutput string

	thinking      bool
	thinkingSince time.Time
	spinner       spinner.Model
	startedAt     time.Time
}

type modelChoice struct {
	Provider string
	Model    string
}

func New(cfg *config.Config, ag *agent.Agent, workspace, version string) *Model {
	ta := textarea.New()
	ta.Placeholder = inputPlaceholder
	ta.ShowLineNumbers = false
	ta.SetHeight(1)
	ta.Prompt = ""
	// Default textarea styles paint the cursor line grey; keep it transparent.
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.FocusedStyle.Text = StyleText
	ta.FocusedStyle.Placeholder = StyleDim
	ta.BlurredStyle = ta.FocusedStyle
	ta.Cursor.Style = StyleBrand
	ta.CharLimit = 8192
	ta.Focus()
	ta.KeyMap.InsertNewline.SetEnabled(false)

	vp := viewport.New(80, 20)
	vp.MouseWheelEnabled = true
	// Only page keys scroll: the default map also binds k/j/u/d/space,
	// which would scroll the chat while typing.
	vp.KeyMap = viewport.KeyMap{
		PageUp:   key.NewBinding(key.WithKeys("pgup")),
		PageDown: key.NewBinding(key.WithKeys("pgdown")),
	}

	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(StyleDotAllan))

	m := &Model{
		Cfg:       cfg,
		Ag:        ag,
		Workspace: workspace,
		Version:   version,
		textarea:  ta,
		viewport:  vp,
		spinner:   sp,
		startedAt: time.Now(),
	}
	m.welcome()
	return m
}

func (m *Model) welcome() {
	m.appendMsg(Message{Kind: MsgWelcome})
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
		if isMouseGarbage(msg) {
			return m, nil
		}
		if cmd, consumed := m.handleKey(msg); consumed {
			m.recalcLayout()
			return m, cmd
		}
	case agentEventMsg:
		m.handleAgentEvent(msg.ev)
	case agentDoneMsg:
		m.thinking = false
		if m.cancelRun != nil {
			m.cancelRun()
			m.cancelRun = nil
		}
		m.recalcLayout()
	case tickMsg:
		// future PTY tick
	case spinner.TickMsg:
		if m.thinking {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			m.recalcLayout()
			cmds = append(cmds, cmd)
		}
	}

	if !m.thinking && m.focus == FocusTUI {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
		if m.picker != nil {
			m.picker.filter(m.textarea.Value())
		} else {
			m.updatePopup()
		}
		m.recalcLayout()
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// handleKey processes navigation and submit keys. consumed=false passes the
// key on to the input box.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if msg.Type == tea.KeyCtrlC {
		switch {
		case m.picker != nil:
			m.closePicker()
		case m.textarea.Value() != "":
			m.textarea.SetValue("")
			m.popupOpen = false
		case time.Since(m.quitArmedAt) < 2*time.Second:
			return tea.Quit, true
		default:
			m.quitArmedAt = time.Now()
			m.appendMsg(Message{Kind: MsgSys, Text: "Нажмите Ctrl+C ещё раз, чтобы выйти"})
		}
		return nil, true
	}
	if msg.Type == tea.KeyEsc && m.thinking && m.cancelRun != nil {
		m.cancelRun()
		m.appendMsg(Message{Kind: MsgWarn, Text: "Остановлено"})
		return nil, true
	}
	if m.picker != nil {
		return m.pickerKey(msg)
	}
	switch msg.Type {
	case tea.KeyEsc:
		if m.popupOpen {
			m.popupOpen = false
			return nil, true
		}
	case tea.KeyTab:
		if m.popupOpen && len(m.popupItems) > 0 {
			m.insertCompletion(m.popupItems[m.popupSelected])
			return nil, true
		}
		if m.ptyActive {
			if m.focus == FocusTUI {
				m.focus = FocusPTY
			} else {
				m.focus = FocusTUI
			}
			return nil, true
		}
	case tea.KeyUp, tea.KeyShiftTab:
		if m.popupOpen {
			m.popupSelected = (m.popupSelected - 1 + len(m.popupItems)) % len(m.popupItems)
			m.popupNavigated = true
			return nil, true
		}
		if msg.Type == tea.KeyUp && !m.thinking && len(m.history) > 0 && m.historyIdx > 0 {
			m.historyIdx--
			m.textarea.SetValue(m.history[m.historyIdx])
			return nil, true
		}
	case tea.KeyDown:
		if m.popupOpen {
			m.popupSelected = (m.popupSelected + 1) % len(m.popupItems)
			m.popupNavigated = true
			return nil, true
		}
		if !m.thinking && len(m.history) > 0 && m.historyIdx < len(m.history) {
			m.historyIdx++
			if m.historyIdx == len(m.history) {
				m.textarea.SetValue("")
			} else {
				m.textarea.SetValue(m.history[m.historyIdx])
			}
			return nil, true
		}
	case tea.KeyEnter:
		if m.thinking {
			return nil, true
		}
		val := normalizeSpaces(strings.TrimSpace(m.textarea.Value()))
		if m.popupOpen && len(m.popupItems) > 0 {
			item := m.popupItems[m.popupSelected]
			if m.popupNavigated || isPartialCommand(val, item) {
				if strings.HasSuffix(item.Insert, " ") && !strings.HasPrefix(item.Name, "/") {
					// Subcommand that needs arguments: complete it, don't run it.
					m.insertCompletion(item)
					return nil, true
				}
				val = strings.TrimSpace(item.Insert)
				if val == "" {
					val = item.Name
				}
			}
		}
		if val == "" {
			return nil, true
		}
		m.textarea.SetValue("")
		m.popupOpen = false
		m.popupNavigated = false
		m.history = append(m.history, redactSensitiveInput(val))
		m.historyIdx = len(m.history)
		return m.submit(val), true
	}
	return nil, false
}

func (m *Model) insertCompletion(item SlashCommand) {
	insert := item.Insert
	if insert == "" {
		insert = item.Name + " "
	}
	m.textarea.SetValue(insert)
	m.textarea.CursorEnd()
	m.popupOpen = false
	m.popupNavigated = false
}

// isPartialCommand reports that the user typed only the start of a top-level
// command ("/mo" for "/model"), so Enter should run the suggestion.
func isPartialCommand(val string, item SlashCommand) bool {
	return strings.HasPrefix(item.Name, "/") && !strings.Contains(val, " ") &&
		val != item.Name && strings.HasPrefix(item.Name, val)
}

// normalizeSpaces turns non-breaking and zero-width spaces (they sneak in
// with pasted text) into plain spaces, so "/model 4" always splits.
func normalizeSpaces(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\u200b' || r == '\u2060' || r == '\ufeff':
			return ' '
		case unicode.IsSpace(r):
			return ' '
		}
		return r
	}, s)
}

func (m *Model) updatePopup() {
	if m.thinking || m.picker != nil {
		m.popupOpen = false
		return
	}
	val := m.textarea.Value()
	if strings.HasPrefix(val, "/") {
		matches := m.completionsFor(val)
		if len(matches) > 0 {
			if !m.popupOpen || len(matches) != len(m.popupItems) {
				m.popupNavigated = false
			}
			m.popupOpen = true
			m.popupItems = matches
			if m.popupSelected >= len(matches) {
				m.popupSelected = 0
			}
			return
		}
	}
	m.popupOpen = false
}

func (m *Model) completionsFor(input string) []SlashCommand {
	if !strings.Contains(input, " ") {
		return MatchSlash(input)
	}
	if strings.HasPrefix(input, "/api ") {
		return m.apiCompletions(input)
	}
	if strings.HasPrefix(input, "/model ") {
		return m.modelCompletions(input)
	}
	return nil
}

func (m *Model) apiCompletions(input string) []SlashCommand {
	endsWithSpace := strings.HasSuffix(input, " ")
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}
	if len(parts) == 1 || (len(parts) == 2 && !endsWithSpace) {
		prefix := ""
		if len(parts) == 2 {
			prefix = parts[1]
		}
		return filterCompletions(prefix, []SlashCommand{
			{Name: "add", Help: "добавить API-ключ", Insert: "/api add "},
			{Name: "key", Help: "заменить ключ провайдера", Insert: "/api key "},
			{Name: "endpoint", Help: "свой эндпоинт (base_url)", Insert: "/api endpoint "},
			{Name: "list", Help: "показать сохранённые провайдеры", Insert: "/api list"},
			{Name: "refresh", Help: "опросить модели всех провайдеров", Insert: "/api refresh"},
			{Name: "use", Help: "сделать провайдера активным", Insert: "/api use "},
			{Name: "remove", Help: "удалить провайдера", Insert: "/api remove "},
		})
	}
	sub := ""
	if len(parts) >= 2 {
		sub = parts[1]
	}
	if (sub == "add" || sub == "key" || sub == "endpoint") && (len(parts) == 2 || (len(parts) == 3 && !endsWithSpace)) {
		prefix := ""
		if len(parts) == 3 {
			prefix = parts[2]
		}
		return providerCompletions(prefix, "/api "+sub+" ")
	}
	if (sub == "use" || sub == "remove" || sub == "delete" || sub == "rm") && (len(parts) == 2 || (len(parts) == 3 && !endsWithSpace)) {
		prefix := ""
		if len(parts) == 3 {
			prefix = parts[2]
		}
		return configuredProviderCompletions(prefix, "/api "+sub+" ", m.Cfg)
	}
	return nil
}

func (m *Model) modelCompletions(input string) []SlashCommand {
	if len(m.modelCatalog) == 0 {
		return []SlashCommand{{
			Name:   "refresh",
			Help:   "сначала обновить список моделей",
			Insert: "/api refresh",
		}}
	}
	prefix := strings.TrimSpace(strings.TrimPrefix(input, "/model"))
	out := make([]SlashCommand, 0, len(m.modelCatalog))
	for i, choice := range m.modelCatalog {
		label := fmt.Sprintf("%d. %s/%s", i+1, choice.Provider, choice.Model)
		insert := fmt.Sprintf("/model %d", i+1)
		if prefix == "" || strings.Contains(strings.ToLower(label), strings.ToLower(prefix)) {
			out = append(out, SlashCommand{Name: label, Help: "выбрать модель", Insert: insert})
		}
	}
	return out
}

func (m *Model) submit(input string) tea.Cmd {
	m.appendMsg(Message{Kind: MsgYou, Text: redactSensitiveInput(input)})

	if strings.HasPrefix(input, "/") {
		return m.handleSlash(input)
	}

	m.thinking = true
	m.thinkingSince = time.Now()
	m.recalcLayout()
	events := make(chan agent.Event, 32)
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelRun = cancel
	go m.Ag.Run(ctx, input, events)

	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return agentDoneMsg{}
		}
		go m.drainEvents(events)
		return agentEventMsg{ev: ev}
	})
}

func redactSensitiveInput(input string) string {
	parts := strings.Fields(input)
	if len(parts) >= 4 && parts[0] == "/api" && (parts[1] == "add" || parts[1] == "set" || parts[1] == "key") {
		parts[3] = "****"
		return strings.Join(parts, " ")
	}
	return input
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
				m.messages[i].rendered = ""
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
		if strings.Contains(ev.Text, "context canceled") {
			break // the user pressed Esc; "Остановлено" is already shown
		}
		m.appendMsg(Message{Kind: MsgWarn, Text: humanError(ev.Text)})
	case "skill_saved":
		if ev.Skill != nil {
			m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf("Навык сохранён: %q (/skills чтобы посмотреть)", ev.Skill.Name)})
		}
	}
	m.recalcLayout()
}

func (m *Model) handleSlash(input string) tea.Cmd {
	parts := strings.Fields(normalizeSpaces(input))
	if len(parts) == 0 {
		return nil
	}
	cmd := parts[0]
	args := parts[1:]
	switch cmd {
	case "/help":
		m.appendMsg(Message{Kind: MsgSys, Text: helpText()})
	case "/quit", "/exit":
		return tea.Quit
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
			m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf("Память включена\nsession=%s\ndb=%s\nchromadb=%s\ntool_calls=%d\ntokens_in=%d\ntokens_out=%d",
				m.Ag.SessionID, config.Expand(m.Cfg.Memory.DBPath), m.Cfg.Memory.ChromaDBURL,
				m.Ag.Stats.ToolCalls, m.Ag.Stats.TokensIn, m.Ag.Stats.TokensOut)})
		}
	case "/skills":
		m.handleSkills(args)
	case "/api":
		m.handleAPI(args)
	case "/model":
		m.handleModel(args)
	case "/backend":
		if len(args) == 0 {
			m.openProviderPicker()
			return nil
		}
		m.appendMsg(Message{Kind: MsgWarn, Text: "Смена бэкенда требует перезапуска. Установлено в конфиге: " + args[0]})
		m.Cfg.Backend.Type = args[0]
		_ = config.Save(m.Cfg)
	default:
		matches := MatchSlash(cmd)
		if len(matches) == 1 {
			expanded := strings.TrimSpace(matches[0].Name + " " + strings.Join(args, " "))
			return m.handleSlash(expanded)
		}
		m.appendMsg(Message{Kind: MsgWarn, Text: "Неизвестная команда: " + cmd})
	}
	return nil
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

func (m *Model) handleAPI(args []string) {
	if len(args) == 0 {
		m.appendMsg(Message{Kind: MsgSys, Text: strings.Join([]string{
			"/api list — провайдеры, эндпоинты и где лежат ключи",
			"/api add <provider> <api_key> [base_url] — ключ провайдера (и свой base_url)",
			"/api key <provider> <api_key> — заменить только ключ",
			"/api endpoint <name> <base_url> [openai|anthropic] — свой эндпоинт без ключа или смена base_url",
			"/api use <provider>",
			"/api remove <provider> — удалить провайдер вместе с ключом",
			"/api refresh",
			"",
			"Ключ безопаснее вводить вне TUI: allan key set <provider> (скрытый ввод).",
			"Ключи хранятся в " + m.secretsKind() + ", не в config.toml.",
			"Провайдеры: " + strings.Join(knownCloudProviders(), ", ") + "; локальные: ollama, llamacpp, lmstudio.",
		}, "\n")})
		return
	}
	switch args[0] {
	case "list", "show":
		m.appendMsg(Message{Kind: MsgSys, Text: m.renderProviders()})
	case "add", "set":
		if len(args) < 3 {
			m.appendMsg(Message{Kind: MsgWarn, Text: "/api add <provider> <api_key> [base_url]"})
			return
		}
		name := normalizeProvider(args[1])
		base := ""
		if len(args) >= 4 {
			base = args[3]
		}
		if err := m.saveProvider(name, base, "", args[2], true); err != nil {
			m.appendMsg(Message{Kind: MsgError, Text: err.Error()})
			return
		}
		m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf("Ключ для %s сохранён в %s. Выполните /api refresh или /model, чтобы обновить список моделей.", name, m.secretsKind())})
	case "key":
		if len(args) < 3 {
			m.appendMsg(Message{Kind: MsgWarn, Text: "/api key <provider> <api_key>"})
			return
		}
		name := normalizeProvider(args[1])
		if err := m.saveProvider(name, "", "", args[2], true); err != nil {
			m.appendMsg(Message{Kind: MsgError, Text: err.Error()})
			return
		}
		m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf("Ключ для %s обновлён (%s).", name, m.secretsKind())})
	case "endpoint", "url":
		if len(args) < 3 {
			m.appendMsg(Message{Kind: MsgWarn, Text: "/api endpoint <name> <base_url> [openai|anthropic]"})
			return
		}
		name := normalizeProvider(args[1])
		typ := ""
		if len(args) >= 4 {
			typ = strings.ToLower(args[3])
			if typ != "openai" && typ != "anthropic" {
				m.appendMsg(Message{Kind: MsgWarn, Text: "Тип эндпоинта: openai (OpenAI-совместимый) или anthropic"})
				return
			}
		}
		if err := m.saveProvider(name, args[2], typ, "", false); err != nil {
			m.appendMsg(Message{Kind: MsgError, Text: err.Error()})
			return
		}
		m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf("Эндпоинт %s → %s сохранён. Ключ при необходимости: /api key %s <api_key>", name, args[2], name)})
	case "use":
		if len(args) < 2 {
			m.openProviderPicker()
			return
		}
		name := normalizeProvider(args[1])
		model := m.Cfg.Backend.Model
		if len(m.modelCatalog) == 0 {
			_ = m.refreshModelCatalog(context.Background())
		}
		for _, choice := range m.modelCatalog {
			if choice.Provider == name {
				model = choice.Model
				break
			}
		}
		m.switchBackendModel(name, model)
	case "remove", "delete", "rm":
		if len(args) < 2 {
			m.appendMsg(Message{Kind: MsgWarn, Text: "/api remove <provider>"})
			return
		}
		name := normalizeProvider(args[1])
		delete(m.Cfg.Providers, name)
		if m.Cfg.Secrets != nil {
			if err := m.Cfg.Secrets.Delete(name); err != nil {
				m.appendMsg(Message{Kind: MsgError, Text: "ключ не удалён: " + err.Error()})
			}
		}
		if m.Cfg.Backend.Type == name {
			m.Cfg.Backend.APIKey = ""
		}
		if err := config.Save(m.Cfg); err != nil {
			m.appendMsg(Message{Kind: MsgError, Text: err.Error()})
			return
		}
		m.appendMsg(Message{Kind: MsgSys, Text: "Провайдер и его ключ удалены: " + name})
	case "refresh":
		if err := m.refreshModelCatalog(context.Background()); err != nil {
			m.appendMsg(Message{Kind: MsgWarn, Text: err.Error()})
		}
		m.appendMsg(Message{Kind: MsgSys, Text: m.renderModelCatalog()})
	default:
		m.appendMsg(Message{Kind: MsgWarn, Text: "Подкоманды /api: list, add, key, endpoint, use, remove, refresh"})
	}
}

// saveProvider creates or updates a provider entry. Empty base/typ keep the
// current value (or the preset default); the key goes to the secret store.
func (m *Model) saveProvider(name, base, typ, key string, setKey bool) error {
	if name == "" {
		return fmt.Errorf("пустое имя провайдера")
	}
	if m.Cfg.Providers == nil {
		m.Cfg.Providers = map[string]config.ProviderConfig{}
	}
	p := m.Cfg.Providers[name]
	if base != "" {
		p.BaseURL = strings.TrimRight(base, "/")
	} else if p.BaseURL == "" {
		p.BaseURL = defaultBaseURL(name)
	}
	if p.BaseURL == "" {
		return fmt.Errorf("%s не входит в список известных провайдеров — укажите base_url: /api add %s <api_key> <base_url>", name, name)
	}
	if typ != "" {
		p.Type = typ
	} else if p.Type == "" {
		p.Type = providerType(name)
	}
	if setKey {
		if m.Cfg.Secrets == nil {
			return fmt.Errorf("хранилище ключей не инициализировано")
		}
		if err := m.Cfg.Secrets.Set(name, key); err != nil {
			return fmt.Errorf("не удалось сохранить ключ в %s: %w", m.Cfg.Secrets.Kind(), err)
		}
		p.APIKey = key
	}
	m.Cfg.Providers[name] = p
	if m.Cfg.Backend.Type == name {
		m.Cfg.Backend.BaseURL = p.BaseURL
		m.Cfg.Backend.APIKey = p.APIKey
	}
	return config.Save(m.Cfg)
}

func (m *Model) secretsKind() string {
	if m.Cfg.Secrets == nil {
		return "не настроено"
	}
	return m.Cfg.Secrets.Kind()
}

func knownCloudProviders() []string {
	out := []string{}
	for name, p := range backend.ProviderPresets {
		if !p.Local {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func (m *Model) handleModel(args []string) {
	if len(args) == 0 {
		m.openModelPicker("")
		return
	}
	choice, ok := m.resolveModelChoice(strings.Join(args, " "))
	if !ok {
		m.appendMsg(Message{Kind: MsgWarn, Text: "Модель не найдена. Сначала выполните /model или /api refresh, затем /model <номер>."})
		return
	}
	m.switchBackendModel(choice.Provider, choice.Model)
}

func (m *Model) refreshModelCatalog(ctx context.Context) error {
	choices := []modelChoice{}
	failures := []string{}
	providers := m.availableProviders()
	for _, name := range providers {
		if hugeCatalogProvider(name) {
			continue
		}
		be, err := backendForProvider(name, m.Cfg, m.Cfg.Backend.Model)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		pctx, cancel := context.WithTimeout(ctx, 6*time.Second)
		models, err := be.Models(pctx)
		cancel()
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		for _, model := range models {
			if model != "" {
				choices = append(choices, modelChoice{Provider: name, Model: model})
			}
		}
	}
	sort.SliceStable(choices, func(i, j int) bool {
		if choices[i].Provider == choices[j].Provider {
			return choices[i].Model < choices[j].Model
		}
		return choices[i].Provider < choices[j].Provider
	})
	m.modelCatalog = choices
	if len(choices) == 0 && len(failures) > 0 {
		return fmt.Errorf("не удалось получить модели: %s", strings.Join(failures, "; "))
	}
	return nil
}

func (m *Model) availableProviders() []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		name = normalizeProvider(name)
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	add(m.Cfg.Backend.Type)
	for name := range m.Cfg.Providers {
		add(name)
	}
	for _, name := range []string{"ollama", "llamacpp", "lmstudio"} {
		add(name)
	}
	sort.Strings(out)
	return out
}

func (m *Model) renderModelCatalog() string {
	if len(m.modelCatalog) == 0 {
		return "Модели не найдены. Для API-провайдера добавьте ключ: /api add <provider> <api_key>" + m.hugeCatalogHint()
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Текущая модель: %s/%s\n\n", m.Cfg.Backend.Type, m.Cfg.Backend.Model))
	currentProvider := ""
	for i, choice := range m.modelCatalog {
		if choice.Provider != currentProvider {
			currentProvider = choice.Provider
			sb.WriteString(currentProvider + ":\n")
		}
		mark := " "
		if choice.Provider == m.Cfg.Backend.Type && choice.Model == m.Cfg.Backend.Model {
			mark = "*"
		}
		sb.WriteString(fmt.Sprintf("%s %2d. %s\n", mark, i+1, choice.Model))
	}
	sb.WriteString("\nВыбор: /model <номер> или /model <provider>/<model>")
	sb.WriteString(m.hugeCatalogHint())
	return strings.TrimRight(sb.String(), "\n")
}

func (m *Model) hugeCatalogHint() string {
	if _, ok := m.Cfg.Providers["featherless"]; !ok {
		return ""
	}
	return "\nfeatherless: каталог ~20 тыс. моделей в список не выводится, выбор по id: /model featherless/<owner>/<model> (список: featherless.ai/models)"
}

func (m *Model) renderProviders() string {
	if len(m.Cfg.Providers) == 0 {
		return "API-провайдеры не настроены. Добавить: /api add <provider> <api_key> [base_url]\nКлючи хранятся в " + m.secretsKind()
	}
	names := make([]string, 0, len(m.Cfg.Providers))
	for name := range m.Cfg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	var sb strings.Builder
	for _, name := range names {
		p := m.Cfg.Providers[name]
		key := "нет"
		if p.APIKey != "" {
			key = "есть"
		}
		active := ""
		if name == m.Cfg.Backend.Type {
			active = " (active)"
		}
		sb.WriteString(fmt.Sprintf("%s%s: type=%s base_url=%s ключ=%s\n", name, active, p.Type, p.BaseURL, key))
	}
	sb.WriteString("\nКлючи хранятся в " + m.secretsKind())
	return sb.String()
}

func (m *Model) resolveModelChoice(input string) (modelChoice, bool) {
	input = strings.TrimSpace(input)
	if n, err := strconv.Atoi(input); err == nil && n >= 1 && n <= len(m.modelCatalog) {
		return m.modelCatalog[n-1], true
	}
	if strings.Contains(input, "/") {
		parts := strings.SplitN(input, "/", 2)
		return modelChoice{Provider: normalizeProvider(parts[0]), Model: parts[1]}, parts[0] != "" && parts[1] != ""
	}
	if len(m.modelCatalog) == 0 {
		_ = m.refreshModelCatalog(context.Background())
	}
	for _, choice := range m.modelCatalog {
		if choice.Model == input {
			return choice, true
		}
	}
	return modelChoice{Provider: m.Cfg.Backend.Type, Model: input}, input != ""
}

func (m *Model) switchBackendModel(provider, model string) {
	provider = normalizeProvider(provider)
	next, err := backendForProvider(provider, m.Cfg, model)
	if err != nil {
		m.appendMsg(Message{Kind: MsgError, Text: err.Error()})
		return
	}
	m.Cfg.Backend.Type = provider
	m.Cfg.Backend.Model = model
	if p, ok := m.Cfg.Providers[provider]; ok {
		m.Cfg.Backend.BaseURL = p.BaseURL
		m.Cfg.Backend.APIKey = p.APIKey
	} else {
		m.Cfg.Backend.BaseURL = defaultBaseURL(provider)
		if isLocalProvider(provider) {
			m.Cfg.Backend.APIKey = ""
		}
	}
	if err := config.Save(m.Cfg); err != nil {
		m.appendMsg(Message{Kind: MsgError, Text: err.Error()})
		return
	}
	m.Ag.Backend = next
	m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf("Модель сменена: %s/%s", provider, model)})
}

func backendForProvider(provider string, cfg *config.Config, model string) (backend.Backend, error) {
	provider = normalizeProvider(provider)
	if p, ok := cfg.Providers[provider]; ok {
		tmp := *cfg
		tmp.Backend.Type = provider
		tmp.Backend.Model = model
		tmp.Backend.BaseURL = p.BaseURL
		tmp.Backend.APIKey = p.APIKey
		if p.Type != "" {
			tmp.Backend.Type = p.Type
			if provider != p.Type && p.Type == "openai" {
				return backend.NewOpenAILike(provider, p.BaseURL, p.APIKey, model), nil
			}
		}
		return backend.New(&tmp)
	}
	switch {
	case !isKnownProvider(provider):
		return nil, fmt.Errorf("неизвестный провайдер %s: добавьте эндпоинт /api endpoint %s <base_url>", provider, provider)
	case !isLocalProvider(provider) && provider != "anthropic" && provider != "openai":
		return nil, fmt.Errorf("%s требует API-ключ: /api add %s <api_key>", provider, provider)
	default:
		tmp := *cfg
		tmp.Backend.Type = provider
		tmp.Backend.Model = model
		tmp.Backend.BaseURL = defaultBaseURL(provider)
		if isLocalProvider(provider) {
			tmp.Backend.APIKey = ""
		}
		return backend.New(&tmp)
	}
}

func normalizeProvider(name string) string { return backend.NormalizeProvider(name) }
func providerType(name string) string      { return backend.ProviderType(name) }
func defaultBaseURL(name string) string    { return backend.DefaultBaseURL(name) }
func isKnownProvider(name string) bool     { return backend.IsKnownProvider(name) }

// hugeCatalogProvider reports providers whose /models lists tens of thousands
// of entries (Featherless mirrors most of Hugging Face), too many for /model.
func hugeCatalogProvider(name string) bool {
	return normalizeProvider(name) == "featherless"
}

func isLocalProvider(name string) bool { return backend.IsLocalProvider(name) }

func filterCompletions(prefix string, items []SlashCommand) []SlashCommand {
	if prefix == "" {
		return items
	}
	prefix = strings.ToLower(prefix)
	out := make([]SlashCommand, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(item.Name), prefix) {
			out = append(out, item)
		}
	}
	return out
}

func providerCompletions(prefix, commandPrefix string) []SlashCommand {
	names := make([]string, 0, len(backend.ProviderPresets))
	for name, p := range backend.ProviderPresets {
		if !p.Local {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	items := make([]SlashCommand, 0, len(names))
	for _, name := range names {
		p := backend.ProviderPresets[name]
		items = append(items, SlashCommand{Name: name, Help: p.Help + ", " + p.BaseURL, Insert: commandPrefix + name + " "})
	}
	return filterCompletions(prefix, items)
}

func configuredProviderCompletions(prefix, commandPrefix string, cfg *config.Config) []SlashCommand {
	items := []SlashCommand{}
	names := make([]string, 0, len(cfg.Providers)+3)
	for name := range cfg.Providers {
		names = append(names, name)
	}
	names = append(names, "ollama", "llamacpp", "lmstudio")
	sort.Strings(names)
	seen := map[string]bool{}
	for _, name := range names {
		name = normalizeProvider(name)
		if seen[name] {
			continue
		}
		seen[name] = true
		items = append(items, SlashCommand{Name: name, Help: "provider", Insert: commandPrefix + name})
	}
	return filterCompletions(prefix, items)
}

func (m *Model) appendMsg(msg Message) {
	m.messages = append(m.messages, msg)
	if msg.Kind == MsgYou {
		m.viewport.GotoBottom() // own input always brings the chat back down
	}
	m.recalcLayout()
}

func (m *Model) recalcLayout() {
	if m.width < 20 || m.height < 8 {
		return
	}
	m.textarea.SetWidth(maxInt(10, m.width-6))
	m.textarea.SetHeight(m.inputLines())

	headerH := lipgloss.Height(m.renderHeader())
	inputH := lipgloss.Height(m.renderInput())
	statusH := 1
	popupH := 0
	switch {
	case m.picker != nil:
		popupH = lipgloss.Height(m.renderPicker())
	case m.popupOpen:
		popupH = lipgloss.Height(m.renderPopup())
	}
	m.viewport.Width = m.width
	m.viewport.Height = maxInt(3, m.height-headerH-inputH-statusH-popupH)
	m.refreshContent()
}

// refreshContent rebuilds the chat text from cached renders. It follows new
// output only when the view was already at the bottom, so scrolling up to
// read history is not undone by the next keypress or spinner tick.
func (m *Model) refreshContent() {
	w := maxInt(20, m.width-2)
	blocks := make([]string, 0, len(m.messages)+1)
	for i := range m.messages {
		msg := &m.messages[i]
		if msg.Kind == MsgWelcome {
			blocks = append(blocks, m.renderWelcome(w))
			continue
		}
		if msg.rendered == "" || msg.renderedW != w {
			msg.rendered, msg.renderedW = renderMessage(*msg, w), w
		}
		blocks = append(blocks, msg.rendered)
	}
	if m.thinking {
		elapsed := time.Since(m.thinkingSince).Round(time.Second)
		blocks = append(blocks, m.spinner.View()+" "+StyleMuted.Render(fmt.Sprintf("Allan думает… %s · Esc — остановить", elapsed)))
	}
	follow := m.viewport.AtBottom() || m.viewport.TotalLineCount() == 0
	m.viewport.SetContent(indent(strings.Join(blocks, "\n\n"), " "))
	if follow {
		m.viewport.GotoBottom()
	}
}

// inputLines grows the input box with its content, up to 6 lines.
func (m *Model) inputLines() int {
	w := maxInt(10, m.width-6)
	n := 0
	for _, l := range strings.Split(m.textarea.Value(), "\n") {
		n += 1 + lipgloss.Width(l)/w
	}
	return minInt(6, maxInt(1, n))
}

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Загрузка…"
	}
	parts := []string{m.renderHeader(), m.viewport.View()}
	switch {
	case m.picker != nil:
		parts = append(parts, m.renderPicker())
	case m.popupOpen:
		parts = append(parts, m.renderPopup())
	}
	parts = append(parts, m.renderInput(), m.renderStatus())
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *Model) currentModel() string {
	return m.Cfg.Backend.Type + "/" + m.Cfg.Backend.Model
}

func (m *Model) renderHeader() string {
	left := " " + StyleBrand.Render("◆ allan") + StyleDim.Render(" v"+m.Version)
	right := StyleMuted.Render(truncate(m.currentModel(), maxInt(10, m.width/2))) + " "
	gap := maxInt(1, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	line := left + strings.Repeat(" ", gap) + right
	rule := StyleDim.Render(strings.Repeat("─", m.width))
	return line + "\n" + rule
}

func (m *Model) renderWelcome(width int) string {
	keys := "нет"
	if n := m.providersWithKeys(); n > 0 {
		keys = fmt.Sprintf("%d (%s)", n, m.secretsKind())
	}
	row := func(k, v string) string {
		return StyleMuted.Render(padRight(k, 12)) + StyleText.Render(v)
	}
	tip := func(cmd, text string) string {
		return StyleKey.Render(padRight(cmd, 12)) + StyleMuted.Render(text)
	}
	lines := []string{
		StyleBrand.Render(Logo),
		"",
		row("модель", m.currentModel()),
		row("папка", abbrevPath(m.Workspace)),
		row("ключи API", keys),
		"",
		tip("/model", "выбрать модель: ↑↓, поиск, Enter"),
		tip("/api", "ключи провайдеров и свои эндпоинты"),
		tip("/help", "все команды"),
		tip("Tab", "дополнить команду"),
		tip("↑ ↓", "подсказки и история ввода"),
		tip("Esc", "остановить агента"),
		tip("Ctrl+C ×2", "выход"),
	}
	card := StyleWelcome.Render(strings.Join(lines, "\n"))
	if lipgloss.Width(card) > width {
		return strings.Join(lines, "\n")
	}
	return card
}

func (m *Model) providersWithKeys() int {
	n := 0
	for _, p := range m.Cfg.Providers {
		if p.APIKey != "" {
			n++
		}
	}
	return n
}

func (m *Model) renderInput() string {
	style := StyleInput
	if m.thinking {
		style = StyleInputBusy
	}
	prompt := StyleBrand.Render("› ")
	return style.Width(maxInt(10, m.width-2)).Render(prompt + m.textarea.View())
}

const popupMaxItems = 8

func (m *Model) renderPopup() string {
	start := 0
	if m.popupSelected >= popupMaxItems {
		start = m.popupSelected - popupMaxItems + 1
	}
	end := minInt(len(m.popupItems), start+popupMaxItems)
	nameW := 0
	for _, c := range m.popupItems[start:end] {
		nameW = maxInt(nameW, lipgloss.Width(c.Name))
	}
	nameW = minInt(nameW, maxInt(10, m.width/2))
	helpW := maxInt(10, m.width-nameW-10)
	lines := make([]string, 0, end-start+1)
	for i := start; i < end; i++ {
		c := m.popupItems[i]
		name := padRight(truncate(c.Name, nameW), nameW)
		help := truncate(c.Help, helpW)
		if i == m.popupSelected {
			lines = append(lines, StylePopupSelected.Render("› "+name)+"  "+StyleText.Render(help))
		} else {
			lines = append(lines, StyleMuted.Render("  "+name)+"  "+StyleDim.Render(help))
		}
	}
	if len(m.popupItems) > popupMaxItems {
		lines = append(lines, StyleDim.Render(fmt.Sprintf("  %d из %d · ↑↓ выбор · Tab вставить", m.popupSelected+1, len(m.popupItems))))
	}
	return StylePopup.Width(maxInt(10, m.width-2)).Render(strings.Join(lines, "\n"))
}

func (m *Model) renderStatus() string {
	left := " " + abbrevPath(m.Workspace)
	if m.ptyActive {
		target := "shell"
		if m.focus == FocusPTY {
			target = "Allan"
		}
		left += " · ⚙ " + m.ptyCmd + " · Tab: фокус " + target
	}
	right := fmt.Sprintf("инструменты %d · токены %s · /help ", m.Ag.Stats.ToolCalls, humanTokens(m.Ag.Stats.TokensIn+m.Ag.Stats.TokensOut))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		left = truncate(left, maxInt(5, m.width-lipgloss.Width(right)-1))
		gap = maxInt(1, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	}
	return StyleDim.Render(left + strings.Repeat(" ", gap) + right)
}

func humanTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	default:
		return strconv.Itoa(n)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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

// openModelPicker lists every model of every reachable provider. With
// provider set, the list is limited to it (used after picking a provider).
func (m *Model) openModelPicker(provider string) {
	if err := m.refreshModelCatalog(context.Background()); err != nil && len(m.modelCatalog) == 0 {
		m.appendMsg(Message{Kind: MsgWarn, Text: err.Error()})
		return
	}
	items := make([]pickItem, 0, len(m.modelCatalog))
	for _, c := range m.modelCatalog {
		if provider != "" && c.Provider != provider {
			continue
		}
		low := strings.ToLower(c.Model)
		aux := strings.Contains(low, "embed") || strings.Contains(low, "ocr") || strings.Contains(low, "rerank")
		detail := ""
		if aux {
			detail = "не для чата"
		} else if strings.HasSuffix(low, ":cloud") || strings.HasSuffix(low, "-cloud") {
			detail = "облако Ollama"
		} else if strings.HasSuffix(low, "-free") || strings.HasSuffix(low, ":free") {
			detail = "бесплатно"
		}
		items = append(items, pickItem{
			Label:   c.Model,
			Group:   c.Provider,
			Detail:  detail,
			Current: c.Provider == m.Cfg.Backend.Type && c.Model == m.Cfg.Backend.Model,
			Dim:     aux,
			Value:   c,
		})
	}
	if len(items) == 0 {
		m.appendMsg(Message{Kind: MsgWarn, Text: "Модели не найдены. Добавьте ключ: /api add <provider> <api_key>" + m.hugeCatalogHint()})
		return
	}
	title := "Выбор модели"
	if provider != "" {
		title += " · " + provider
	}
	m.openPicker(newPicker(title, items, func(it pickItem) tea.Cmd {
		c := it.Value.(modelChoice)
		m.switchBackendModel(c.Provider, c.Model)
		return nil
	}))
}

// openProviderPicker lists configured and local providers; picking one
// opens its model list.
func (m *Model) openProviderPicker() {
	items := []pickItem{}
	for _, name := range m.availableProviders() {
		detail := "локальный"
		if p, ok := m.Cfg.Providers[name]; ok {
			detail = p.BaseURL
			if p.APIKey == "" && !isLocalProvider(name) {
				detail += " · без ключа"
			}
		}
		items = append(items, pickItem{Label: name, Detail: detail, Current: name == m.Cfg.Backend.Type, Value: name})
	}
	m.openPicker(newPicker("Выбор провайдера", items, func(it pickItem) tea.Cmd {
		name := it.Value.(string)
		if hugeCatalogProvider(name) {
			m.appendMsg(Message{Kind: MsgSys, Text: m.hugeCatalogHint()})
			return nil
		}
		m.openModelPicker(name)
		return nil
	}))
}

// mouseSeq matches pieces of SGR mouse reports ("\x1b[<65;72;16M"). If the
// terminal splits a report across reads, the tail can arrive as plain key
// runes; those must not end up in the input box.
var mouseSeq = regexp.MustCompile(`^(\x1b)?\[?<?(\d+;\d+;\d+[Mm])+$|^(\[?<\d+;\d+;\d+[Mm])+`)

func isMouseGarbage(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyRunes || msg.Paste {
		return false
	}
	return mouseSeq.MatchString(string(msg.Runes))
}

var apiErrMessage = regexp.MustCompile(`"message"\s*:\s*"((?:[^"\\]|\\.)*)"`)

// humanError cuts the JSON envelope out of provider errors:
// `backend ollama error 402: {"error":{"message":"..."}}` → `ollama 402: ...`.
func humanError(text string) string {
	sub := apiErrMessage.FindStringSubmatch(text)
	if sub == nil {
		return text
	}
	msg := strings.ReplaceAll(sub[1], `\"`, `"`)
	prefix := text
	if i := strings.Index(text, "{"); i > 0 {
		prefix = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text[:i]), ":"))
	}
	prefix = strings.TrimPrefix(prefix, "backend error: ")
	return prefix + ": " + msg
}
