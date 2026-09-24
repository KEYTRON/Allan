package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/backend"
	"golang.org/x/term"
)

const keyUsage = `allan key — API-ключи провайдеров (хранятся в системном keyring, не в config.toml)

  allan key set <provider> [base_url]   ввести ключ скрытым вводом (или из stdin)
  allan key rm <provider>               удалить провайдер и его ключ
  allan key list                        провайдеры, эндпоинты и наличие ключей

Известные провайдеры: %s
Для своего OpenAI-совместимого эндпоинта укажите base_url.
`

// runKey handles `allan key ...`. Keys are read with echo off so they never
// land in shell history or the TUI input history.
func runKey(args []string) int {
	providers := knownProviders()
	if len(args) == 0 {
		fmt.Printf(keyUsage, strings.Join(providers, ", "))
		return 0
	}
	cfg, _, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}
	switch args[0] {
	case "list", "ls":
		names := make([]string, 0, len(cfg.Providers))
		for name := range cfg.Providers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			p := cfg.Providers[name]
			key := "нет"
			if p.APIKey != "" {
				key = "есть"
			}
			fmt.Printf("%-14s type=%-9s base_url=%s ключ=%s\n", name, p.Type, p.BaseURL, key)
		}
		if len(names) == 0 {
			fmt.Println("Провайдеры не настроены.")
		}
		fmt.Println("Хранилище ключей:", cfg.Secrets.Kind())
	case "set", "add":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "allan key set <provider> [base_url]")
			return 2
		}
		name := backend.NormalizeProvider(args[1])
		p := cfg.Providers[name]
		if len(args) >= 3 {
			p.BaseURL = strings.TrimRight(args[2], "/")
		} else if p.BaseURL == "" {
			p.BaseURL = backend.DefaultBaseURL(name)
		}
		if p.BaseURL == "" {
			fmt.Fprintf(os.Stderr, "%s не входит в список известных провайдеров — укажите base_url: allan key set %s <base_url>\n", name, name)
			return 2
		}
		if p.Type == "" {
			p.Type = backend.ProviderType(name)
		}
		key, err := readSecret(fmt.Sprintf("API-ключ для %s: ", name))
		if err != nil {
			fmt.Fprintf(os.Stderr, "ввод ключа: %v\n", err)
			return 1
		}
		if key == "" {
			fmt.Fprintln(os.Stderr, "пустой ключ, ничего не сохранено")
			return 1
		}
		if err := cfg.Secrets.Set(name, key); err != nil {
			fmt.Fprintf(os.Stderr, "не удалось сохранить ключ в %s: %v\n", cfg.Secrets.Kind(), err)
			return 1
		}
		p.APIKey = key
		cfg.Providers[name] = p
		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			return 1
		}
		fmt.Printf("Ключ для %s сохранён (%s), base_url=%s\n", name, cfg.Secrets.Kind(), p.BaseURL)
	case "rm", "remove", "delete":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "allan key rm <provider>")
			return 2
		}
		name := backend.NormalizeProvider(args[1])
		delete(cfg.Providers, name)
		if err := cfg.Secrets.Delete(name); err != nil {
			fmt.Fprintf(os.Stderr, "ключ не удалён: %v\n", err)
			return 1
		}
		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			return 1
		}
		fmt.Println("Удалено:", name)
	default:
		fmt.Printf(keyUsage, strings.Join(providers, ", "))
		return 2
	}
	return 0
}

// readSecret reads without echo from a terminal, or a single line from a pipe.
func readSecret(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, prompt)
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		return strings.TrimSpace(string(b)), err
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func knownProviders() []string {
	out := []string{}
	for name, p := range backend.ProviderPresets {
		if !p.Local {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
