package tui

import (
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

// copyStatusDuration is how long a "copied"/"copy failed" toast stays
// visible before clearCopyStatusAfter clears it.
const copyStatusDuration = 2 * time.Second

// copyToClipboardCmd writes text to the OS clipboard and reports the
// outcome, tagged with token, as a clipboardCopyMsg.
func copyToClipboardCmd(text string, token int) tea.Cmd {
	return func() tea.Msg {
		return clipboardCopyMsg{token: token, err: clipboard.WriteAll(text)}
	}
}

// clearCopyStatusAfter returns a command that, after copyStatusDuration,
// sends a clipboardCopyExpiredMsg tagged with token so a "copied"/error
// toast disappears without a persistent ticker.
func clearCopyStatusAfter(token int) tea.Cmd {
	return tea.Tick(copyStatusDuration, func(time.Time) tea.Msg {
		return clipboardCopyExpiredMsg{token: token}
	})
}
