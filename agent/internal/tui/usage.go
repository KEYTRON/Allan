package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/keytron/allan/agent/internal/backend"
	"github.com/keytron/allan/agent/internal/i18n"
)

// modelUsage is what one model spent in this session, counted on this machine.
type modelUsage struct {
	Turns     int
	TokensIn  int
	TokensOut int
}

// beginTurnUsage remembers the counters before a turn so the turn's tokens
// can be put on the model that answered it.
func (m *Model) beginTurnUsage() {
	m.turnIn, m.turnOut = m.Ag.Stats.TokensIn, m.Ag.Stats.TokensOut
	m.turnProvider = m.Cfg.Backend.Type
}

func (m *Model) endTurnUsage() {
	if m.sessionUsage == nil {
		m.sessionUsage = map[string]*modelUsage{}
	}
	key := m.turnProvider + "/" + m.turnModel
	u := m.sessionUsage[key]
	if u == nil {
		u = &modelUsage{}
		m.sessionUsage[key] = u
	}
	u.Turns++
	u.TokensIn += m.Ag.Stats.TokensIn - m.turnIn
	u.TokensOut += m.Ag.Stats.TokensOut - m.turnOut
	m.turnIn, m.turnOut = m.Ag.Stats.TokensIn, m.Ag.Stats.TokensOut
}

// siteUsage is the reply of KEYTRON Prime's GET /usage.
type siteUsage struct {
	Periods map[string]struct {
		Totals  usageRow   `json:"totals"`
		ByModel []usageRow `json:"by_model"`
	} `json:"periods"`
	Balance struct {
		Site struct {
			AvailableUSD float64 `json:"available_usd"`
			Plan         *string `json:"plan"`
			Unlimited    bool    `json:"unlimited"`
		} `json:"site"`
		Providers []struct {
			Name      string  `json:"name"`
			Amount    float64 `json:"amount"`
			Currency  string  `json:"currency"`
			Note      string  `json:"note"`
			Available *bool   `json:"available"`
		} `json:"providers"`
	} `json:"balance"`
}

type usageRow struct {
	Model            string   `json:"model"`
	Requests         int      `json:"requests"`
	PromptTokens     int      `json:"prompt_tokens"`
	CompletionTokens int      `json:"completion_tokens"`
	CostUSD          *float64 `json:"cost_usd"`
	ChargedUSD       float64  `json:"charged_usd"`
}

type directBalance struct {
	provider string
	balance  *backend.Balance
	err      error
}

type usageMsg struct {
	site     *siteUsage
	siteErr  error
	balances []directBalance
}

// fetchUsage asks KEYTRON Prime for the month, the year and this session, and
// every directly configured provider that reports a balance, all at once.
func (m *Model) fetchUsage() tea.Cmd {
	keytron, hasKeytron := m.Cfg.Providers["keytron"]
	since := m.startedAt
	type direct struct{ name, key string }
	var directs []direct
	for name, p := range m.Cfg.Providers {
		if name != "keytron" && p.APIKey != "" && backend.BalanceSupported(name) {
			directs = append(directs, direct{name, p.APIKey})
		}
	}
	sort.Slice(directs, func(i, j int) bool { return directs[i].name < directs[j].name })
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		out := usageMsg{balances: make([]directBalance, len(directs))}
		var wg sync.WaitGroup
		for i, d := range directs {
			wg.Add(1)
			go func(i int, d direct) {
				defer wg.Done()
				b, err := backend.ProviderBalance(ctx, d.name, d.key)
				out.balances[i] = directBalance{d.name, b, err}
			}(i, d)
		}
		if hasKeytron && keytron.APIKey != "" {
			out.site, out.siteErr = getSiteUsage(ctx, keytron.BaseURL, keytron.APIKey, since)
		}
		wg.Wait()
		return out
	}
}

