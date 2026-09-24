package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/agent"
	"github.com/keytron/allan/agent/internal/backend"
	"github.com/keytron/allan/agent/internal/memory"
	"github.com/keytron/allan/agent/internal/skills"
	"github.com/keytron/allan/agent/internal/tools"
	"github.com/keytron/allan/agent/internal/tui"
	"github.com/keytron/allan/agent/internal/vector"
)

var version = "0.3.3"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "key":
			os.Exit(runKey(os.Args[2:]))
		case "help":
			printUsage()
			return
		}
	}
	var (
		flBackend   = flag.String("backend", "", "провайдер")
		flModel     = flag.String("model", "", "модель")
		flWorkspace = flag.String("workspace", "", "рабочая папка")
		flResume    = flag.Bool("resume", false, "продолжить последнюю сессию")
		flVersion   = flag.Bool("version", false, "версия")
		flNoMemory  = flag.Bool("no-memory", false, "без памяти")
	)
	flag.Usage = printUsage
	flag.Parse()

	if *flVersion {
		fmt.Printf("allan %s\n", version)
		return
	}

	cfg, created, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	if created {
		path, _ := config.ConfigPath()
		fmt.Printf("Создан дефолтный конфиг: %s\n", path)
	}

	if *flBackend != "" {
		cfg.Backend.Type = *flBackend
	}
	if *flModel != "" {
		cfg.Backend.Model = *flModel
	}

	workspace, _ := os.Getwd()
	if *flWorkspace != "" {
		workspace = *flWorkspace
	} else if cfg.Agent.Workspace != "" && cfg.Agent.Workspace != "." {
		workspace = config.Expand(cfg.Agent.Workspace)
	}
	abs, err := filepath.Abs(workspace)
	if err == nil {
		workspace = abs
	}

	// Auto-detect backend if no explicit choice and api keys missing
	ctx := context.Background()
	// A configured API provider (openrouter, ollama-cloud, ...) is an explicit choice too.
	_, configured := cfg.Providers[cfg.Backend.Type]
	if *flBackend == "" && !configured && cfg.Backend.Type != "anthropic" && cfg.Backend.Type != "openai" {
		if detected := backend.Detect(ctx); detected != "" {
			cfg.Backend.Type = detected
		}
	}

	be, err := backend.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backend: %v\n", err)
		os.Exit(1)
	}
	if *flModel == "" && backend.IsLocalProvider(cfg.Backend.Type) {
		if picked := pickInstalledModel(ctx, be, cfg.Backend.Model); picked != "" && picked != cfg.Backend.Model {
			cfg.Backend.Model = picked
			_ = config.Save(cfg)
			if be, err = backend.New(cfg); err != nil {
				fmt.Fprintf(os.Stderr, "backend: %v\n", err)
				os.Exit(1)
			}
		}
	}

	if cfg.Backend.Type == "anthropic" && cfg.Backend.APIKey == "" {
		if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" {
			cfg.Backend.APIKey = k
			be = backend.NewAnthropic(k, cfg.Backend.Model, cfg.Backend.BaseURL)
		}
	}
	if cfg.Backend.Type == "openai" && cfg.Backend.APIKey == "" {
		if k := os.Getenv("OPENAI_API_KEY"); k != "" {
			cfg.Backend.APIKey = k
			be = backend.NewOpenAILike("openai", "https://api.openai.com/v1", k, cfg.Backend.Model)
		}
	}

	registry := tools.NewRegistry()
	registry.Register(tools.NewBashTool(cfg.Tools.Shell.Default, workspace))
	registry.Register(tools.NewReadFileTool(workspace))
	registry.Register(tools.NewWriteFileTool(workspace))
	registry.Register(tools.NewSearchTool())
	if cfg.Tools.SSH.Enabled {
		registry.Register(tools.NewSSHTool(cfg.Tools.SSH.KnownHostsStrict, cfg.Tools.SSH.DefaultTimeout))
	}

	var mem *memory.Memory
	if cfg.Memory.Enabled && !*flNoMemory {
		dbPath := config.Expand(cfg.Memory.DBPath)
		mem, err = memory.Open(dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "memory open warning: %v\n", err)
			mem = nil
		}
	}

	var vec vector.Store
	if cfg.Memory.ChromaDBURL != "" {
		c := vector.NewChroma(cfg.Memory.ChromaDBURL, cfg.Memory.ChromaDBCollectionPrefix)
		if c.Healthy(ctx) {
			vec = c
		} else {
			vec = &vector.NoopStore{}
		}
	} else {
		vec = &vector.NoopStore{}
	}

	var skillEngine *skills.Engine
	if mem != nil {
		skillEngine = skills.NewEngine(mem.DB(), vec)
	}

	ag := agent.New(cfg, be, registry, mem, skillEngine, vec, workspace)
	ag.SessionID = uuid.NewString()

	if mem != nil {
		_ = mem.StartSession(ctx, &memory.Session{
			ID:        ag.SessionID,
			StartedAt: time.Now(),
			Backend:   ag.Backend.Name(),
			Model:     cfg.Backend.Model,
		})
		if *flResume {
			msgs, err := mem.LastSessionMessages(ctx, 20)
			if err == nil {
				for _, mg := range msgs {
					switch mg.Role {
					case "user":
						ag.History = append(ag.History, backend.Message{Role: backend.RoleUser, Content: mg.Content})
					case "assistant":
						ag.History = append(ag.History, backend.Message{Role: backend.RoleAssistant, Content: mg.Content})
					}
				}
			}
		}
	}

	startedAt := time.Now()
	model := tui.New(cfg, ag, workspace, version)
	if err := tui.Run(model); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
	}

	// On exit:
	if mem != nil {
		_ = mem.EndSession(ctx, &memory.Session{
			ID:        ag.SessionID,
			ToolCalls: ag.Stats.ToolCalls,
			TokensIn:  ag.Stats.TokensIn,
			TokensOut: ag.Stats.TokensOut,
		})
		mem.Close()
	}
	printSummary(ag, startedAt)
}

