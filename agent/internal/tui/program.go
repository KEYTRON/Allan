package tui

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// globalProgram is set when Run() is invoked, so background goroutines
// can call program.Send() to push agent events into the bubbletea loop.
var globalProgram *tea.Program

func osUserHomeDir() (string, error) {
	return os.UserHomeDir()
}

func Run(m *Model) error {
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	globalProgram = p
	_, err := p.Run()
	return err
}
