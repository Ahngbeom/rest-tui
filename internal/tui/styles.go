package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorAccent    = lipgloss.Color("#00D7D7")
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

	// browserActivePaneStyle marks which of Browser's two side-by-side panes
	// currently has focus. Border color alone (paneFocusedStyle) doesn't
	// read clearly enough when two panes sit side by side, so this also
	// switches to a thicker border shape. Single-pane screens (History,
	// Directory History, Request view, Help) keep using paneFocusedStyle
	// as-is -- there's no ambiguity to resolve there, and this change
	// should not alter their look.
	browserActivePaneStyle = paneFocusedStyle.
				Border(lipgloss.ThickBorder())

	// paneTitleActiveStyle/paneTitleInactiveStyle mark which of Browser's
	// two panes has focus via the pane title's background, in addition to
	// the border (browserActivePaneStyle above) -- a solid color block is a
	// much stronger visual signal than border weight/color alone. Both
	// panes' titles otherwise render identically via bubbles/list's default
	// Styles.Title (always the same purple pill) regardless of focus, so
	// without this the title carries no focus information at all.
	paneTitleActiveStyle = lipgloss.NewStyle().
				Background(colorAccent).
				Foreground(lipgloss.Color("0")).
				Bold(true).
				Padding(0, 1)

	paneTitleInactiveStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				Padding(0, 1)

	errorTextStyle  = lipgloss.NewStyle().Foreground(colorError)
	mutedTextStyle  = lipgloss.NewStyle().Foreground(colorFaintText)
	copiedTextStyle = lipgloss.NewStyle().Foreground(colorSuccess)
)
