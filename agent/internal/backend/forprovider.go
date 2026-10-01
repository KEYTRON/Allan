package backend

import (
	"fmt"

	"github.com/keytron/allan/agent/config"
)

// ForProvider builds a backend for an arbitrary provider name using the keys
// and endpoints from the config, the same way the TUI switches models.
//
// The returned backend shares the config's provider map, so custom endpoints
// added with `/api endpoint <name> <base_url>` work here too.
func ForProvider(provider string, cfg *config.Config, model string) (Backend, error) {
	provider = NormalizeProvider(provider)
	if p, ok := cfg.Providers[provider]; ok {
		if p.Type == "openai" && provider != "openai" {
			// A custom OpenAI-compatible endpoint keeps its own name for the
			// User-Agent style headers, but talks the OpenAI protocol.
			return NewOpenAILike(provider, p.BaseURL, p.APIKey, model), nil
		}
		tmp := *cfg
		tmp.Backend.Type = provider
		tmp.Backend.Model = model
		tmp.Backend.BaseURL = p.BaseURL
		tmp.Backend.APIKey = p.APIKey
		if p.Type != "" {
			tmp.Backend.Type = p.Type
		}
		return New(&tmp)
	}
	switch {
	case !IsKnownProvider(provider):
		return nil, fmt.Errorf("неизвестный провайдер %s: добавьте эндпоинт /api endpoint %s <base_url>", provider, provider)
	case !IsLocalProvider(provider) && provider != "anthropic" && provider != "openai":
		return nil, fmt.Errorf("%s требует API-ключ: /api add %s <api_key>", provider, provider)
	default:
		tmp := *cfg
		tmp.Backend.Type = provider
		tmp.Backend.Model = model
		tmp.Backend.BaseURL = DefaultBaseURL(provider)
		if IsLocalProvider(provider) {
			tmp.Backend.APIKey = ""
		}
		return New(&tmp)
	}
}
