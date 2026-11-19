package tui

import (
	"fmt"
	"strings"

	"github.com/keytron/allan/agent/internal/i18n"
	"github.com/keytron/allan/agent/internal/mcp"
)

// mcpStatusText — ответ на /mcp: какие серверы подключены и что они дали агенту.
func (m *Model) mcpStatusText() string {
	if len(m.Cfg.MCP) == 0 {
		return i18n.S("MCP-серверов нет. Добавить: allan mcp add <имя> <команда|url>, или allan mcp import claude")
	}
	var sb strings.Builder
	sb.WriteString(i18n.S("MCP-серверы (настройка: allan mcp, ~/.allan/config.toml)\n"))
	for _, st := range mcp.Shared(m.Cfg).Status() {
		line := fmt.Sprintf("  %-20s %-5s %s", st.Name, st.Transport, i18n.S(string(st.State)))
		if st.State == mcp.StateReady {
			line += fmt.Sprintf(i18n.S(" · инструментов: %d"), len(st.Tools))
		}
		if st.Error != "" {
			line += " · " + st.Error
		}
		sb.WriteString(line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
