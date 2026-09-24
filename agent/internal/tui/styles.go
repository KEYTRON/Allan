package tui

import "github.com/charmbracelet/lipgloss"

// No style sets a background: the terminal's own background shows through
// everywhere, so there are no mismatched patches between regions.
var (
	ColorAccent = lipgloss.Color("#4dd9e0") // cyan: brand, user, focus
	ColorGreen  = lipgloss.Color("#3ddc84")
	ColorOrange = lipgloss.Color("#ff9f5a")
	ColorYellow = lipgloss.Color("#f5c842")
	ColorRed    = lipgloss.Color("#ff6b6b")
	ColorFg     = lipgloss.Color("#e6e6ea")
	ColorMuted  = lipgloss.Color("#8a8a9a")
	ColorDim    = lipgloss.Color("#55556a")

	StyleBrand = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	StyleMuted = lipgloss.NewStyle().Foreground(ColorMuted)
	StyleDim   = lipgloss.NewStyle().Foreground(ColorDim)
	StyleText  = lipgloss.NewStyle().Foreground(ColorFg)
	StyleKey   = lipgloss.NewStyle().Foreground(ColorAccent)

	StyleUserBar = lipgloss.NewStyle().
			BorderStyle(lipgloss.ThickBorder()).
			BorderLeft(true).
			BorderForeground(ColorAccent).
			Foreground(ColorFg).
			PaddingLeft(1)

	StyleDotAllan = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)
	StyleDotTool  = lipgloss.NewStyle().Foreground(ColorOrange).Bold(true)
	StyleWarn     = lipgloss.NewStyle().Foreground(ColorYellow)
	StyleError    = lipgloss.NewStyle().Foreground(ColorRed)
	StyleOK       = lipgloss.NewStyle().Foreground(ColorGreen)

	StyleInput = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccent).
			Padding(0, 1)

	StyleInputBusy = StyleInput.BorderForeground(ColorDim)

	StylePopup = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(ColorDim).
			Padding(0, 1)

	StylePopupSelected = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)

	StyleWelcome = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(ColorDim).
			Padding(1, 2)

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
)

// Logo is drawn with box-drawing blocks, so it reads as "ALLAN" in any
// monospace font instead of a pile of slashes.
const Logo = `▄▀█ █   █   ▄▀█ █▄ █
█▀█ █▄▄ █▄▄ █▀█ █ ▀█`