func printSummary(ag *agent.Agent, started time.Time) {
	dur := time.Since(started).Round(time.Second)
	fmt.Println()
	fmt.Printf("◆ allan · сессия завершена за %s\n", dur)
	fmt.Printf("  модель       %s/%s\n", ag.Cfg.Backend.Type, ag.Cfg.Backend.Model)
	fmt.Printf("  инструменты  %d (✓ %d  ✗ %d)\n", ag.Stats.ToolCalls, ag.Stats.SuccessCalls, ag.Stats.FailedCalls)
	fmt.Printf("  токены       %d вход · %d выход\n", ag.Stats.TokensIn, ag.Stats.TokensOut)
	fmt.Printf("  сессия       %s (продолжить: allan --resume)\n", ag.SessionID)
}

// pickInstalledModel keeps the configured model if the local server has it,
// otherwise picks an installed chat model so the first request does not fail
// on a model that was never pulled (the default llama3.2, for example).
// Ollama's ":cloud" models come first: local 3-7B models rarely handle tools well.
func pickInstalledModel(ctx context.Context, be backend.Backend, want string) string {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	models, err := be.Models(cctx)
	if err != nil || len(models) == 0 {
		return ""
	}
	chat := []string{}
	for _, m := range models {
		if m == want || m == want+":latest" {
			return m
		}
		low := strings.ToLower(m)
		if strings.Contains(low, "embed") || strings.Contains(low, "ocr") || strings.Contains(low, "rerank") {
			continue
		}
		chat = append(chat, m)
	}
	for _, m := range chat {
		if strings.HasSuffix(m, ":cloud") || strings.HasSuffix(m, "-cloud") {
			return m
		}
	}
	if len(chat) > 0 {
		return chat[0]
	}
	return ""
}

func printUsage() {
	fmt.Printf(`Allan %s — автономный агент в терминале

Использование:
  allan [флаги]                 запустить TUI в текущей папке
  allan key set <provider>      сохранить API-ключ (скрытый ввод, системный keyring)
  allan key list | rm <provider>
  allan help                    эта справка

Флаги:
  --model <имя>          модель на этот запуск (например glm-5.2:cloud)
  --backend <провайдер>  провайдер на этот запуск: ollama, openrouter, opencode, ...
  --workspace <папка>    рабочая папка агента (по умолчанию текущая)
  --resume               продолжить последнюю сессию
  --no-memory            не читать и не писать память
  --version              показать версию

Провайдеры:
  облачные   %s
  локальные  ollama, llamacpp, lmstudio (без ключа, находятся автоматически)

Внутри TUI: /help — команды, /model — выбрать модель, /api — ключи и эндпоинты.

Файлы:
  ~/.allan/config.toml   настройки (без ключей)
  ~/.allan/memory.db     память и сессии
  ключи                  системный keyring, иначе ~/.allan/secrets.json (0600)

Примеры:
  allan key set openrouter
  allan --backend opencode --model deepseek-v4-flash-free
  allan --workspace ~/git/project --resume
`, version, strings.Join(knownProviders(), ", "))
}
