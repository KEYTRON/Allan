package tui

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/keytron/allan/agent/internal/backend"
	"github.com/keytron/allan/agent/internal/i18n"
	"github.com/keytron/allan/agent/internal/link"
)

// Sections of the model list, in the order they are shown: what runs here,
// then the user's other machines, then KEYTRON Prime itself, then the user's
// own keys routed through it, and only then providers called directly.
const (
	secLocal = iota
	secMine
	secKeytron
	secBYOK
	secDirect
	secCount
)

var sectionTitles = [secCount]string{"Этот ПК", "Мои ПК", "KEYTRON", "Ключи через KEYTRON", "Напрямую"}

// keytronLogin is the Value of the placeholder row shown when KEYTRON is not
// reachable from this machine.
type keytronLogin struct{}

// catalogTimeout is how long one provider may take to list its models. Local
// servers answer at once or are not running; the site polls the user's other
// computers, which takes longer.
func catalogTimeout(name string) time.Duration {
	switch {
	case isLocalProvider(name):
		return 2 * time.Second
	case name == "keytron":
		return 15 * time.Second
	default:
		return 6 * time.Second
	}
}

func (m *Model) refreshModelCatalog(ctx context.Context) error {
	providers := []string{}
	for _, name := range m.availableProviders() {
		if !hugeCatalogProvider(name) {
			providers = append(providers, name)
		}
	}
	lists := make([][]modelChoice, len(providers))
	errs := make([]error, len(providers))
	var wg sync.WaitGroup
	for i, name := range providers {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			lists[i], errs[i] = m.providerModels(ctx, name)
		}(i, name)
	}
	wg.Wait()

	choices := []modelChoice{}
	failures := []string{}
	for i, name := range providers {
		if errs[i] != nil {
			// a sleeping computer or a stopped local server simply is not in the list
			failures = append(failures, fmt.Sprintf("%s: %v", name, errs[i]))
			continue
		}
		choices = append(choices, lists[i]...)
	}
	sort.SliceStable(choices, func(i, j int) bool {
		a, b := choices[i], choices[j]
		if a.Section != b.Section {
			return a.Section < b.Section
		}
		if a.Group != b.Group {
			// KEYTRON's own models keep the site's order; other groups go by name
			return a.Group < b.Group
		}
		if a.Section == secKeytron || a.Section == secBYOK {
			return false
		}
		return a.Model < b.Model
	})
	m.modelCatalog = choices
	if len(choices) == 0 && len(failures) > 0 {
		return fmt.Errorf("не удалось получить модели: %s", strings.Join(failures, "; "))
	}
	return nil
}

func (m *Model) providerModels(ctx context.Context, name string) ([]modelChoice, error) {
	be, err := backendForProvider(name, m.Cfg, m.Cfg.Backend.Model)
	if err != nil {
		return nil, err
	}
	pctx, cancel := context.WithTimeout(ctx, catalogTimeout(name))
	defer cancel()
	var infos []backend.ModelInfo
	if l, ok := be.(backend.ModelLister); ok {
		infos, err = l.ModelInfos(pctx)
	} else {
		var ids []string
		ids, err = be.Models(pctx)
		for _, id := range ids {
			infos = append(infos, backend.ModelInfo{ID: id})
		}
	}
	if err != nil {
		return nil, err
	}
	out := make([]modelChoice, 0, len(infos))
	for _, info := range infos {
		if info.ID == "" {
			continue
		}
		c := modelChoice{Provider: name, Model: info.ID, Info: info}
		c.Section, c.Group = m.placeModel(name, info)
		out = append(out, c)
	}
	return out, nil
}

// placeModel decides which section and group a model is listed under.
func (m *Model) placeModel(provider string, info backend.ModelInfo) (int, string) {
	switch {
	case isLocalProvider(provider):
		return secLocal, provider
	case provider == "keytron":
		switch info.OwnedBy {
		case "", "keytron":
			return secKeytron, i18n.S("наши модели")
		case "allan":
			return secMine, info.Worker + i18n.S(" · через KEYTRON")
		default:
			return secBYOK, info.OwnedBy
		}
	case m.personalProvider(provider):
		return secMine, provider
	default:
		return secDirect, provider
	}
}

// personalProvider reports a custom provider that points at one of the
// user's own machines (another PC on the LAN or on Tailscale) rather than a
// cloud API.
func (m *Model) personalProvider(name string) bool {
	if isKnownProvider(name) {
		return false
	}
	p, ok := m.Cfg.Providers[name]
	if !ok {
		return false
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" || strings.HasSuffix(host, ".ts.net") || strings.HasSuffix(host, ".local") || !strings.Contains(host, ".") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10") // Tailscale
	return ip.IsPrivate() || ip.IsLoopback() || cgnat.Contains(ip)
}

// keytronStatus explains why KEYTRON is missing from the list, or returns ""
// when it is configured.
func (m *Model) keytronStatus() string {
	if _, ok := m.Cfg.Providers["keytron"]; ok {
		return ""
	}
	st, token, err := link.Load()
	switch {
	case err != nil || st == nil:
		return i18n.S("не подключено · /connect")
	case token == "":
		return i18n.S("токен не найден в keyring · /connect <код>")
	default:
		return "/connect"
	}
}

// modelDetail is the dim text next to a model: price, abilities, where it runs.
func modelDetail(c modelChoice) (detail string, aux, free bool) {
	low := strings.ToLower(c.Model)
	if strings.Contains(low, "embed") || strings.Contains(low, "ocr") || strings.Contains(low, "rerank") {
		return i18n.S("не для чата"), true, false
	}
	info := c.Info
	parts := []string{}
	cloud := strings.HasSuffix(low, ":cloud") || strings.HasSuffix(low, "-cloud")
	free = info.Free != nil && *info.Free || strings.HasSuffix(low, "-free") || strings.HasSuffix(low, ":free") ||
		!cloud && (c.Section == secLocal || c.Section == secMine)
	switch {
	case cloud:
		parts = append(parts, i18n.S("облако Ollama"))
	case c.Section == secLocal || c.Section == secMine:
		if f := strings.SplitN(info.ID, "/", 4); info.OwnedBy == "allan" && len(f) == 4 {
			parts = append(parts, f[2]) // allan/<worker>/<provider>/<model>
		}
	case free:
		parts = append(parts, i18n.S("бесплатно"))
	case info.Price != nil && info.Price.In != nil && info.Price.Out != nil:
		parts = append(parts, fmt.Sprintf(i18n.S("$%s/$%s за 1M"), shortPrice(*info.Price.In), shortPrice(*info.Price.Out)))
	}
	for _, cap := range info.Caps {
		switch cap {
		case "tools", "vision", "reasoning", "web":
			parts = append(parts, cap)
		}
	}
	if info.Context >= 1000 {
		parts = append(parts, fmt.Sprintf("%dk", info.Context/1000))
	}
	return strings.Join(parts, " · "), false, free
}

func shortPrice(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" {
		return "0"
	}
	return s
}

// modelLabel is the model's name inside its group: the group already says
// which provider or computer it comes from.
func modelLabel(c modelChoice) string {
	if c.Provider != "keytron" {
		return c.Model
	}
	if c.Info.OwnedBy == "allan" && c.Info.Name != "" {
		return c.Info.Name
	}
	if c.Info.OwnedBy != "" {
		return strings.TrimPrefix(c.Model, c.Info.OwnedBy+"/")
	}
	return c.Model
}
