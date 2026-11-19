package tui

import (
	"fmt"
	"strings"

	"github.com/keytron/allan/agent/internal/i18n"
	"github.com/keytron/allan/agent/internal/mcp"
)

// mcpStatusText — ответ на /mcp: какие серверы подключены, сколько весят их
// схемы и что модель видит прямо сейчас.
func (m *Model) mcpStatusText() string {
	if len(m.Cfg.MCP) == 0 {
		return i18n.S("MCP-серверов нет. Добавить: allan mcp add <имя> <команда|url>, или allan mcp import claude")
	}
	hidden := map[string]int{}
	if m.Ag != nil && m.Ag.Tools != nil {
		for _, t := range m.Ag.Tools.Deferred() {
			if g, ok := t.(interface{ Group() string }); ok {
				hidden[g.Group()]++
			}
		}
	}
	var sb strings.Builder
	sb.WriteString(i18n.S("MCP-серверы (настройка: allan mcp, ~/.allan/config.toml)\n"))
	for _, st := range mcp.Shared(m.Cfg).Status() {
		line := fmt.Sprintf("  %-20s %-5s %-8s %s", st.Name, st.Transport, st.Load, i18n.S(string(st.State)))
		if st.State == mcp.StateReady {
			line += fmt.Sprintf(i18n.S(" · инструментов: %d, ~%d токенов"), len(st.Tools), st.Tokens)
			if n := hidden[st.Name]; n > 0 {
				line += fmt.Sprintf(i18n.S(" · скрыто до load_tools: %d"), n)
			}
		}
		if st.Error != "" {
			line += " · " + st.Error
		}
		sb.WriteString(line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
