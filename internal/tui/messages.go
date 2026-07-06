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
