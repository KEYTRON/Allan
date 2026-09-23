package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/agent"
	"github.com/keytron/allan/agent/internal/backend"
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

	popupOpen     bool
	popupItems    []SlashCommand
	popupSelected int
	modelCatalog  []modelChoice

	focus     FocusMode
	ptyActive bool
	ptyCmd    string
	ptyOutput string

	thinking  bool
	startedAt time.Time
}

type modelChoice struct {
	Provider string
	Model    string
}

func New(cfg *config.Config, ag *agent.Agent, workspace, version string) *Model {
	ta := textarea.New()
	ta.Placeholder = "Ask Allan..."
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
		m.updatePopup()
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
				insert := m.popupItems[m.popupSelected].Insert
				if insert == "" {
					insert = m.popupItems[m.popupSelected].Name + " "
				}
				m.textarea.SetValue(insert)
				m.textarea.SetCursor(len(insert))
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
		m.history = append(m.history, redactSensitiveInput(val))
		m.historyIdx = len(m.history)
		return m.submit(val)
	}
	return nil
}

func (m *Model) updatePopup() {
	if m.thinking {
		m.popupOpen = false
		return
	}
	val := m.textarea.Value()
	if strings.HasPrefix(val, "/") {
		matches := m.completionsFor(val)
		if len(matches) > 0 {
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
	if sub == "add" && (len(parts) == 2 || (len(parts) == 3 && !endsWithSpace)) {
		prefix := ""
		if len(parts) == 3 {
			prefix = parts[2]
		}
		return providerCompletions(prefix, "/api add ")
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

func redactSensitiveInput(input string) string {
	parts := strings.Fields(input)
	if len(parts) >= 4 && parts[0] == "/api" && (parts[1] == "add" || parts[1] == "set") {
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

func (m *Model) handleSlash(input string) tea.Cmd {
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
			m.appendMsg(Message{Kind: MsgSys, Text: "Текущий бэкенд: " + m.Cfg.Backend.Type})
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
			"/api list",
			"/api add <provider> <api_key> [base_url]",
			"/api use <provider>",
			"/api remove <provider>",
			"/api refresh",
			"",
			"Провайдеры: anthropic, openai, openrouter, deepseek, xai/grok, groq, ollama, llamacpp, lmstudio.",
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
		} else {
			base = defaultBaseURL(name)
		}
		if m.Cfg.Providers == nil {
			m.Cfg.Providers = map[string]config.ProviderConfig{}
		}
		m.Cfg.Providers[name] = config.ProviderConfig{
			Type:    providerType(name),
			BaseURL: base,
			APIKey:  args[2],
		}
		if err := config.Save(m.Cfg); err != nil {
			m.appendMsg(Message{Kind: MsgError, Text: err.Error()})
			return
		}
		m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf("API-ключ для %s сохранён. Выполните /api refresh или /model, чтобы обновить список моделей.", name)})
	case "use":
		if len(args) < 2 {
			m.appendMsg(Message{Kind: MsgWarn, Text: "/api use <provider>"})
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
		if err := config.Save(m.Cfg); err != nil {
			m.appendMsg(Message{Kind: MsgError, Text: err.Error()})
			return
		}
		m.appendMsg(Message{Kind: MsgSys, Text: "Провайдер удалён: " + name})
	case "refresh":
		if err := m.refreshModelCatalog(context.Background()); err != nil {
			m.appendMsg(Message{Kind: MsgWarn, Text: err.Error()})
		}
		m.appendMsg(Message{Kind: MsgSys, Text: m.renderModelCatalog()})
	default:
		m.appendMsg(Message{Kind: MsgWarn, Text: "Подкоманды /api: list, add, use, remove, refresh"})
	}
}

func (m *Model) handleModel(args []string) {
	if len(args) == 0 {
		if err := m.refreshModelCatalog(context.Background()); err != nil {
			m.appendMsg(Message{Kind: MsgWarn, Text: err.Error()})
		}
		m.appendMsg(Message{Kind: MsgSys, Text: m.renderModelCatalog()})
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
		return "Модели не найдены. Для API-провайдера добавьте ключ: /api add <provider> <api_key>"
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
	return strings.TrimRight(sb.String(), "\n")
}

func (m *Model) renderProviders() string {
	if len(m.Cfg.Providers) == 0 {
		return "API-провайдеры не настроены. Добавить: /api add <provider> <api_key> [base_url]"
	}
	names := make([]string, 0, len(m.Cfg.Providers))
	for name := range m.Cfg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	var sb strings.Builder
	for _, name := range names {
		p := m.Cfg.Providers[name]
		key := "missing"
		if p.APIKey != "" {
			key = "set"
		}
		active := ""
		if name == m.Cfg.Backend.Type {
			active = " (active)"
		}
		sb.WriteString(fmt.Sprintf("%s%s: type=%s base_url=%s api_key=%s\n", name, active, p.Type, p.BaseURL, key))
	}
	return strings.TrimRight(sb.String(), "\n")
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
	switch provider {
	case "openrouter", "deepseek", "xai", "grok", "groq":
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

func normalizeProvider(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "grok" {
		return "xai"
	}
	return name
}

func providerType(name string) string {
	switch normalizeProvider(name) {
	case "openrouter", "deepseek", "xai", "groq":
		return "openai"
	default:
		return normalizeProvider(name)
	}
}

func defaultBaseURL(name string) string {
	switch normalizeProvider(name) {
	case "openai":
		return "https://api.openai.com/v1"
	case "anthropic":
		return "https://api.anthropic.com"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "xai":
		return "https://api.x.ai/v1"
	case "groq":
		return "https://api.groq.com/openai/v1"
	case "ollama":
		return "http://localhost:11434/v1"
	case "llamacpp":
		return "http://localhost:8080/v1"
	case "lmstudio":
		return "http://localhost:1234/v1"
	default:
		return ""
	}
}

func isLocalProvider(name string) bool {
	switch normalizeProvider(name) {
	case "ollama", "llamacpp", "lmstudio":
		return true
	default:
		return false
	}
}

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
	return filterCompletions(prefix, []SlashCommand{
		{Name: "anthropic", Help: "Claude API, base https://api.anthropic.com", Insert: commandPrefix + "anthropic "},
		{Name: "openai", Help: "OpenAI API, base https://api.openai.com/v1", Insert: commandPrefix + "openai "},
		{Name: "openrouter", Help: "OpenRouter, OpenAI-compatible", Insert: commandPrefix + "openrouter "},
		{Name: "deepseek", Help: "DeepSeek, OpenAI-compatible", Insert: commandPrefix + "deepseek "},
		{Name: "xai", Help: "Grok / xAI, base https://api.x.ai/v1", Insert: commandPrefix + "xai "},
		{Name: "grok", Help: "alias for xai", Insert: commandPrefix + "grok "},
		{Name: "groq", Help: "GroqCloud, not Grok", Insert: commandPrefix + "groq "},
	})
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

	pad := strings.Repeat(" ", maxInt(2, m.width-logoWidth-len(infoText)-2))
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
