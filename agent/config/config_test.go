package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Keys from an old config.toml must move to the secret store and vanish from the file.
func TestLoadMigratesKeysOutOfConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ALLAN_SECRETS", "file")
	dir := filepath.Join(home, ".allan")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := `[backend]
type = "openrouter"
model = "x"
api_key = "sk-old"

[providers.openrouter]
type = "openai"
base_url = "https://openrouter.ai/api/v1"
api_key = "sk-old"
`
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Providers["openrouter"].APIKey; got != "sk-old" {
		t.Fatalf("provider key in memory = %q", got)
	}
	if cfg.Backend.APIKey != "sk-old" {
		t.Fatalf("backend key in memory = %q", cfg.Backend.APIKey)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "sk-old") {
		t.Fatalf("key still in config.toml:\n%s", data)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("config.toml mode = %v, want 0600", st.Mode().Perm())
	}
	sec := filepath.Join(dir, "secrets.json")
	if st, err := os.Stat(sec); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("secrets.json missing or wrong mode: %v", err)
	}

	cfg2, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Providers["openrouter"].APIKey != "sk-old" || cfg2.Backend.APIKey != "sk-old" {
		t.Fatal("key not restored from secret store on second load")
	}
}
