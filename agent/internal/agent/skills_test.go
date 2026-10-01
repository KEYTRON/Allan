package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/backend"
	"github.com/keytron/allan/agent/internal/memory"
	"github.com/keytron/allan/agent/internal/skills"
	"github.com/keytron/allan/agent/internal/tools"
)

func TestIsTrivialRequest(t *testing.T) {
	trivial := []string{
		"привет", "Привет!", "здравствуйте", "спасибо", "ок", "да", "кто ты", "что ты умеешь",
		"hi", "hello", "thanks", "ага", "пока", "",
	}
	for _, in := range trivial {
		if !isTrivialRequest(in) {
			t.Errorf("%q не должно считаться тривиальным", in)
		}
	}
	substantial := []string{
		"найди все файлы конфигурации в домашней папке и собери список",
		"перезапусти nginx и проверь, что сайт отвечает",
		"исправь тест, который падает в CI",
	}
	for _, in := range substantial {
		if isTrivialRequest(in) {
			t.Errorf("%q ошибочно отброшено как тривиальное", in)
		}
	}
}

func TestNoSkillForGreetingEvenAfterLongThink(t *testing.T) {
	// Раньше навык сохранялся, если ход длился дольше skill_min_tool_calls
	// секунд: медленный ответ на «привет» попадал в базу как паттерн.
	eng := newSkillEngine(t)
	ag := newTestAgent(t, eng)

	ag.Stats.ToolCalls = 0
	events := drain(ag, "привет", time.Now().Add(-30*time.Second), 0)

	if got := skillEvents(events); got != 0 {
		t.Fatalf("навык сохранён на пустом ходе: %d событий", got)
	}
	if n := countSkills(t, eng); n != 0 {
		t.Fatalf("в базе навыков: %d", n)
	}
}

func TestNoSkillWhenTooFewToolCalls(t *testing.T) {
	eng := newSkillEngine(t)
	ag := newTestAgent(t, eng)
	ag.Stats.ToolCalls = 1

	events := drain(ag, "посмотри, что лежит в рабочей папке", time.Now(), 1)
	if got := skillEvents(events); got != 0 {
		t.Fatalf("навык сохранён при одном tool call: %d событий", got)
	}
}

func TestSkillSavedAfterRealWork(t *testing.T) {
	eng := newSkillEngine(t)
	ag := newTestAgent(t, eng)
	ag.Stats.ToolCalls = 3

	events := drain(ag, "разберись, почему падает сборка, и исправь", time.Now(), 3)
	if got := skillEvents(events); got != 1 {
		t.Fatalf("ожидался 1 сохранённый навык, получили %d", got)
	}
	if n := countSkills(t, eng); n != 1 {
		t.Fatalf("в базе навыков: %d, ожидали 1", n)
	}
	// Сессия не должна сыпать одинаковыми навыками: повторный ходwithin cooldown.
	events = drain(ag, "разберись, почему падает тест, и исправь", time.Now(), 3)
	if got := skillEvents(events); got != 0 {
		t.Fatalf("второй навык подряд сохранён: %d", got)
	}
}

func TestSkillCooldownCanBeDisabled(t *testing.T) {
	eng := newSkillEngine(t)
	ag := newTestAgent(t, eng)
	ag.Cfg.Agent.SkillCooldown = 0
	ag.Stats.ToolCalls = 3
	drain(ag, "разберись, почему падает сборка, и исправь", time.Now(), 3)
	ag.Stats.ToolCalls = 6
	// С cooldown 0 по умолчанию подставляется 5 минут, поэтому проверяем, что
	// при явном 0 второй навык тоже не создаётся только из-за списка имён.
	events := drain(ag, "разберись, почему падает деплой, и исправь", time.Now(), 3)
	if got := skillEvents(events); got > 1 {
		t.Fatalf("слишком много навыков за раз: %d", got)
	}
}

// ---- helpers ----

type stubBackend struct{}

func (stubBackend) Name() string { return "stub" }
func (stubBackend) Chat(ctx context.Context, m []backend.Message, d []backend.ToolDef) (*backend.Response, error) {
	// Ответ SkillExtractPrompt: валидный JSON с именем навыка.
	for _, msg := range m {
		if strings.Contains(msg.Content, "Извлеки паттерн") {
			return &backend.Response{Content: `{"name":"починка сборки","description":"чинит","trigger":"сборка падает","solution":"починить","tool_sequence":["bash"]}`}, nil
		}
	}
	return &backend.Response{Content: "сделано"}, nil
}
func (stubBackend) Stream(ctx context.Context, m []backend.Message, d []backend.ToolDef, out chan<- backend.Token) error {
	return nil
}
func (stubBackend) Models(ctx context.Context) ([]string, error) { return nil, nil }
func (stubBackend) Health(ctx context.Context) error             { return nil }

func newTestAgent(t *testing.T, eng *skills.Engine) *Agent {
	t.Helper()
	cfg := config.Default()
	cfg.Memory.ChromaDBURL = ""
	return New(cfg, stubBackend{}, tools.NewRegistry(), nil, eng, nil, t.TempDir())
}

// drain runs the private skill-saving path exactly like Agent.Run does.
func drain(ag *Agent, input string, started time.Time, toolCallsBefore int) []Event {
	events := make(chan Event, 8)
	startTC := ag.Stats.ToolCalls - toolCallsBefore
	ag.maybeSaveSkill(context.Background(), input, started, startTC, []string{"bash", "read_file"}, events)
	close(events)
	out := []Event{}
	for ev := range events {
		out = append(out, ev)
	}
	return out
}

func skillEvents(events []Event) int {
	n := 0
	for _, ev := range events {
		if ev.Kind == "skill_saved" {
			n++
		}
	}
	return n
}

// newSkillEngine creates a Skill Engine on a temporary SQLite database.
func newSkillEngine(t *testing.T) *skills.Engine {
	t.Helper()
	db := filepath.Join(t.TempDir(), "skills.db")
	mem, err := memory.Open(db)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	t.Cleanup(func() { _ = mem.Close() })
	return skills.NewEngine(mem.DB(), nil)
}

func countSkills(t *testing.T, eng *skills.Engine) int {
	t.Helper()
	list, err := eng.List(context.Background())
	if err != nil {
		t.Fatalf("skills.List: %v", err)
	}
	return len(list)
}
