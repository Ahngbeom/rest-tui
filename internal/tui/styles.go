package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	colorAccent    = lipgloss.Color("#00D7D7")
	colorMuted     = lipgloss.Color("8")
	colorError     = lipgloss.Color("9")
	colorSuccess   = lipgloss.Color("10")
	colorWarning   = lipgloss.Color("11")
	colorFaintText = lipgloss.Color("245")

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			Padding(0, 1)

	warnTextStyle = lipgloss.NewStyle().Foreground(colorWarning)

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

	blockedHighlightStyle = errorTextStyle.Bold(true).Reverse(true)
)

func statusCodeStyle(code int) lipgloss.Style {
	color := colorError
	switch {
	case code >= 100 && code < 200:
		color = colorFaintText
	case code >= 200 && code < 300:
		color = colorSuccess
	case code >= 300 && code < 400:
		color = colorAccent
	case code >= 400 && code < 500:
		color = colorWarning
	}
	return lipgloss.NewStyle().Foreground(color)
}

func methodStyle(method string) lipgloss.Style {
	color := colorFaintText
	switch strings.ToUpper(method) {
	case "GET":
		color = colorSuccess
	case "POST":
		color = colorWarning
	case "PUT", "PATCH":
		color = colorAccent
	case "DELETE":
		color = colorError
	}
	return lipgloss.NewStyle().Foreground(color)
}

func formatSize(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

// wrapToWidth word-wraps s to width columns (ANSI-color-code aware) and
// makes any URL in it clickable as a single OSC 8 hyperlink even where the
// wrap splits it across several physical lines -- a bubbles/viewport.Model
// never needs to horizontally clip a line this way, and terminal
// cmd+click/right-click URL detection still resolves the whole address.
// width <= 0 (viewport not sized yet) returns s unchanged.
func wrapToWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	s = strings.ReplaceAll(s, "\t", "    ") // lipgloss Render's default tab width; a raw tab would re-wrap there
	wrapped := ansi.Hardwrap(s, width, false)
	return hyperlinkURLs(s, wrapped)
}
