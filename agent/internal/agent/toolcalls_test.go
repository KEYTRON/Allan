package agent

import (
	"testing"

	"github.com/keytron/allan/agent/internal/backend"
)

func TestCloseToolCallsAnswersEveryCall(t *testing.T) {
	calls := []backend.ToolCall{{ID: "a", Name: "bash"}, {ID: "b", Name: "bash"}}
	hist := []backend.Message{
		{Role: backend.RoleUser, Content: "q"},
		{Role: backend.RoleAssistant, ToolCalls: calls},
		{Role: backend.RoleTool, ToolCallID: "a", Content: "ok"},
		{Role: backend.RoleUser, Content: "долго ещё?"},
	}
	got := closeToolCalls(hist)
	if len(got) != 5 {
		t.Fatalf("len = %d, want 5", len(got))
	}
	if got[3].Role != backend.RoleTool || got[3].ToolCallID != "b" || got[3].Content != skippedToolResult {
		t.Fatalf("missing result for b not inserted: %+v", got[3])
	}
	if got[4].Content != "долго ещё?" {
		t.Fatalf("user message moved: %+v", got[4])
	}
	if len(hist) != 4 {
		t.Fatal("input history was modified")
	}
}

func TestCloseToolCallsKeepsCompleteHistory(t *testing.T) {
	hist := []backend.Message{
		{Role: backend.RoleUser, Content: "q"},
		{Role: backend.RoleAssistant, ToolCalls: []backend.ToolCall{{ID: "a"}}},
		{Role: backend.RoleTool, ToolCallID: "a"},
		{Role: backend.RoleAssistant, Content: "done"},
	}
	got := closeToolCalls(hist)
	if len(got) != len(hist) {
		t.Fatalf("len = %d, want %d", len(got), len(hist))
	}
}
