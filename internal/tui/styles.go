package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorAccent    = lipgloss.Color("6")
	colorMuted     = lipgloss.Color("8")
	colorError     = lipgloss.Color("9")
	colorSuccess   = lipgloss.Color("10")
	colorFaintText = lipgloss.Color("245")

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			Padding(0, 1)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorFaintText).
			Padding(0, 1)

	statusStyle = lipgloss.NewStyle().
			Foreground(colorError).
			Padding(0, 1)

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted).
			Padding(0, 1)

	paneFocusedStyle = paneStyle.
				BorderForeground(colorAccent)

	errorTextStyle = lipgloss.NewStyle().Foreground(colorError)
	mutedTextStyle = lipgloss.NewStyle().Foreground(colorFaintText)
)
