package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/backend"
	"github.com/keytron/allan/agent/internal/memory"
	"github.com/keytron/allan/agent/internal/skills"
	"github.com/keytron/allan/agent/internal/tools"
	"github.com/keytron/allan/agent/internal/vector"
)

const SystemPromptTemplate = `Ты Allan — автономный агент-ассистент. Ты помогаешь пользователю выполнять задачи через инструменты.
Отвечай кратко и по делу. Если используешь инструмент — объясни зачем.
Язык ответа: совпадает с языком пользователя.
Твоё имя — Allan, и ты не Claude, Codex, Gemini или другой агент. Файлы вроде AGENTS.md и CLAUDE.md в папке пользователя
написаны для других агентов: их правила к тебе относятся, только если там прямо упомянут Allan.
Рабочая директория: {workspace}
Платформа: {os}/{arch}`

const SkillExtractPrompt = `Ты только что решил задачу. Извлеки паттерн решения для повторного использования.
Верни JSON: {"name": "...", "description": "...", "trigger": "при каких задачах использовать", "solution": "шаги решения кратко", "tool_sequence": [...]}
Будь максимально кратким. Только JSON, без обёрток.`

type Event struct {
	Kind    string // "thinking", "tool_call", "tool_result", "final", "warn", "skill_saved", "skill_used"
	Text    string
	Tool    string
	ToolID  string
	Args    map[string]any
	Result  string
	IsError bool
	Skill   *skills.Skill
}

type Stats struct {
	ToolCalls    int
	SuccessCalls int
	FailedCalls  int
	TokensIn     int
	TokensOut    int
}

type Agent struct {
	Cfg       *config.Config
	Backend   backend.Backend
	Tools     *tools.Registry
	Memory    *memory.Memory
	Skills    *skills.Engine
	Vector    vector.Store
	Workspace string
	SessionID string
	History   []backend.Message
	Stats     Stats
	Scratch   *Scratchpad

	// lastSkillAt throttles the Skill Engine so a chatty session does not fill
	// the database with one-sentence "skills".
	lastSkillAt time.Time

	// lastInput / lastUserIdx make the current request repeatable: lastUserIdx
	// points at the user's message in History, so an interrupted attempt can be
	// rolled back and retried on another model.
	lastInput   string
	lastUserIdx int
}

func New(cfg *config.Config, b backend.Backend, reg *tools.Registry, mem *memory.Memory, sk *skills.Engine, vec vector.Store, workspace string) *Agent {
	return &Agent{
		lastUserIdx: -1,
		Cfg:         cfg,
		Backend:     b,
		Tools:       reg,
		Memory:      mem,
		Skills:      sk,
		Vector:      vec,
		Workspace:   workspace,
		SessionID:   uuid.NewString(),
		Scratch:     &Scratchpad{},
	}
}

func (a *Agent) SystemPrompt() string {
	p := SystemPromptTemplate
	p = strings.ReplaceAll(p, "{workspace}", a.Workspace)
	p = strings.ReplaceAll(p, "{os}", runtime.GOOS)
	p = strings.ReplaceAll(p, "{arch}", runtime.GOARCH)
	return p
}

