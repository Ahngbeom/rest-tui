package tui

import (
	"github.com/Ahngbeom/rest-tui/internal/history"
	"github.com/Ahngbeom/rest-tui/internal/httpfile"
)

// openRequestMsg asks the app to switch to the Request view for a request
// selected in the Browser.
type openRequestMsg struct {
	filePath string
	file     *httpfile.File
	req      httpfile.Request
}

// rerunMsg asks the app to switch to the Request view pre-loaded with an
// already-resolved past entry and execute it immediately.
type rerunMsg struct {
	entry history.Entry
}

// openRequestFromEntryMsg asks the app to switch to the Request view
// pre-loaded with a past history entry's data, without sending it. This is
// used when the user selects an entry from the Browser's "Recent" list, as
// opposed to rerunMsg (used from the History screen's "r" key), which
// resends immediately.
type openRequestFromEntryMsg struct {
	entry history.Entry
}

// backToBrowserMsg asks the app to return to the Browser screen.
type backToBrowserMsg struct{}

// openHistoryMsg asks the app to switch to the History screen.
type openHistoryMsg struct{}

// execResultMsg carries the outcome of sending a request from requestModel's
// async command back into its Update. Execution failures are carried inside
// entry.Error rather than as a separate error, since a failed send is still a
// history-worthy event.
type execResultMsg struct {
	entry       history.Entry
	historyWarn string
}

// clipboardCopyMsg carries the outcome of a clipboard write triggered by the
// Copy key. token guards against a stale result from a superseded copy
// attempt clobbering a newer one's status.
type clipboardCopyMsg struct {
	token int
	err   error
}

// clipboardCopyExpiredMsg clears a previously-shown clipboard status message
// after a short delay. token guards against clearing a newer status.
type clipboardCopyExpiredMsg struct {
	token int
}

// switchDirMsg asks the app to record path in the directory history and make
// it the active root, re-scanning it in the Browser.
type switchDirMsg struct {
	path string
}
