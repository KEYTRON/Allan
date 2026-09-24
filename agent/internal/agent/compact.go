package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/keytron/allan/agent/internal/backend"
)

const compactPrompt = `Сожми разговор пользователя с агентом Allan в сводку для продолжения работы.
Сохрани: цель пользователя, принятые решения, изменённые файлы и команды, найденные факты,
что осталось сделать. Убери повторы, приветствия и сырой вывод инструментов.
Пиши на языке разговора, списком, не длиннее 300 слов.`

// compactKeepTail is how many latest messages stay verbatim after /compact,
// so the model still sees the exact last exchange.
const compactKeepTail = 2

// Compact replaces the conversation history with a model-written summary and
// returns it. Token counts of the summary call are added to Stats.
func (a *Agent) Compact(ctx context.Context) (string, error) {
	if len(a.History) <= compactKeepTail {
		return "", fmt.Errorf("история слишком короткая, сжимать нечего")
	}
	head := a.History[:len(a.History)-compactKeepTail]
	tail := append([]backend.Message(nil), a.History[len(a.History)-compactKeepTail:]...)
	// Do not start the kept tail with orphaned tool results.
	for len(tail) > 0 && tail[0].Role == backend.RoleTool {
		head = append(head, tail[0])
		tail = tail[1:]
	}

	var sb strings.Builder
	for _, m := range head {
		text := m.Content
		if m.Role == backend.RoleTool && len(text) > 1500 {
			text = text[:1500] + "…"
		}
		for _, tc := range m.ToolCalls {
			text += fmt.Sprintf("\n[вызов %s %v]", tc.Name, tc.Arguments)
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		fmt.Fprintf(&sb, "### %s\n%s\n\n", m.Role, text)
	}

	resp, err := a.Backend.Chat(ctx, []backend.Message{
		{Role: backend.RoleSystem, Content: compactPrompt},
		{Role: backend.RoleUser, Content: sb.String()},
	}, nil)
	if err != nil {
		return "", err
	}
	a.Stats.TokensIn += resp.TokensIn
	a.Stats.TokensOut += resp.TokensOut
	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return "", fmt.Errorf("модель вернула пустую сводку")
	}

	a.History = append([]backend.Message{
		{Role: backend.RoleUser, Content: "Сводка предыдущей части разговора (сжата командой /compact):\n\n" + summary},
		{Role: backend.RoleAssistant, Content: "Понял, продолжаю с учётом сводки."},
	}, tail...)
	a.Scratch = &Scratchpad{}
	return summary, nil
}
