package agent

import (
	"fmt"
	"strings"
)

type Scratchpad struct {
	Prev   string
	Plan   []string
	Step   int
	Done   []string
	Errors []string
}

func (s *Scratchpad) Render() string {
	if s == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[SCRATCHPAD]\n")
	if s.Prev != "" {
		sb.WriteString("prev: ")
		sb.WriteString(s.Prev)
		sb.WriteString("\n")
	}
	if len(s.Plan) > 0 {
		sb.WriteString("plan: ")
		for i, p := range s.Plan {
			if i > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(fmt.Sprintf("%d)%s", i+1, p))
		}
		sb.WriteString("\n")
	}
	sb.WriteString(fmt.Sprintf("state: step=%d done=[%s]", s.Step, strings.Join(s.Done, ",")))
	if len(s.Errors) > 0 {
		sb.WriteString(fmt.Sprintf(" errors=[%s]", strings.Join(s.Errors, ",")))
	}
	sb.WriteString("\n[/SCRATCHPAD]")
	return sb.String()
}

func (s *Scratchpad) RecordStep(action string, ok bool, errMsg string) {
	s.Step++
	if ok {
		s.Done = append(s.Done, action)
	} else {
		s.Errors = append(s.Errors, action+":"+truncate(errMsg, 40))
	}
	s.Prev = action
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
