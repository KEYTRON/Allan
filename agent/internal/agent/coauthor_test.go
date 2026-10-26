package agent

import (
	"strings"
	"testing"

	"github.com/keytron/allan/agent/config"
)

func TestCoAuthorOnlyForAllanLM(t *testing.T) {
	for model, want := range map[string]bool{
		"allan-lm":                       true,
		"keytron/Allan-LM-1b":            true,
		"allan/2/ollama/allan-lm:latest": true,
		"keytron/qwen3-8b":               false,
		"":                               false,
	} {
		cfg := &config.Config{}
		cfg.Backend.Model = model
		a := &Agent{Cfg: cfg}
		if got := strings.Contains(a.SystemPrompt(), AllanLMCoAuthor); got != want {
			t.Errorf("%q: co-author in prompt = %v, want %v", model, got, want)
		}
	}
}
