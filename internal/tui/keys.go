package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap holds the bindings shared across screens. Screen-specific bindings
// (e.g. "e" to cycle environments) are matched by literal key string in the
// owning screen's Update instead of living here.
type keyMap struct {
	Quit      key.Binding
	Help      key.Binding
	Back      key.Binding
	Enter     key.Binding
	Tab       key.Binding
	ToggleLog key.Binding // "h": jump to the History screen from anywhere
	Up        key.Binding
	Down      key.Binding
	Rerun     key.Binding
	CycleEnv  key.Binding
	Send      key.Binding
}

var keys = keyMap{
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc", "backspace"),
		key.WithHelp("esc", "back"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "open"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch pane"),
	),
	ToggleLog: key.NewBinding(
		key.WithKeys("h"),
		key.WithHelp("h", "history"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Rerun: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "rerun"),
	),
	CycleEnv: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "cycle env"),
	),
	Send: key.NewBinding(
		key.WithKeys("enter", "ctrl+r"),
		key.WithHelp("enter", "send"),
	),
}
