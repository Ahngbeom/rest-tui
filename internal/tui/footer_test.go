package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderHints_DropsMiddleHintsKeepsHelpAndQuit(t *testing.T) {
	hints := []hint{{"e", "cycle env"}, {"enter", "send"}, {"c", "copy"}, {"h", "history"}, {"?", "help"}, {"q", "quit"}}
	cases := []struct {
		width int
		want  string
	}{
		{80, "e cycle env  enter send  c copy  h history  ? help  q quit"},
		{40, "e cycle env  enter send  ? help  q quit"},
		{20, "? help  q quit"},
		{0, "e cycle env  enter send  c copy  h history  ? help  q quit"},
	}
	for _, c := range cases {
		if got := renderHints(hints, c.width); got != c.want {
			t.Errorf("renderHints(width=%d) = %q, want %q", c.width, got, c.want)
		}
	}
	if got := renderHints(hints, 10); lipgloss.Width(got) > 10 {
		t.Errorf("renderHints(width=10) = %q, wider than 10", got)
	}
}
