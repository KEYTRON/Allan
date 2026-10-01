// Package bootstrap builds a ready-to-run agent from the local config.
//
// Both frontends use it: the TUI (cmd/allan) and the headless worker
// (`allan serve`, internal/serve). Keeping the wiring in one place means the
// server exposes exactly the same tools, memory and skills as the terminal.
package bootstrap

import (
	"context"
	"errors"
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
	"github.com/keytron/allan/agent/internal/vector"
)

// Options tweaks the wiring for a particular frontend.
type Options struct {
	// Workspace is the agent's working directory. Empty means config.Agent.Workspace
	// or the current directory.
	Workspace string
	// NoMemory disables SQLite memory and, with it, the Skill Engine.
	NoMemory bool
	// Backend and Model override the configured provider for this run only.
	Backend string
	Model   string
	// PickLocalModel replaces a missing local model with one that is installed.
	PickLocalModel bool
	// NoSSH leaves the SSH tool out of the registry.
	NoSSH bool
	// NoShell leaves the shell tool out of the registry.
	NoShell bool
}

// Runtime is everything a frontend needs to talk to the agent.
type Runtime struct {
	Cfg       *config.Config
	Agent     *agent.Agent
	Memory    *memory.Memory
	Vector    vector.Store
	Skills    *skills.Engine
	Registry  *tools.Registry
	Workspace string
	// Created is true when Build had to write a default config file.
	Created bool
	// Notice is a human-readable remark to show on startup (model substitution).
	Notice string
}

// Build loads the config, wires the backend, tools, memory and skills, and
// returns an agent that is ready for its first Run.
func Build(ctx context.Context, opts Options) (*Runtime, error) {
	cfg, created, err := config.Load()
	if err != nil {
		return nil, err
	}
	if opts.Backend != "" {
		cfg.Backend.Type = backend.NormalizeProvider(opts.Backend)
	}
	if opts.Model != "" {
		cfg.Backend.Model = opts.Model
	}

	workspace, err := resolveWorkspace(cfg, opts.Workspace)
	if err != nil {
		return nil, err
	}

	// A configured provider is an explicit choice, so only auto-detect when
	// nothing was configured at all.
	_, configured := cfg.Providers[cfg.Backend.Type]
	if opts.Backend == "" && !configured && cfg.Backend.Type != "anthropic" && cfg.Backend.Type != "openai" {
		if detected := backend.Detect(ctx); detected != "" {
			cfg.Backend.Type = detected
		}
	}

	be, err := backend.New(cfg)
	if err != nil {
		return nil, err
	}
	notice := ""
	if opts.Model == "" && opts.PickLocalModel && backend.IsLocalProvider(cfg.Backend.Type) {
		if picked := pickInstalledModel(ctx, be, cfg.Backend.Model); picked != "" && picked != cfg.Backend.Model {
			missing := cfg.Backend.Model
			cfg.Backend.Model = picked
			if missing == config.Default().Backend.Model {
				// The stock default was never chosen by the user: replace it for good.
				_ = config.Save(cfg)
			} else {
				notice = fmt.Sprintf("Модели %s нет в %s, на этот запуск выбрана %s. Выбрать другую: /model", missing, cfg.Backend.Type, picked)
				notice += fmt.Sprintf(" (если она нужна: ollama pull %s)", missing)
			}
			if be, err = backend.New(cfg); err != nil {
				return nil, err
			}
		}
	}

	be = withEnvFallback(cfg, be)

	registry := tools.NewRegistry()
	if !opts.NoShell {
		registry.Register(tools.NewBashTool(cfg.Tools.Shell.Default, workspace))
	}
	registry.Register(tools.NewReadFileTool(workspace))
	registry.Register(tools.NewWriteFileTool(workspace))
	registry.Register(tools.NewSearchTool())
	if cfg.Tools.SSH.Enabled && !opts.NoSSH {
		registry.Register(tools.NewSSHTool(cfg.Tools.SSH.KnownHostsStrict, cfg.Tools.SSH.DefaultTimeout))
	}

	var mem *memory.Memory
	if cfg.Memory.Enabled && !opts.NoMemory {
		mem, err = memory.Open(config.Expand(cfg.Memory.DBPath))
		if err != nil {
			// A broken memory database must not stop the agent: tools and the
			// conversation still work, only the long-term memory is lost.
			mem = nil
		}
	}

	vec := vector.Store(&vector.NoopStore{})
	if cfg.Memory.ChromaDBURL != "" {
		c := vector.NewChroma(cfg.Memory.ChromaDBURL, cfg.Memory.ChromaDBCollectionPrefix)
		if c.Healthy(ctx) {
			vec = c
		}
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
	}

	return &Runtime{
		Cfg:       cfg,
		Agent:     ag,
		Memory:    mem,
		Vector:    vec,
		Skills:    skillEngine,
		Registry:  registry,
		Workspace: workspace,
		Created:   created,
		Notice:    notice,
	}, nil
}

