package tools

import (
	"context"
	"strings"
	"testing"
)

type fakeTool struct{ name, group, desc string }

func (f fakeTool) Name() string        { return f.name }
func (f fakeTool) Description() string { return f.desc }
func (f fakeTool) Schema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}}
}
func (f fakeTool) Run(context.Context, map[string]any) (string, error) { return "ran " + f.name, nil }
func (f fakeTool) Group() string                                       { return f.group }
func (f fakeTool) GroupNote() string                                   { return "заметка " + f.group }

func defNames(r *Registry) map[string]bool {
	out := map[string]bool{}
	for _, d := range r.ToolDefs() {
		out[d.Name] = true
	}
	return out
}

func TestAutoToolsStayVisibleUnderThreshold(t *testing.T) {
	r := NewRegistry()
	r.Register(fakeTool{name: "bash"})
	r.RegisterWith(fakeTool{name: "mcp__web__scrape", group: "web", desc: "скачать страницу"}, LoadAuto)
	got := defNames(r)
	if !got["mcp__web__scrape"] || got[LoaderName] {
		t.Fatalf("small auto set should be shown as is: %v", got)
	}
}

func TestDeferredFlow(t *testing.T) {
	r := NewRegistry()
	r.AutoTokens = 10 // любой auto-сервер тяжелее
	r.Register(fakeTool{name: "bash"})
	r.RegisterWith(fakeTool{name: "mcp__web__scrape", group: "web", desc: "скачать страницу по адресу"}, LoadAuto)
	r.RegisterWith(fakeTool{name: "mcp__web__search", group: "web", desc: "поиск в интернете"}, LoadAuto)
	r.RegisterWith(fakeTool{name: "mcp__mem__store", group: "mem", desc: "запомнить факт"}, LoadDeferred)
	r.RegisterWith(fakeTool{name: "mcp__note__get", group: "note", desc: "заметка"}, LoadAlways)

	got := defNames(r)
	if !got["bash"] || !got["mcp__note__get"] || !got[LoaderName] || got["mcp__web__scrape"] || got["mcp__mem__store"] {
		t.Fatalf("before load: %v", got)
	}
	var loaderDesc string
	for _, d := range r.ToolDefs() {
		if d.Name == LoaderName {
			loaderDesc = d.Description
		}
	}
	if !strings.Contains(loaderDesc, "- web (заметка web): scrape, search") || !strings.Contains(loaderDesc, "- mem") {
		t.Fatalf("catalog: %s", loaderDesc)
	}

	// по имени сервера
	if out, _ := r.Run(context.Background(), LoaderName, map[string]any{"names": []any{"web"}}); !strings.Contains(out, "mcp__web__search") {
		t.Fatal(out)
	}
	got = defNames(r)
	if !got["mcp__web__scrape"] || !got["mcp__web__search"] || got["mcp__mem__store"] {
		t.Fatalf("after loading web: %v", got)
	}

	// поиском
	if out, _ := r.Run(context.Background(), LoaderName, map[string]any{"query": "запомнить"}); !strings.Contains(out, "mcp__mem__store") {
		t.Fatal(out)
	}
	if got = defNames(r); got[LoaderName] {
		t.Fatalf("nothing left to defer, loader must go: %v", got)
	}

	// /clear прячет обратно; прямой вызов по имени из каталога подгружает
	r.ResetLoaded()
	if defNames(r)["mcp__mem__store"] {
		t.Fatal("reset did not hide")
	}
	if out, err := r.Run(context.Background(), "mcp__mem__store", nil); err != nil || out != "ran mcp__mem__store" {
		t.Fatal(out, err)
	}
	if !defNames(r)["mcp__mem__store"] {
		t.Fatal("direct call should load the tool")
	}

	// неизвестное имя — понятный ответ с каталогом
	out, _ := r.Run(context.Background(), LoaderName, map[string]any{"names": []any{"nope"}})
	if !strings.Contains(out, "nope") || !strings.Contains(out, "web") {
		t.Fatal(out)
	}
}
