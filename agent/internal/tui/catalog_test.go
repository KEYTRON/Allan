package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/backend"
)

func TestPlaceModel(t *testing.T) {
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"ollama-mac": {Type: "openai", BaseURL: "http://s-macbook-air-1.tail54a59b.ts.net:11434/v1"},
		"lan-box":    {Type: "openai", BaseURL: "http://192.168.1.20:8080/v1"},
		"my-cloud":   {Type: "openai", BaseURL: "https://api.example.com/v1"},
		"openrouter": {Type: "openai", BaseURL: "https://openrouter.ai/api/v1"},
	}
	m := &Model{Cfg: cfg}
	cases := []struct {
		provider string
		info     backend.ModelInfo
		section  int
	}{
		{"ollama", backend.ModelInfo{ID: "qwen3:8b"}, secLocal},
		{"ollama-mac", backend.ModelInfo{ID: "qwen3:8b"}, secMine},
		{"lan-box", backend.ModelInfo{ID: "x"}, secMine},
		{"keytron", backend.ModelInfo{ID: "allan/2/ollama/qwen3:8b", OwnedBy: "allan", Worker: "Mac"}, secMine},
		{"keytron", backend.ModelInfo{ID: "keytron/allan-lm", OwnedBy: "keytron"}, secKeytron},
		{"keytron", backend.ModelInfo{ID: "openrouter/qwen/qwen3-coder", OwnedBy: "openrouter"}, secBYOK},
		{"openrouter", backend.ModelInfo{ID: "qwen/qwen3-coder"}, secDirect},
		{"my-cloud", backend.ModelInfo{ID: "x"}, secDirect},
	}
	for _, c := range cases {
		if got, _ := m.placeModel(c.provider, c.info); got != c.section {
			t.Errorf("placeModel(%s, %s) = %d, want %d", c.provider, c.info.ID, got, c.section)
		}
	}
}

func TestModelLabelAndDetail(t *testing.T) {
	yes := true
	c := modelChoice{Provider: "keytron", Model: "openrouter/qwen/qwen3-coder:free", Section: secBYOK,
		Info: backend.ModelInfo{ID: "openrouter/qwen/qwen3-coder:free", OwnedBy: "openrouter", Free: &yes, Caps: []string{"tools", "files"}, Context: 262144}}
	if got := modelLabel(c); got != "qwen/qwen3-coder:free" {
		t.Errorf("label = %q", got)
	}
	if d, aux, free := modelDetail(c); d != "бесплатно · tools · 262k" || aux || !free {
		t.Errorf("detail = %q aux=%v free=%v", d, aux, free)
	}
	w := modelChoice{Provider: "keytron", Model: "allan/2/ollama/qwen3:8b", Section: secMine,
		Info: backend.ModelInfo{ID: "allan/2/ollama/qwen3:8b", OwnedBy: "allan", Name: "qwen3:8b"}}
	if modelLabel(w) != "qwen3:8b" {
		t.Errorf("worker label = %q", modelLabel(w))
	}
	if d, _, free := modelDetail(w); d != "ollama" || !free {
		t.Errorf("worker detail = %q free=%v", d, free)
	}
}

func TestPickerSectionsAndFree(t *testing.T) {
	items := []pickItem{
		{Label: "a", Section: secLocal, Free: true},
		{Label: "b", Section: secKeytron},
		{Label: "c", Section: secDirect, Free: true},
		{Label: "d", Section: secDirect},
	}
	p := newPicker("t", items, nil)
	p.sections = sectionTitles[:]
	if len(p.visible) != 4 {
		t.Fatalf("all: %d", len(p.visible))
	}
	p.cycleSection(1) // Этот ПК
	if p.section != secLocal || len(p.visible) != 1 {
		t.Fatalf("local: section %d, %d items", p.section, len(p.visible))
	}
	p.cycleSection(1) // empty "Мои ПК" is skipped
	if p.section != secKeytron {
		t.Fatalf("expected KEYTRON, got %d", p.section)
	}
	p.cycleSection(-1)
	p.cycleSection(-1) // back to all
	if p.section != -1 {
		t.Fatalf("expected all, got %d", p.section)
	}
	p.freeOnly = true
	p.filter("")
	if len(p.visible) != 2 {
		t.Fatalf("free: %d", len(p.visible))
	}
	p.freeOnly = false
	p.filter("d")
	if len(p.visible) != 1 || p.items[p.visible[0]].Label != "d" {
		t.Fatalf("text filter broken: %v", p.visible)
	}
}

func TestWheelMovesPicker(t *testing.T) {
	m := New(config.Default(), nil, "/tmp", "test")
	m.width, m.height = 100, 40
	m.openPicker(newPicker("t", []pickItem{{Label: "a"}, {Label: "b"}, {Label: "c"}}, nil))
	m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if m.picker.selected != 2 {
		t.Fatalf("wheel down twice: selected %d", m.picker.selected)
	}
	m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if m.picker.selected != 1 {
		t.Fatalf("wheel up: selected %d", m.picker.selected)
	}
}

func TestPickerFoldsLongSections(t *testing.T) {
	items := []pickItem{
		{Label: "local", Section: secLocal},
		{Label: "picked", Section: secDirect},
		{Label: "hidden-a", Section: secDirect, Folded: true},
		{Label: "hidden-b", Section: secBYOK, Folded: true},
		{Label: "current", Section: secDirect, Folded: true, Current: true},
	}
	p := newPicker("t", items, nil)
	p.setSections(sectionTitles[:])
	if len(p.visible) != 3 || p.hidden[secDirect] != 1 || p.hidden[secBYOK] != 1 {
		t.Fatalf("all view: visible %d hidden %v", len(p.visible), p.hidden)
	}
	if p.foldedNote() == "" {
		t.Fatal("no note about folded models")
	}
	p.filter("hidden") // searching looks through everything
	if len(p.visible) != 2 {
		t.Fatalf("search: %d", len(p.visible))
	}
	p.filter("")
	p.section = secDirect // opening the section shows it whole
	p.filter("")
	if len(p.visible) != 3 || p.foldedNote() != "" {
		t.Fatalf("section: %d, note %q", len(p.visible), p.foldedNote())
	}
}