// Resume loads a past conversation back into the agent and returns it, so the
// frontend can show the same transcript instead of an empty screen.
//
// sessionID may be empty: then the most recent finished session is taken. The
// session keeps its id and is reopened, so the continuation is appended to the
// same history in SQLite.
func (r *Runtime) Resume(ctx context.Context, sessionID string, n int) ([]memory.ConvMessage, error) {
	if r.Memory == nil {
		return nil, errors.New("память отключена, --resume не работает (уберите --no-memory)")
	}
	id := sessionID
	if id == "" {
		last, err := r.Memory.LastSessionID(ctx)
		if err != nil {
			return nil, err
		}
		id = last
	}
	msgs, err := r.Memory.SessionMessages(ctx, id, n)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("сессия %s пуста", id)
	}
	r.Agent.History = nil
	for _, mg := range msgs {
		switch mg.Role {
		case "user":
			r.Agent.History = append(r.Agent.History, backend.Message{Role: backend.RoleUser, Content: mg.Content})
		case "assistant":
			r.Agent.History = append(r.Agent.History, backend.Message{Role: backend.RoleAssistant, Content: mg.Content})
		}
	}
	if err := r.Memory.ReopenSession(ctx, id, time.Now(), r.Cfg.Backend.Type, r.Cfg.Backend.Model); err != nil {
		return msgs, fmt.Errorf("сессия %s загружена, но не переоткрыта: %w", id, err)
	}
	r.Agent.SessionID = id
	return msgs, nil
}

// Close finishes the memory session and flushes stats.
func (r *Runtime) Close(ctx context.Context) {
	if r.Memory == nil {
		return
	}
	_ = r.Memory.EndSession(ctx, &memory.Session{
		ID:        r.Agent.SessionID,
		ToolCalls: r.Agent.Stats.ToolCalls,
		TokensIn:  r.Agent.Stats.TokensIn,
		TokensOut: r.Agent.Stats.TokensOut,
	})
	r.Memory.Close()
}

func resolveWorkspace(cfg *config.Config, override string) (string, error) {
	workspace := override
	if workspace == "" {
		if cfg.Agent.Workspace != "" && cfg.Agent.Workspace != "." {
			workspace = config.Expand(cfg.Agent.Workspace)
		} else {
			workspace, _ = filepath.Abs(".")
		}
	}
	return filepath.Abs(workspace)
}

// withEnvFallback lets ANTHROPIC_API_KEY / OPENAI_API_KEY work without
// touching the config, exactly like the TUI does.
func withEnvFallback(cfg *config.Config, be backend.Backend) backend.Backend {
	switch cfg.Backend.Type {
	case "anthropic":
		if cfg.Backend.APIKey == "" {
			if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" {
				cfg.Backend.APIKey = k
				return backend.NewAnthropic(k, cfg.Backend.Model, cfg.Backend.BaseURL)
			}
		}
	case "openai":
		if cfg.Backend.APIKey == "" {
			if k := os.Getenv("OPENAI_API_KEY"); k != "" {
				cfg.Backend.APIKey = k
				return backend.NewOpenAILike("openai", "https://api.openai.com/v1", k, cfg.Backend.Model)
			}
		}
	}
	return be
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
