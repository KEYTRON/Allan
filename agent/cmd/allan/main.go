package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
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

var version = "0.1.0"

func main() {
	var (
		flBackend   = flag.String("backend", "", "Override backend from config (anthropic|openai|ollama|llamacpp|lmstudio)")
		flModel     = flag.String("model", "", "Override model from config")
		flWorkspace = flag.String("workspace", "", "Set working directory (default: current dir)")
		flResume    = flag.Bool("resume", false, "Resume last session")
		flVersion   = flag.Bool("version", false, "Show version")
		flNoMemory  = flag.Bool("no-memory", false, "Disable memory for this session")
	)
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
	if *flBackend == "" && cfg.Backend.Type != "anthropic" && cfg.Backend.Type != "openai" {
		if detected := backend.Detect(ctx); detected != "" {
			cfg.Backend.Type = detected
		}
	}

	be, err := backend.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backend: %v\n", err)
		os.Exit(1)
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
	fmt.Println("Allan Session Summary")
	fmt.Println("─────────────────────")
	fmt.Printf("Session ID:    %s\n", ag.SessionID)
	fmt.Printf("Duration:      %s\n", dur)
	fmt.Printf("Tool Calls:    %d (✓ %d  ✗ %d)\n", ag.Stats.ToolCalls, ag.Stats.SuccessCalls, ag.Stats.FailedCalls)
	fmt.Printf("Tokens In:     %d\n", ag.Stats.TokensIn)
	fmt.Printf("Tokens Out:    %d\n", ag.Stats.TokensOut)
	fmt.Printf("Backend:       %s\n", ag.Backend.Name())
	fmt.Printf("Model:         %s\n", ag.Cfg.Backend.Model)
}
