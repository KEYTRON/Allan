package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Agent   AgentConfig   `toml:"agent"`
	Backend BackendConfig `toml:"backend"`
	TUI     TUIConfig     `toml:"tui"`
	Memory  MemoryConfig  `toml:"memory"`
	Tools   ToolsConfig   `toml:"tools"`
}

type AgentConfig struct {
	Workspace                string `toml:"workspace"`
	MaxToolCalls             int    `toml:"max_tool_calls"`
	ToolTimeout              int    `toml:"tool_timeout"`
	ScratchpadEnabled        bool   `toml:"scratchpad_enabled"`
	ScratchpadKeepLastResult int    `toml:"scratchpad_keep_last_results"`
	SkillMinToolCalls        int    `toml:"skill_min_tool_calls"`
	SkillSimilarityThreshold float64 `toml:"skill_similarity_threshold"`
}

type BackendConfig struct {
	Type    string `toml:"type"`
	Model   string `toml:"model"`
	BaseURL string `toml:"base_url"`
	APIKey  string `toml:"api_key"`
}

type TUIConfig struct {
	Theme          string `toml:"theme"`
	ShowTimestamps bool   `toml:"show_timestamps"`
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
		},
		Backend: BackendConfig{
			Type:  "ollama",
			Model: "llama3.2",
		},
		TUI: TUIConfig{
			Theme:          "dark",
			ShowTimestamps: false,
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

func Expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
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
		if err := Save(cfg); err != nil {
			return nil, false, err
		}
		return cfg, true, nil
	}
	cfg := Default()
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, false, fmt.Errorf("failed to decode config: %w", err)
	}
	return cfg, false, nil
}

func Save(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := toml.NewEncoder(f)
	return enc.Encode(cfg)
}
