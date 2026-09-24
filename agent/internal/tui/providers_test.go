package tui

import "testing"

func TestCloudProviders(t *testing.T) {
	cases := []struct {
		in, name, base string
	}{
		{"ollama-cloud", "ollama-cloud", "https://ollama.com/v1"},
		{"OllamaCloud", "ollama-cloud", "https://ollama.com/v1"},
		{"hf", "huggingface", "https://router.huggingface.co/v1"},
		{"featherless", "featherless", "https://api.featherless.ai/v1"},
		{"openrouter", "openrouter", "https://openrouter.ai/api/v1"},
		{"ollama", "ollama", "http://localhost:11434/v1"},
	}
	for _, c := range cases {
		name := normalizeProvider(c.in)
		if name != c.name {
			t.Errorf("normalizeProvider(%q) = %q, want %q", c.in, name, c.name)
		}
		if got := defaultBaseURL(name); got != c.base {
			t.Errorf("defaultBaseURL(%q) = %q, want %q", name, got, c.base)
		}
	}
	for _, name := range []string{"ollama-cloud", "huggingface", "featherless"} {
		if providerType(name) != "openai" {
			t.Errorf("providerType(%q) = %q, want openai", name, providerType(name))
		}
		if isLocalProvider(name) {
			t.Errorf("%s must not be local", name)
		}
	}
	if !isLocalProvider("ollama") {
		t.Error("ollama must stay local")
	}
}
