package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/backend"
	"github.com/keytron/allan/agent/internal/i18n"
)

// thinkHelp — что значит каждый уровень /think.
var thinkHelp = map[string]string{
	"default": "как решит провайдер (параметр не передаётся)",
	"off":     "не размышлять, где модель это позволяет (у Claude — минимум)",
	"low":     "коротко: быстрее и дешевле",
	"medium":  "обычная глубина",
	"high":    "глубоко: дольше и дороже",
	"max":     "максимум, что умеет модель",
}

// handleThinkSlash: /think — выбор уровня, /think <уровень>, /think show.
func (m *Model) handleThinkSlash(args []string) tea.Cmd {
	if len(args) == 0 {
		m.openThinkPicker()
		return nil
	}
	switch a := strings.ToLower(args[0]); a {
	case "show", "last":
		if m.lastReasoning == "" {
			m.appendMsg(Message{Kind: MsgSys, Text: i18n.S("Рассуждений пока не было: модель их не отдаёт или ещё не отвечала")})
			return nil
		}
		m.appendMsg(Message{Kind: MsgSys, Text: i18n.S("Рассуждения модели:\n") + m.lastReasoning})
	case "default", "reset", "auto":
		m.setThink("")
	default:
		for _, l := range backend.EffortLevels {
			if a == l {
				m.setThink(l)
				return nil
			}
		}
		m.appendMsg(Message{Kind: MsgWarn, Text: i18n.S("Уровни: off, low, medium, high, max, default; /think show — последние рассуждения")})
	}
	return nil
}

func (m *Model) openThinkPicker() {
	cur := m.Cfg.ThinkFor(m.Cfg.Backend.Type, m.Cfg.Backend.Model)
	var items []pickItem
	for _, l := range append([]string{"default"}, backend.EffortLevels...) {
		items = append(items, pickItem{Label: l, Detail: i18n.S(thinkHelp[l]),
			Current: l == cur || l == "default" && cur == "", Value: l})
	}
	title := fmt.Sprintf(i18n.S("Размышления · %s/%s"), m.Cfg.Backend.Type, m.Cfg.Backend.Model)
	m.openPicker(newPicker(title, items, func(it pickItem) tea.Cmd {
		l := it.Value.(string)
		if l == "default" {
			l = ""
		}
		m.setThink(l)
		return nil
	}))
}

// setThink запоминает уровень для текущей модели и сразу отдаёт его бэкенду.
func (m *Model) setThink(level string) {
	key := m.Cfg.Backend.Type + "/" + m.Cfg.Backend.Model
	if level == "" {
		delete(m.Cfg.Think, key)
	} else {
		if m.Cfg.Think == nil {
			m.Cfg.Think = map[string]string{}
		}
		m.Cfg.Think[key] = level
	}
	if s, ok := m.Ag.Backend.(backend.EffortSetter); ok {
		s.SetEffort(level)
	}
	if err := config.Save(m.Cfg); err != nil {
		m.appendMsg(Message{Kind: MsgWarn, Text: i18n.S("Не удалось сохранить конфиг: ") + err.Error()})
	}
	switch {
	case level == "":
		m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf(i18n.S("Размышления для %s: как решит провайдер"), key)})
	case !m.thinkSupported():
		m.appendMsg(Message{Kind: MsgWarn, Text: fmt.Sprintf(i18n.S("Размышления для %s: %s — но этот провайдер уровень не принимает"), key, level)})
	default:
		m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf(i18n.S("Размышления для %s: %s. Если модель не умеет уровни, Allan молча отправит запрос без него"), key, level)})
	}
	m.recalcLayout()
}

func (m *Model) thinkSupported() bool {
	_, ok := m.Ag.Backend.(backend.EffortSetter)
	return ok
}