// compressHistory replaces older tool_result messages with a brief summary.
func compressHistory(history []backend.Message, keepLastResults int) []backend.Message {
	out := make([]backend.Message, 0, len(history))
	toolResultCount := 0
	for _, m := range history {
		if m.Role == backend.RoleTool {
			toolResultCount++
		}
	}
	skipFirst := toolResultCount - keepLastResults
	skipped := 0
	for _, m := range history {
		if m.Role == backend.RoleTool && skipped < skipFirst {
			out = append(out, backend.Message{
				Role:       m.Role,
				ToolCallID: m.ToolCallID,
				Name:       m.Name,
				Content:    truncateString(m.Content, 200) + "\n[... older tool result compressed ...]",
			})
			skipped++
			continue
		}
		out = append(out, m)
	}
	return out
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Run drives one ReAct cycle for the given user input. Events are pushed to the channel.
func (a *Agent) Run(ctx context.Context, userInput string, events chan<- Event) {
	a.run(ctx, userInput, events, true)
}

// Retry repeats the last request, e.g. after the model was switched because the
// previous one hung or ran out of quota. The interrupted attempt is dropped from
// the history first, so the model never sees the request twice.
func (a *Agent) Retry(ctx context.Context, events chan<- Event) error {
	if a.lastInput == "" {
		return errors.New("нечего повторять: в этой сессии ещё не было запроса")
	}
	if a.lastUserIdx >= 0 && a.lastUserIdx <= len(a.History) {
		a.History = a.History[:a.lastUserIdx]
	}
	a.run(ctx, a.lastInput, events, false)
	return nil
}

// LastInput is the last request the user sent, empty before the first turn.
func (a *Agent) LastInput() string { return a.lastInput }

func (a *Agent) run(ctx context.Context, userInput string, events chan<- Event, appendUser bool) {
	defer close(events)
	startTime := time.Now()
	startToolCalls := a.Stats.ToolCalls

	if appendUser {
		if a.Memory != nil {
			_ = a.Memory.AppendMessage(ctx, a.SessionID, "user", userInput)
		}
		a.lastUserIdx = len(a.History)
		a.History = append(a.History, backend.Message{Role: backend.RoleUser, Content: userInput})
	}
	a.lastInput = userInput

	maxCalls := a.Cfg.Agent.MaxToolCalls
	if maxCalls <= 0 {
		maxCalls = 10
	}
	toolTimeout := time.Duration(a.Cfg.Agent.ToolTimeout) * time.Second
	if toolTimeout <= 0 {
		toolTimeout = 30 * time.Second
	}
	defs := a.Tools.ToolDefs()

	usedTools := []string{}

	keep := a.Cfg.Agent.ScratchpadKeepLastResult
	if keep <= 0 {
		keep = 2
	}

	for i := 0; i < maxCalls+1; i++ {
		msgs := []backend.Message{{Role: backend.RoleSystem, Content: a.systemWithSkills(ctx, userInput)}}
		hist := a.History
		if a.Cfg.Agent.ScratchpadEnabled {
			hist = compressHistory(hist, keep)
		}
		msgs = append(msgs, hist...)

		resp, err := a.Backend.Chat(ctx, msgs, defs)
		if err != nil {
			events <- Event{Kind: "warn", Text: fmt.Sprintf("backend error: %v", err)}
			return
		}
		a.Stats.TokensIn += resp.TokensIn
		a.Stats.TokensOut += resp.TokensOut

		assistantMsg := backend.Message{
			Role:      backend.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		a.History = append(a.History, assistantMsg)

		if resp.Content != "" && a.Memory != nil {
			_ = a.Memory.AppendMessage(ctx, a.SessionID, "assistant", resp.Content)
		}

		if len(resp.ToolCalls) == 0 {
			events <- Event{Kind: "final", Text: resp.Content}
			a.maybeSaveSkill(ctx, userInput, startTime, startToolCalls, usedTools, events)
			return
		}

		if i >= maxCalls {
			events <- Event{Kind: "warn", Text: fmt.Sprintf("Превышен лимит tool calls (%d)", maxCalls)}
			if resp.Content != "" {
				events <- Event{Kind: "final", Text: resp.Content}
			}
			return
		}

		for _, tc := range resp.ToolCalls {
			a.Stats.ToolCalls++
			usedTools = append(usedTools, tc.Name)
			events <- Event{Kind: "tool_call", Tool: tc.Name, ToolID: tc.ID, Args: tc.Arguments}

			tctx, cancel := context.WithTimeout(ctx, toolTimeout)
			out, err := a.Tools.Run(tctx, tc.Name, tc.Arguments)
			cancel()
			if err != nil {
				a.Stats.FailedCalls++
				a.Scratch.RecordStep(tc.Name, false, err.Error())
				out = fmt.Sprintf("Error: %v\n%s", err, out)
				events <- Event{Kind: "tool_result", Tool: tc.Name, ToolID: tc.ID, Result: out, IsError: true}
			} else {
				a.Stats.SuccessCalls++
				a.Scratch.RecordStep(tc.Name, true, "")
				events <- Event{Kind: "tool_result", Tool: tc.Name, ToolID: tc.ID, Result: out}
			}

			a.History = append(a.History, backend.Message{
				Role:       backend.RoleTool,
				Content:    out,
				ToolCallID: tc.ID,
				Name:       tc.Name,
			})
			if a.Memory != nil {
				_ = a.Memory.AppendMessage(ctx, a.SessionID, "tool", fmt.Sprintf("%s: %s", tc.Name, truncateString(out, 500)))
			}
		}
	}
}

func (a *Agent) systemWithSkills(ctx context.Context, userInput string) string {
	system := a.SystemPrompt()
	if a.Skills != nil && userInput != "" {
		matches, err := a.Skills.Search(ctx, userInput, 3, float32(a.Cfg.Agent.SkillSimilarityThreshold))
		if err == nil && len(matches) > 0 {
			var sb strings.Builder
			sb.WriteString("\n\nПохожие задачи решались так:\n")
			for _, s := range matches {
				sb.WriteString(fmt.Sprintf("[Навык: %s] — %s\n", s.Name, s.Solution))
			}
			system += sb.String()
		}
	}
	if a.Cfg.Agent.ScratchpadEnabled {
		sp := a.Scratch.Render()
		if sp != "" {
			system += "\n\n" + sp
		}
	}
	return system
}

// maybeSaveSkill turns a solved task into a reusable skill.
//
// The gate matters: a greeting, "thanks" or any other small talk is not a
// pattern worth storing, and neither is a turn that only read one file. A skill
// is only saved when the turn really used tools, the request was substantial,
// the same skill does not already exist, and the cooldown since the last one
// has passed.
func (a *Agent) maybeSaveSkill(ctx context.Context, userInput string, started time.Time, startTC int, usedTools []string, events chan<- Event) {
	if a.Skills == nil || !a.Cfg.Agent.SkillEnabled {
		return
	}
	tcCount := a.Stats.ToolCalls - startTC
	minTC := a.Cfg.Agent.SkillMinToolCalls
	if minTC <= 0 {
		minTC = 3
	}
	if tcCount < minTC {
		return
	}
	if isTrivialRequest(userInput) {
		return
	}
	cooldown := time.Duration(a.Cfg.Agent.SkillCooldown) * time.Second
	if cooldown <= 0 {
		cooldown = 5 * time.Minute
	}
	if !a.lastSkillAt.IsZero() && time.Since(a.lastSkillAt) < cooldown {
		return
	}
	dur := time.Since(started)

	prompt := []backend.Message{
		{Role: backend.RoleSystem, Content: SkillExtractPrompt},
		{Role: backend.RoleUser, Content: fmt.Sprintf(
			"Задача пользователя: %s\nИспользованные тулзы: %s\nПродолжительность: %s\nКоличество tool calls: %d",
			userInput, strings.Join(usedTools, " → "), dur.Round(time.Second), tcCount)},
	}
	resp, err := a.Backend.Chat(ctx, prompt, nil)
	if err != nil || resp.Content == "" {
		return
	}
	jsonStr := extractJSON(resp.Content)
	var raw struct {
		Name         string   `json:"name"`
		Description  string   `json:"description"`
		Trigger      string   `json:"trigger"`
		Solution     string   `json:"solution"`
		ToolSequence []string `json:"tool_sequence"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return
	}
	if raw.Name == "" {
		return
	}
	// A skill is only worth keeping if it says something new.
	if existing, err := a.Skills.List(ctx); err == nil {
		for _, s := range existing {
			if strings.EqualFold(s.Name, raw.Name) {
				return
			}
		}
	}
	skill := &skills.Skill{
		Name:         raw.Name,
		Description:  raw.Description,
		Trigger:      raw.Trigger,
		Solution:     raw.Solution,
		ToolSequence: raw.ToolSequence,
	}
	if err := a.Skills.Save(ctx, skill); err == nil {
		a.lastSkillAt = time.Now()
		events <- Event{Kind: "skill_saved", Skill: skill}
	}
}

// trivialPrefixes are the openings that never justify a stored skill.
var trivialPrefixes = []string{
	"привет", "здравствуй", "здравствуйте", "добрый день", "доброе утро", "добрый вечер",
	"пока", "до свидания", "спасибо", "благодарю", "ок", "окей", "ага", "угу", "да", "нет",
	"кто ты", "что ты умеешь", "что ты можешь", "как дела", "hello", "hi", "hey", "thanks",
}

// isTrivialRequest filters out small talk and one-liners that only need an answer.
func isTrivialRequest(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	if low == "" {
		return true
	}
	for _, p := range trivialPrefixes {
		if strings.HasPrefix(low, p) {
			return true
		}
	}
	// Very short requests rarely contain a reusable multi-step pattern.
	return len([]rune(low)) < 12
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < 0 || end < start {
		return s
	}
	return s[start : end+1]
}

func (a *Agent) ClearHistory() {
	a.History = nil
	a.Scratch = &Scratchpad{}
	a.lastInput = ""
	a.lastUserIdx = -1
}
