package tui

import "github.com/charmbracelet/lipgloss"

var (
	ColorCyan     = lipgloss.Color("#4dd9e0")
	ColorGreen    = lipgloss.Color("#3ddc84")
	ColorOrange   = lipgloss.Color("#ff8c42")
	ColorGrey     = lipgloss.Color("#4a4a5a")
	ColorYellow   = lipgloss.Color("#f5c842")
	ColorRed      = lipgloss.Color("#ff5f5f")
	ColorBgHeader = lipgloss.Color("#111114")
	ColorBgMain   = lipgloss.Color("#0c0c0e")
	ColorFg       = lipgloss.Color("#e6e6ea")
	ColorMuted    = lipgloss.Color("#7a7a8a")

	StyleHeader = lipgloss.NewStyle().
			Background(ColorBgHeader).
			Foreground(ColorFg).
			Padding(0, 1)

	StyleLogo = lipgloss.NewStyle().
			Foreground(ColorCyan).
			Background(ColorBgHeader).
			Bold(true)

	StyleHeaderInfo = lipgloss.NewStyle().
			Background(ColorBgHeader).
			Foreground(ColorMuted)

	StyleStatus = lipgloss.NewStyle().
			Background(ColorBgHeader).
			Foreground(ColorMuted).
			Padding(0, 1)

	StyleBadgeYou   = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	StyleBadgeAllan = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)
	StyleBadgeTool  = lipgloss.NewStyle().Foreground(ColorOrange).Bold(true)
	StyleBadgeSys   = lipgloss.NewStyle().Foreground(ColorGrey).Bold(true)
	StyleBadgeWarn  = lipgloss.NewStyle().Foreground(ColorYellow).Bold(true)
	StyleBadgeError = lipgloss.NewStyle().Foreground(ColorRed).Bold(true)

	StyleToolBlock = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderLeft(true).
			BorderForeground(ColorOrange).
			Foreground(ColorFg).
			PaddingLeft(1)

	StylePTYFocused = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderLeft(true).
			BorderForeground(ColorYellow).
			Foreground(ColorFg).
			PaddingLeft(1)

	StylePTYWaiting = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderLeft(true).
			BorderForeground(ColorRed).
			Foreground(ColorFg).
			PaddingLeft(1)

	StylePrompt = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)

	StylePopup = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(ColorCyan).
			Background(ColorBgHeader).
			Foreground(ColorFg).
			Padding(0, 1)
)

const Logo = ` ___  __  __
/ _ |/ / / /__ ____
/ __ / /_/ / _ ` + "`" + `/ _ \
/_/ |_\____/\_,_/_//_/`
