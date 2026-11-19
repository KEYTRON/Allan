package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/i18n"
	"github.com/keytron/allan/agent/internal/mcp"
)

const mcpUsage = `allan mcp — внешние MCP-серверы: их инструменты получает агент

  allan mcp list                         серверы, состояние и инструменты
  allan mcp add <имя> <команда> [арг…]   stdio-сервер (запускается Allan)
  allan mcp add <имя> <url> [--sse]      HTTP-сервер (streamable HTTP или старый SSE)
  allan mcp rm <имя>                     удалить сервер
  allan mcp enable|disable <имя>         включить или выключить, не удаляя
  allan mcp import claude [--enable]     забрать серверы из ~/.claude.json
                                         (по умолчанию выключенными)

Серверы хранятся в ~/.allan/config.toml, секция [mcp.<имя>]: command, args,
env, url, transport, headers, tools (список разрешённых), disabled, timeout.
В env и headers можно ссылаться на переменные окружения: ${NAME}.
Инструменты видны агенту как mcp__<сервер>__<инструмент>; каждый занимает
место в контексте модели, поэтому включайте только нужные серверы.
`

func runMCP(args []string) int {
	if len(args) == 0 {
		fmt.Print(i18n.S(mcpUsage))
		return 0
	}
	cfg, _, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}
	if cfg.MCP == nil {
		cfg.MCP = map[string]config.MCPServer{}
	}
	save := func() int {
		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			return 1
		}
		return 0
	}
	switch args[0] {
	case "list", "ls":
		return mcpList(cfg)
	case "add":
		if len(args) < 3 {
			fmt.Fprint(os.Stderr, i18n.S(mcpUsage))
			return 2
		}
		name, target := args[1], args[2]
		var sc config.MCPServer
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			sc.URL = target
			for _, a := range args[3:] {
				if a == "--sse" {
					sc.Transport = "sse"
				}
			}
		} else {
			sc.Command = target
			sc.Args = args[3:]
		}
		cfg.MCP[name] = sc
		if rc := save(); rc != 0 {
			return rc
		}
		fmt.Printf(i18n.S("MCP-сервер %s добавлен. Проверить: allan mcp list\n"), name)
		return 0
	case "rm", "remove":
		if len(args) < 2 {
			fmt.Fprint(os.Stderr, i18n.S(mcpUsage))
			return 2
		}
		if _, ok := cfg.MCP[args[1]]; !ok {
			fmt.Fprintf(os.Stderr, i18n.S("нет MCP-сервера %s\n"), args[1])
			return 1
		}
		delete(cfg.MCP, args[1])
		return save()
	case "enable", "disable":
		if len(args) < 2 {
			fmt.Fprint(os.Stderr, i18n.S(mcpUsage))
			return 2
		}
		for _, name := range args[1:] {
			sc, ok := cfg.MCP[name]
			if !ok {
				fmt.Fprintf(os.Stderr, i18n.S("нет MCP-сервера %s\n"), name)
				return 1
			}
			sc.Disabled = args[0] == "disable"
			cfg.MCP[name] = sc
		}
		return save()
	case "import":
		if len(args) < 2 || args[1] != "claude" {
			fmt.Fprint(os.Stderr, i18n.S(mcpUsage))
			return 2
		}
		enable := len(args) > 2 && args[2] == "--enable"
		n, skipped, err := importClaude(cfg, enable)
		if err != nil {
			fmt.Fprintf(os.Stderr, "import: %v\n", err)
			return 1
		}
		if rc := save(); rc != 0 {
			return rc
		}
		fmt.Printf(i18n.S("Импортировано из Claude: %d (уже были: %d).\n"), n, skipped)
		if !enable && n > 0 {
			fmt.Println(i18n.S("Серверы добавлены выключенными. Включить нужные: allan mcp enable <имя> …"))
		}
		return 0
	default:
		fmt.Fprint(os.Stderr, i18n.S(mcpUsage))
		return 2
	}
}

func mcpList(cfg *config.Config) int {
	if len(cfg.MCP) == 0 {
		fmt.Println(i18n.S("MCP-серверов нет. Добавить: allan mcp add <имя> <команда|url>, или allan mcp import claude"))
		return 0
	}
	m := mcp.Shared(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	m.Wait(ctx)
	cancel()
	defer mcp.Shutdown()
	for _, st := range m.Status() {
		line := fmt.Sprintf("%-22s %-5s %s", st.Name, st.Transport, i18n.S(string(st.State)))
		if st.State == mcp.StateReady {
			line += fmt.Sprintf(i18n.S(" · инструментов: %d"), len(st.Tools))
		}
		if st.Error != "" {
			line += " · " + st.Error
		}
		fmt.Println(line)
		if len(st.Tools) > 0 {
			names := append([]string(nil), st.Tools...)
			sort.Strings(names)
			fmt.Println("    " + strings.Join(names, ", "))
		}
	}
	return 0
}

// importClaude переносит серверы пользовательского уровня из ~/.claude.json.
func importClaude(cfg *config.Config, enable bool) (added, skipped int, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, 0, err
	}
	raw, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		return 0, 0, err
	}
	var doc struct {
		MCPServers map[string]struct {
			Type    string            `json:"type"`
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return 0, 0, err
	}
	for name, s := range doc.MCPServers {
		if _, ok := cfg.MCP[name]; ok {
			skipped++
			continue
		}
		sc := config.MCPServer{
			Command: s.Command, Args: s.Args, Env: s.Env,
			URL: s.URL, Headers: s.Headers, Disabled: !enable,
		}
		if strings.EqualFold(s.Type, "sse") {
			sc.Transport = "sse"
		}
		if sc.Command == "" && sc.URL == "" {
			skipped++
			continue
		}
		cfg.MCP[name] = sc
		added++
	}
	return added, skipped, nil
}
