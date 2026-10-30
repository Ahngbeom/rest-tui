package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Unsized (<= 0) returns s unchanged; otherwise unsized callers' text would vanish.
func fitLines(s string, width, height int) string {
	if width <= 0 || height <= 0 {
		return s
	}
	lines := strings.Split(ansi.Wrap(s, width, ""), "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

// ansi.Truncate returns "" for width <= 0, which would blank the pre-resize frame.
func truncateLine(s string, width int) string {
	if width <= 0 {
		return s
	}
	return ansi.Truncate(s, width, "…")
}
