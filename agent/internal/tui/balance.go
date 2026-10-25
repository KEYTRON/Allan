package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/keytron/allan/agent/internal/backend"
	"github.com/keytron/allan/agent/internal/i18n"
)

// providerTitles names BYOK providers the way the site does.
var providerTitles = map[string]string{
	"openai": "OpenAI", "anthropic": "Anthropic", "openrouter": "OpenRouter", "gemini": "Google Gemini",
	"xai": "xAI Grok", "deepseek": "DeepSeek", "mistral": "Mistral", "groq": "Groq", "ollama": "Ollama Cloud",
	"huggingface": "Hugging Face", "perplexity": "Perplexity", "elevenlabs": "ElevenLabs",
}

func providerTitle(slug string) string {
	if t, ok := providerTitles[slug]; ok {
		return t
	}
	return slug
}

// balanceEvery limits how often the status line asks for the balance.
const balanceEvery = time.Minute

type statusBalanceMsg struct {
	text string
	at   time.Time
}

// refreshBalance asks for the money behind the current model: the provider's
// own balance for a direct key, the BYOK balance for a model routed through
// KEYTRON with the user's key, otherwise the KEYTRON balance. force skips the
// rate limit (after switching the model).
func (m *Model) refreshBalance(force bool) tea.Cmd {
	if !force && time.Since(m.balanceAt) < balanceEvery {
		return nil
	}
	m.balanceAt = time.Now()
	provider, model := m.Cfg.Backend.Type, m.Cfg.Backend.Model
	direct, hasDirect := m.Cfg.Providers[provider]
	keytron, hasKeytron := m.Cfg.Providers["keytron"]
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		at := time.Now()
		if provider != "keytron" && hasDirect && direct.APIKey != "" && backend.BalanceSupported(provider) {
			if b, err := backend.ProviderBalance(ctx, provider, direct.APIKey); err == nil && b != nil {
				return statusBalanceMsg{providerTitle(provider) + " " + money(b.Amount, b.Currency), at}
			}
		}
		if !hasKeytron || keytron.APIKey == "" {
			return statusBalanceMsg{"", at}
		}
		bal, err := getSiteBalance(ctx, keytron.BaseURL, keytron.APIKey)
		if err != nil {
			return statusBalanceMsg{"", at}
		}
		if provider == "keytron" {
			owner := strings.SplitN(model, "/", 2)[0]
			for _, p := range bal.Providers {
				if p.Slug == owner {
					return statusBalanceMsg{"BYOK " + providerTitle(owner) + " " + money(p.Amount, p.Currency), at}
				}
			}
		}
		if bal.Site.Unlimited {
			return statusBalanceMsg{"KEYTRON " + i18n.S("без ограничений"), at}
		}
		return statusBalanceMsg{fmt.Sprintf("KEYTRON $%.2f", bal.Site.AvailableUSD), at}
	}
}

type siteBalance struct {
	Site struct {
		AvailableUSD float64 `json:"available_usd"`
		Unlimited    bool    `json:"unlimited"`
	} `json:"site"`
	Providers []struct {
		Slug     string  `json:"slug"`
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
	} `json:"providers"`
}

func getSiteBalance(ctx context.Context, base, key string) (*siteBalance, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(base, "/")+"/usage?only=balance", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var out struct {
		Balance siteBalance `json:"balance"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out.Balance, nil
}
