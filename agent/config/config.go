package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/keytron/allan/agent/internal/secrets"
)

type Config struct {
	// Secrets holds provider API keys. Keys are never written to config.toml:
	// Load fills APIKey fields from here and Save strips them.
	Secrets secrets.Store `toml:"-"`

	Agent     AgentConfig               `toml:"agent"`
	Backend   BackendConfig             `toml:"backend"`
	Providers map[string]ProviderConfig `toml:"providers"`
	TUI       TUIConfig                 `toml:"tui"`
	Memory    MemoryConfig              `toml:"memory"`
	Tools     ToolsConfig               `toml:"tools"`
}

type AgentConfig struct {
	Workspace                string  `toml:"workspace"`
	MaxToolCalls             int     `toml:"max_tool_calls"`
	ToolTimeout              int     `toml:"tool_timeout"`
	ScratchpadEnabled        bool    `toml:"scratchpad_enabled"`
	ScratchpadKeepLastResult int     `toml:"scratchpad_keep_last_results"`
	SkillMinToolCalls        int     `toml:"skill_min_tool_calls"`
	SkillSimilarityThreshold float64 `toml:"skill_similarity_threshold"`
	SkillEnabled             bool    `toml:"skill_enabled"`
	// SkillCooldown is the minimum pause between two saved skills, in seconds.
	SkillCooldown int `toml:"skill_cooldown"`
}

type BackendConfig struct {
	Type    string `toml:"type"`
	Model   string `toml:"model"`
	BaseURL string `toml:"base_url"`
	APIKey  string `toml:"api_key,omitempty"`
}

type ProviderConfig struct {
	Type    string `toml:"type"`
	BaseURL string `toml:"base_url"`
	APIKey  string `toml:"api_key,omitempty"`
}

type TUIConfig struct {
	Theme          string `toml:"theme"`
	ShowTimestamps bool   `toml:"show_timestamps"`
	// Lang is the interface language: ru (default), en or de.
	Lang string `toml:"lang"`
}

type MemoryConfig struct {
	Enabled                  bool   `toml:"enabled"`
	DBPath                   string `toml:"db_path"`
	ChromaDBURL              string `toml:"chromadb_url"`
	ChromaDBCollectionPrefix string `toml:"chromadb_collection_prefix"`
	MemorySummarizeEvery     int    `toml:"memory_summarize_every"`
}

type ToolsConfig struct {
	Shell ShellToolConfig `toml:"shell"`
	SSH   SSHToolConfig   `toml:"ssh"`
}

type ShellToolConfig struct {
	Default                string   `toml:"default"`
	PTYEnabled             bool     `toml:"pty_enabled"`
	PTYFocusKey            string   `toml:"pty_focus_key"`
	PTYAutoFocusOnInput    bool     `toml:"pty_auto_focus_on_input"`
	PTYInteractivePatterns []string `toml:"pty_interactive_patterns"`
}

type SSHToolConfig struct {
	Enabled          bool `toml:"enabled"`
	KnownHostsStrict bool `toml:"known_hosts_strict"`
	DefaultTimeout   int  `toml:"default_timeout"`
}

func Default() *Config {
	return &Config{
		Agent: AgentConfig{
			Workspace:                ".",
			MaxToolCalls:             10,
			ToolTimeout:              30,
			ScratchpadEnabled:        true,
			ScratchpadKeepLastResult: 2,
			SkillMinToolCalls:        3,
			SkillSimilarityThreshold: 0.75,
			SkillEnabled:             true,
			SkillCooldown:            300,
		},
		Backend: BackendConfig{
			Type:  "ollama",
			Model: "llama3.2",
		},
		Providers: map[string]ProviderConfig{},
		TUI: TUIConfig{
			Theme:          "dark",
			ShowTimestamps: false,
			Lang:           "ru",
		},
		Memory: MemoryConfig{
			Enabled:                  true,
			DBPath:                   "~/.allan/memory.db",
			ChromaDBURL:              "http://localhost:8000",
			ChromaDBCollectionPrefix: "allan",
			MemorySummarizeEvery:     20,
		},
		Tools: ToolsConfig{
			Shell: ShellToolConfig{
				Default:             "bash",
				PTYEnabled:          true,
				PTYFocusKey:         "tab",
				PTYAutoFocusOnInput: true,
				PTYInteractivePatterns: []string{
					"password", "Password", "пароль",
					"(yes/no)", "Passphrase", "[Y/n]", "[y/N]",
				},
			},
			SSH: SSHToolConfig{
				Enabled:          true,
				KnownHostsStrict: true,
				DefaultTimeout:   30,
			},
		},
	}
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".allan"), nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// OpenSecrets exposes the secret store for callers outside the config package
// (the pairing flow keeps the worker token there).
func OpenSecrets(dir string) secrets.Store {
	return secrets.Open(dir)
}

func Expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

// LangFromFile reads only the interface language from config.toml, without
// touching the secret store: the CLI needs it before anything else is parsed.
func LangFromFile() string {
	path, err := ConfigPath()
	if err != nil {
		return ""
	}
	var probe struct {
		TUI struct {
			Lang string `toml:"lang"`
		} `toml:"tui"`
	}
	if _, err := toml.DecodeFile(path, &probe); err != nil {
		return ""
	}
	return probe.TUI.Lang
}

func Load() (*Config, bool, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, false, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, false, err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := Default()
		cfg.Secrets = secrets.Open(dir)
		if err := Save(cfg); err != nil {
			return nil, false, err
		}
		return cfg, true, nil
	}
	cfg := Default()
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, false, fmt.Errorf("failed to decode config: %w", err)
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderConfig{}
	}
	cfg.Secrets = secrets.Open(dir)
	if err := cfg.loadSecrets(); err != nil {
		return nil, false, err
	}
	return cfg, false, nil
}

// loadSecrets moves any plaintext api_key left in config.toml (older
// versions stored them there) into the secret store, then fills APIKey
// fields from the store for the running process.
func (c *Config) loadSecrets() error {
	migrated := false
	for name, p := range c.Providers {
		if p.APIKey != "" {
			if err := c.Secrets.Set(name, p.APIKey); err != nil {
				return fmt.Errorf("move %s key to %s: %w", name, c.Secrets.Kind(), err)
			}
			migrated = true
			continue
		}
		key, err := c.Secrets.Get(name)
		if err != nil && err != secrets.ErrNotFound {
			return fmt.Errorf("read %s key from %s: %w", name, c.Secrets.Kind(), err)
		}
		p.APIKey = key
		c.Providers[name] = p
	}
	if c.Backend.APIKey != "" {
		if _, ok := c.Providers[c.Backend.Type]; !ok {
			if err := c.Secrets.Set(c.Backend.Type, c.Backend.APIKey); err != nil {
				return err
			}
		}
		migrated = true
	} else if p, ok := c.Providers[c.Backend.Type]; ok {
		c.Backend.APIKey = p.APIKey
	} else if key, err := c.Secrets.Get(c.Backend.Type); err == nil {
		c.Backend.APIKey = key
	}
	if migrated {
		return Save(c)
	}
	return nil
}

func Save(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	// Encode a copy without keys: secrets live only in cfg.Secrets.
	out := *cfg
	out.Backend.APIKey = ""
	out.Providers = make(map[string]ProviderConfig, len(cfg.Providers))
	for name, p := range cfg.Providers {
		p.APIKey = ""
		out.Providers[name] = p
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(&out); err != nil {
		return err
	}
	return secrets.WriteFileAtomic(path, buf.Bytes(), 0o600)
}
