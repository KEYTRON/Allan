package tools

import (
	"fmt"
	"sort"
	"strings"

	"github.com/keytron/allan/agent/internal/backend"
)

// LoaderName — служебный инструмент, которым модель подгружает отложенные.
const LoaderName = "load_tools"

const maxQueryHits = 6

func groupOf(t Tool) (string, string) {
	if g, ok := t.(Grouped); ok {
		return g.Group(), g.GroupNote()
	}
	return "", ""
}

// shortName — имя без префикса mcp__<сервер>__.
func shortName(t Tool) string {
	n := t.Name()
	if g, _ := groupOf(t); g != "" {
		if i := strings.LastIndex(n, "__"); i >= 0 {
			return n[i+2:]
		}
	}
	return n
}

// Catalog — компактный список отложенного: строка на группу.
func Catalog(deferred []Tool) string {
	type grp struct {
		note  string
		names []string
	}
	groups := map[string]*grp{}
	var order []string
	for _, t := range deferred {
		g, note := groupOf(t)
		if g == "" {
			g = "прочие"
		}
		if groups[g] == nil {
			groups[g] = &grp{note: note}
			order = append(order, g)
		}
		groups[g].names = append(groups[g].names, shortName(t))
	}
	sort.Strings(order)
	var sb strings.Builder
	for _, g := range order {
		x := groups[g]
		sb.WriteString("- " + g)
		if x.note != "" {
			sb.WriteString(" (" + x.note + ")")
		}
		sb.WriteString(": " + strings.Join(x.names, ", ") + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func loaderDef(deferred []Tool) backend.ToolDef {
	return backend.ToolDef{
		Name: LoaderName,
		Description: "Подгружает инструменты MCP-серверов, которые не показаны сразу, чтобы не тратить контекст. " +
			"Если задаче нужен инструмент из каталога ниже — сначала вызови load_tools, после этого он станет " +
			"обычным инструментом (имя mcp__<сервер>__<инструмент>). Каталог (сервер: инструменты):\n" + Catalog(deferred),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"names": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Имена инструментов (полные или короткие из каталога) или имена серверов целиком",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Поиск по словам в именах и описаниях, если точное имя неизвестно",
				},
			},
		},
	}
}

func (r *Registry) runLoader(params map[string]any) (string, error) {
	deferred := r.Deferred()
	var picked []Tool
	seen := map[string]bool{}
	add := func(t Tool) {
		if !seen[t.Name()] {
			seen[t.Name()] = true
			picked = append(picked, t)
		}
	}
	var unknown []string
	if raw, ok := params["names"].([]any); ok {
		for _, v := range raw {
			want := strings.TrimSpace(fmt.Sprint(v))
			if want == "" {
				continue
			}
			hit := false
			for _, t := range deferred {
				g, _ := groupOf(t)
				if t.Name() == want || shortName(t) == want || strings.EqualFold(g, want) {
					add(t)
					hit = true
				}
			}
			if !hit {
				if r.IsActive(want) {
					hit = true // уже доступен
				}
			}
			if !hit {
				unknown = append(unknown, want)
			}
		}
	}
	if q, _ := params["query"].(string); strings.TrimSpace(q) != "" {
		words := strings.Fields(strings.ToLower(q))
		type scored struct {
			t Tool
			s int
		}
		var hits []scored
		for _, t := range deferred {
			g, _ := groupOf(t)
			hay := strings.ToLower(t.Name() + " " + g + " " + t.Description())
			s := 0
			for _, w := range words {
				if strings.Contains(hay, w) {
					s++
				}
			}
			if s > 0 {
				hits = append(hits, scored{t, s})
			}
		}
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].s > hits[j].s })
		for i, h := range hits {
			if i >= maxQueryHits {
				break
			}
			add(h.t)
		}
	}
	if len(picked) == 0 {
		msg := "Ничего не подгружено."
		if len(unknown) > 0 {
			msg += " Нет таких инструментов: " + strings.Join(unknown, ", ") + "."
		}
		return msg + " Каталог:\n" + Catalog(deferred), nil
	}
	names := make([]string, len(picked))
	var sb strings.Builder
	sb.WriteString("Подгружено, теперь вызывай напрямую:\n")
	for i, t := range picked {
		names[i] = t.Name()
		d := strings.SplitN(strings.TrimSpace(t.Description()), "\n", 2)[0]
		if r := []rune(d); len(r) > 160 {
			d = string(r[:160]) + "…"
		}
		sb.WriteString("- " + t.Name() + " — " + d + "\n")
	}
	r.MarkLoaded(names...)
	if len(unknown) > 0 {
		sb.WriteString("Не найдены: " + strings.Join(unknown, ", ") + "\n")
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}
