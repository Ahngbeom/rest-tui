package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type hint struct{ key, desc string }

const hintSeparator = "  "

func renderHints(hints []hint, width int) string {
	join := func(hs []hint) string {
		parts := make([]string, len(hs))
		for i, h := range hs {
			parts[i] = h.key + " " + h.desc
		}
		return strings.Join(parts, hintSeparator)
	}
	if width <= 0 {
		return join(hints)
	}
	kept := append([]hint(nil), hints...)
	for len(kept) > 2 && lipgloss.Width(join(kept)) > width {
		kept = append(kept[:len(kept)-3], kept[len(kept)-2:]...)
	}
	s := join(kept)
	if lipgloss.Width(s) > width {
		s = ansi.Truncate(s, width, "")
	}
	return s
}
