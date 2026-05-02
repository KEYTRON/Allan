package agent

import (
	"strings"
	"testing"
)

func TestScratchpadRender(t *testing.T) {
	s := &Scratchpad{
		Plan: []string{"read", "build", "test"},
	}
	s.RecordStep("read", true, "")
	s.RecordStep("build", false, "compile error")
	out := s.Render()
	if !strings.Contains(out, "[SCRATCHPAD]") {
		t.Error("missing scratchpad marker")
	}
	if !strings.Contains(out, "step=2") {
		t.Errorf("unexpected step: %s", out)
	}
	if !strings.Contains(out, "errors=") {
		t.Errorf("missing errors: %s", out)
	}
}
