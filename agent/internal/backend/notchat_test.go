package backend

import "testing"

func TestNotChatName(t *testing.T) {
	for id, want := range map[string]bool{
		"meta/llama-3.3-70b-instruct":        false,
		"nvidia/nemotron-3.5-embed-1b":       true,
		"nvidia/llama-3.2-nv-rerankqa-1b-v2": true,
		"nvidia/nemotron-3.5-content-safety": true,
		"models/gemini-3-flash":              false,
		"mistral-moderation-latest":          true,
	} {
		if got := notChatName(id); got != want {
			t.Errorf("notChatName(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestNewProviderPresets(t *testing.T) {
	for alias, name := range map[string]string{"nim": "nvidia", "google": "gemini", "pplx": "perplexity", "lfm": "liquid", "mistral": "mistral"} {
		if got := NormalizeProvider(alias); got != name {
			t.Errorf("NormalizeProvider(%q) = %q", alias, got)
		}
		if ProviderType(name) != "openai" || DefaultBaseURL(name) == "" {
			t.Errorf("%s: no OpenAI-compatible preset", name)
		}
	}
}
