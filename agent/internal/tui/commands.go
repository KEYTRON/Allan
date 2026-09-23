package tui

import "strings"

type SlashCommand struct {
	Name   string
	Help   string
	Insert string
}

var SlashCommands = []SlashCommand{
	{Name: "/help", Help: "Список команд"},
	{Name: "/model", Help: "Выбрать модель: /model или /model <номер|provider/model>"},
	{Name: "/api", Help: "API-ключи провайдеров: /api add|list|use|remove|refresh"},
	{Name: "/backend", Help: "Сменить бэкенд: /backend <name>"},
	{Name: "/tools", Help: "Список доступных тулз"},
	{Name: "/memory", Help: "Состояние памяти"},
	{Name: "/plan", Help: "Текущий план агента (scratchpad)"},
	{Name: "/skills", Help: "Список навыков (или show/delete/export)"},
	{Name: "/clear", Help: "Очистить историю"},
	{Name: "/config", Help: "Показать текущий конфиг"},
	{Name: "/quit", Help: "Выход"},
	{Name: "/exit", Help: "Выход"},
}

func MatchSlash(prefix string) []SlashCommand {
	var out []SlashCommand
	for _, c := range SlashCommands {
		if strings.HasPrefix(c.Name, prefix) {
			c.Insert = c.Name + " "
			out = append(out, c)
		}
	}
	return out
}
