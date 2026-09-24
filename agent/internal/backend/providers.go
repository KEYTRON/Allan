package backend

import "strings"

// ProviderPreset describes a provider known to Allan out of the box.
type ProviderPreset struct {
	Type    string // backend implementation: anthropic, openai, ollama, llamacpp, lmstudio
	BaseURL string
	Help    string
	Local   bool // runs on this machine, no API key
}

var ProviderPresets = map[string]ProviderPreset{
	"anthropic":    {Type: "anthropic", BaseURL: "https://api.anthropic.com", Help: "Claude API"},
	"openai":       {Type: "openai", BaseURL: "https://api.openai.com/v1", Help: "OpenAI API"},
	"openrouter":   {Type: "openai", BaseURL: "https://openrouter.ai/api/v1", Help: "OpenRouter, сотни моделей, есть бесплатные"},
	"opencode":     {Type: "openai", BaseURL: "https://opencode.ai/zen/v1", Help: "OpenCode Zen (провайдер OpenCode), есть бесплатные *-free"},
	"deepseek":     {Type: "openai", BaseURL: "https://api.deepseek.com/v1", Help: "DeepSeek"},
	"xai":          {Type: "openai", BaseURL: "https://api.x.ai/v1", Help: "Grok / xAI"},
	"groq":         {Type: "openai", BaseURL: "https://api.groq.com/openai/v1", Help: "GroqCloud, не Grok"},
	"zai":          {Type: "openai", BaseURL: "https://api.z.ai/api/paas/v4", Help: "Z.ai (GLM), оплата по токенам"},
	"zai-coding":   {Type: "openai", BaseURL: "https://api.z.ai/api/coding/paas/v4", Help: "Z.ai GLM Coding Plan"},
	"ollama-cloud": {Type: "openai", BaseURL: "https://ollama.com/v1", Help: "Ollama Cloud по API-ключу"},
	"huggingface":  {Type: "openai", BaseURL: "https://router.huggingface.co/v1", Help: "Hugging Face Inference Providers"},
	"featherless":  {Type: "openai", BaseURL: "https://api.featherless.ai/v1", Help: "Featherless.ai, ~20 тыс. моделей с HF"},
	"ollama":       {Type: "ollama", BaseURL: "http://localhost:11434/v1", Help: "локальный Ollama", Local: true},
	"llamacpp":     {Type: "llamacpp", BaseURL: "http://localhost:8080/v1", Help: "локальный llama.cpp server", Local: true},
	"lmstudio":     {Type: "lmstudio", BaseURL: "http://localhost:1234/v1", Help: "локальный LM Studio", Local: true},
}

var providerAliases = map[string]string{
	"grok":         "xai",
	"hf":           "huggingface",
	"ollamacloud":  "ollama-cloud",
	"ollama_cloud": "ollama-cloud",
	"zen":          "opencode",
	"glm":          "zai",
}

func NormalizeProvider(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if alias, ok := providerAliases[name]; ok {
		return alias
	}
	return name
}

// ProviderType maps a provider name to its backend implementation.
// Unknown (custom) providers are treated as OpenAI-compatible endpoints.
func ProviderType(name string) string {
	if p, ok := ProviderPresets[NormalizeProvider(name)]; ok {
		return p.Type
	}
	return "openai"
}

func DefaultBaseURL(name string) string {
	return ProviderPresets[NormalizeProvider(name)].BaseURL
}

func IsKnownProvider(name string) bool {
	_, ok := ProviderPresets[NormalizeProvider(name)]
	return ok
}

func IsLocalProvider(name string) bool {
	return ProviderPresets[NormalizeProvider(name)].Local
}
