package tui

import "strings"

type SlashCommand struct {
	Name string
	Help string
}

var SlashCommands = []SlashCommand{
	{"/help", "Список команд"},
	{"/model", "Сменить модель: /model <name>"},
	{"/backend", "Сменить бэкенд: /backend <name>"},
	{"/tools", "Список доступных тулз"},
	{"/memory", "Состояние памяти"},
	{"/plan", "Текущий план агента (scratchpad)"},
	{"/skills", "Список навыков (или show/delete/export)"},
	{"/clear", "Очистить историю"},
	{"/config", "Показать текущий конфиг"},
	{"/quit", "Выход"},
}

func MatchSlash(prefix string) []SlashCommand {
	var out []SlashCommand
	for _, c := range SlashCommands {
		if strings.HasPrefix(c.Name, prefix) {
			out = append(out, c)
		}
	}
	return out
}
