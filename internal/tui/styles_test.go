package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestFormatSize(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1229, "1.2 KB"},
		{1536, "1.5 KB"},
		{5 * 1024 * 1024, "5.0 MB"},
	}
	for _, c := range cases {
		if got := formatSize(c.n); got != c.want {
			t.Errorf("formatSize(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestStatusCodeStyleColors(t *testing.T) {
	cases := []struct {
		code int
		want lipgloss.Color
	}{
		{100, colorFaintText},
		{200, colorSuccess},
		{301, colorAccent},
		{404, colorWarning},
		{500, colorError},
		{0, colorError},
	}
	for _, c := range cases {
		if got := statusCodeStyle(c.code).GetForeground(); got != c.want {
			t.Errorf("statusCodeStyle(%d) foreground = %v, want %v", c.code, got, c.want)
		}
	}
}

func TestMethodStyleColors(t *testing.T) {
	cases := []struct {
		method string
		want   lipgloss.Color
	}{
		{"GET", colorSuccess},
		{"get", colorSuccess},
		{"POST", colorWarning},
		{"PUT", colorAccent},
		{"PATCH", colorAccent},
		{"DELETE", colorError},
		{"TRACE", colorFaintText},
	}
	for _, c := range cases {
		if got := methodStyle(c.method).GetForeground(); got != c.want {
			t.Errorf("methodStyle(%q) foreground = %v, want %v", c.method, got, c.want)
		}
	}
}
