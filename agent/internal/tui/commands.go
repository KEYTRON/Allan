package tui

import (
	"fmt"
	"strings"
)

type SlashCommand struct {
	Name   string
	Help   string
	Insert string
}

var SlashCommands = []SlashCommand{
	{Name: "/help", Help: "Список команд"},
	{Name: "/model", Help: "Выбрать модель из списка (↑↓, поиск, Enter) или /model <provider/model>"},
	{Name: "/api", Help: "API-ключи и эндпоинты: /api add|key|endpoint|list|use|remove|refresh"},
	{Name: "/backend", Help: "Выбрать провайдера, затем его модель"},
	{Name: "/tools", Help: "Список доступных тулз"},
	{Name: "/memory", Help: "Состояние памяти"},
	{Name: "/plan", Help: "Текущий план агента (scratchpad)"},
	{Name: "/skills", Help: "Список навыков (или show/delete/export)"},
	{Name: "/compact", Help: "Сжать историю в сводку, чтобы экономить контекст и токены"},
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

// helpText is the /help output: commands, then keys.
func helpText() string {
	var sb strings.Builder
	sb.WriteString("Команды\n")
	for _, c := range SlashCommands {
		if c.Name == "/exit" {
			continue
		}
		sb.WriteString(fmt.Sprintf("  %-10s %s\n", c.Name, c.Help))
	}
	sb.WriteString("\nКлавиши\n")
	for _, k := range [][2]string{
		{"Enter", "отправить; в подсказке — выполнить выбранную команду"},
		{"Tab", "дополнить команду; при запущенном shell — переключить фокус"},
		{"↑ ↓", "выбор в подсказке или списке, иначе история ввода"},
		{"Esc", "закрыть подсказку или список; пока агент работает — остановить его"},
		{"PgUp PgDn", "прокрутка, колесо мыши тоже работает"},
		{"Shift+мышь", "выделить и скопировать текст"},
		{"Ctrl+C", "очистить ввод; дважды на пустом вводе — выход"},
	} {
		sb.WriteString(fmt.Sprintf("  %-10s %s\n", k[0], k[1]))
	}
	sb.WriteString("\nКлючи вне TUI: allan key set <provider>. Справка по запуску: allan --help")
	return sb.String()
}