func getSiteUsage(ctx context.Context, base, key string, since time.Time) (*siteUsage, error) {
	u := strings.TrimRight(base, "/") + "/usage?since=" + url.QueryEscape(since.Format(time.RFC3339))
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
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
	var out siteUsage
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// renderUsage is the /usage report: this session, then today, the month and
// the year from KEYTRON Prime, then balances.
func (m *Model) renderUsage(msg usageMsg) string {
	var sb strings.Builder
	w := func(format string, a ...any) { sb.WriteString(fmt.Sprintf(format, a...)) }

	w(i18n.S("Сессия · с %s (%s)\n"), m.startedAt.Format("15:04"), time.Since(m.startedAt).Round(time.Minute))
	keys := make([]string, 0, len(m.sessionUsage))
	turns := 0
	for k, u := range m.sessionUsage {
		keys = append(keys, k)
		turns += u.Turns
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := m.sessionUsage[keys[i]], m.sessionUsage[keys[j]]
		return a.TokensIn+a.TokensOut > b.TokensIn+b.TokensOut
	})
	w(i18n.S("  ходов %d · токены %s → %s · инструменты %d\n"), turns, humanTokens(m.Ag.Stats.TokensIn), humanTokens(m.Ag.Stats.TokensOut), m.Ag.Stats.ToolCalls)
	for _, k := range keys {
		u := m.sessionUsage[k]
		w("  %-50s %3d · %s → %s\n", truncate(k, 50), u.Turns, humanTokens(u.TokensIn), humanTokens(u.TokensOut))
	}

	switch {
	case msg.site != nil:
		if s, ok := msg.site.Periods["session"]; ok && s.Totals.Requests > 0 {
			w(i18n.S("  через KEYTRON: %s\n"), usageLine(s.Totals))
		}
		sb.WriteString("\n")
		for _, p := range [][2]string{{"today", "Сегодня"}, {"month", "Этот месяц"}, {"year", "Этот год"}} {
			r := msg.site.Periods[p[0]]
			w("%-12s %s\n", i18n.S(p[1]), usageLine(r.Totals))
			if p[0] == "month" {
				for _, row := range r.ByModel[:minInt(5, len(r.ByModel))] {
					w("  %-50s %s\n", truncate(row.Model, 50), usageLine(row))
				}
			}
		}
	case msg.siteErr != nil:
		w(i18n.S("\nKEYTRON Prime не ответил: %v\n"), msg.siteErr)
	default:
		w(i18n.S("\nДень, месяц и год считает KEYTRON Prime, а эта машина к нему не подключена: %s\n"), m.keytronStatus())
	}

	sb.WriteString(i18n.S("\nБалансы\n"))
	if msg.site != nil {
		b := msg.site.Balance
		site := fmt.Sprintf("$%.2f", b.Site.AvailableUSD)
		if b.Site.Unlimited {
			site = i18n.S("без ограничений")
		}
		if b.Site.Plan != nil {
			site += " · " + *b.Site.Plan
		}
		w("  %-30s %s\n", "KEYTRON Prime", site)
		for _, p := range b.Providers {
			line := money(p.Amount, p.Currency)
			if p.Note != "" {
				line += " · " + p.Note
			}
			if p.Available != nil && !*p.Available {
				line += " · " + i18n.S("нет средств")
			}
			w("  %-30s %s\n", p.Name+i18n.S(" (ключ на сайте)"), line)
		}
	}
	for _, d := range msg.balances {
		var line string
		switch {
		case d.err != nil:
			line = i18n.S("не ответил: ") + d.err.Error()
		case d.balance == nil:
			continue
		default:
			line = money(d.balance.Amount, d.balance.Currency)
			if d.balance.Note != "" {
				line += " · " + i18n.S(d.balance.Note)
			}
		}
		w("  %-30s %s\n", d.provider+i18n.S(" (ключ Allan)"), line)
	}
	sb.WriteString(i18n.S("  остаток сообщают DeepSeek, OpenRouter и ElevenLabs; у остальных — только в их кабинетах"))
	return sb.String()
}

func usageLine(r usageRow) string {
	cost := "—"
	if r.CostUSD != nil {
		cost = fmt.Sprintf("$%.4f", *r.CostUSD)
	}
	s := fmt.Sprintf(i18n.S("%s · %d запр. · %s ток."), cost, r.Requests, humanTokens(r.PromptTokens+r.CompletionTokens))
	if r.ChargedUSD > 0 {
		s += fmt.Sprintf(i18n.S(" · списано $%.4f"), r.ChargedUSD)
	}
	return s
}

func money(v float64, currency string) string {
	switch currency {
	case "USD", "":
		return fmt.Sprintf("$%.2f", v)
	default:
		return fmt.Sprintf("%.2f %s", v, currency)
	}
}
