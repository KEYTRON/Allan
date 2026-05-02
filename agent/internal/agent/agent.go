package agent

import (
	"context"
	"encoding/json"
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
	ToolCalls     int
	SuccessCalls  int
	FailedCalls   int
	TokensIn      int
	TokensOut     int
}

type Agent struct {
	Cfg      *config.Config
	Backend  backend.Backend
	Tools    *tools.Registry
	Memory   *memory.Memory
	Skills   *skills.Engine
	Vector   vector.Store
	Workspace string
	SessionID string
	History  []backend.Message
	Stats    Stats
	Scratch  *Scratchpad
}

func New(cfg *config.Config, b backend.Backend, reg *tools.Registry, mem *memory.Memory, sk *skills.Engine, vec vector.Store, workspace string) *Agent {
	return &Agent{
		Cfg:       cfg,
		Backend:   b,
		Tools:     reg,
		Memory:    mem,
		Skills:    sk,
		Vector:    vec,
		Workspace: workspace,
		SessionID: uuid.NewString(),
		Scratch:   &Scratchpad{},
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
	defer close(events)
	startTime := time.Now()
	startToolCalls := a.Stats.ToolCalls

	if a.Memory != nil {
		_ = a.Memory.AppendMessage(ctx, a.SessionID, "user", userInput)
	}

	a.History = append(a.History, backend.Message{Role: backend.RoleUser, Content: userInput})

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

func (a *Agent) maybeSaveSkill(ctx context.Context, userInput string, started time.Time, startTC int, usedTools []string, events chan<- Event) {
	if a.Skills == nil {
		return
	}
	tcCount := a.Stats.ToolCalls - startTC
	dur := time.Since(started)
	minTC := a.Cfg.Agent.SkillMinToolCalls
	if minTC <= 0 {
		minTC = 3
	}
	if tcCount < minTC && dur < 10*time.Second {
		return
	}

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
	skill := &skills.Skill{
		Name:         raw.Name,
		Description:  raw.Description,
		Trigger:      raw.Trigger,
		Solution:     raw.Solution,
		ToolSequence: raw.ToolSequence,
	}
	if err := a.Skills.Save(ctx, skill); err == nil {
		events <- Event{Kind: "skill_saved", Skill: skill}
	}
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
}
