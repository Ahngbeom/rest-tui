package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Ahngbeom/rest-tui/internal/history"
	"github.com/Ahngbeom/rest-tui/internal/httpfile"
)

// discoverHTTPFiles recursively finds *.http files under root and returns
// their paths relative to root, sorted.
func discoverHTTPFiles(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".http") {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			found = append(found, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(found)
	return found, nil
}

type fileItem struct {
	rel string
}

func (f fileItem) FilterValue() string { return f.rel }
func (f fileItem) Title() string       { return f.rel }
func (f fileItem) Description() string { return "" }

type requestItem struct {
	req httpfile.Request
}

func (r requestItem) FilterValue() string { return r.req.Name }

func (r requestItem) Title() string {
	if r.req.Name != "" {
		return r.req.Name
	}
	return r.req.Method + " " + r.req.URL
}

func (r requestItem) Description() string {
	if r.req.Name == "" {
		return ""
	}
	return r.req.Method + " " + r.req.URL
}

type focusPane int

const (
	paneFiles focusPane = iota
	paneRequests
)

type browserModel struct {
	root  string
	store *history.Store // used to (re)populate the Recent list

	files    list.Model
	requests list.Model
	focus    focusPane

	selectedFile string
	parsedFile   *httpfile.File
	parseErr     error

	// rootErr holds a failure to scan root itself (e.g. the -dir path
	// doesn't exist), distinct from parseErr (a selected file's .http parse
	// failure) so the Files pane can show the right message instead of the
	// Requests pane misleadingly reporting a "parse error" before any file
	// has even been selected.
	rootErr error

	width, height int
}

func newBrowserModel(root string, store *history.Store) browserModel {
	files := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	files.Title = "HTTP Files"
	files.SetShowHelp(false)
	files.SetFilteringEnabled(false)

	requests := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	requests.Title = "Requests"
	requests.SetShowHelp(false)
	requests.SetFilteringEnabled(false)

	m := browserModel{root: root, store: store, files: files, requests: requests}

	if rels, err := discoverHTTPFiles(root); err == nil {
		items := make([]list.Item, len(rels))
		for i, rel := range rels {
			items[i] = fileItem{rel: rel}
		}
		m.files.SetItems(items)
	} else {
		m.rootErr = err
	}

	m = m.refreshRecent()

	return m
}

// refreshRecent repopulates the requests pane with the most recent history
// entries and relabels it "Recent", for display before any .http file has
// been selected. If there is no history yet, it leaves the pane exactly as
// it already was (the default empty "Requests" list). Callers must only
// invoke this when no file is currently selected (m.parsedFile == nil);
// selectFile always overwrites the pane immediately afterward anyway.
func (m browserModel) refreshRecent() browserModel {
	entries, err := m.store.List(20)
	if err != nil || len(entries) == 0 {
		return m
	}
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = entryItem{entry: e}
	}
	m.requests.Title = "Recent"
	m.requests.SetItems(items)
	return m
}

func (m browserModel) Init() tea.Cmd {
	return nil
}

func (m browserModel) SetSize(width, height int) browserModel {
	m.width, m.height = width, height
	half := width / 2
	// Account for the rounded-border pane frame (2 cols/rows each side).
	m.files.SetSize(half-4, height-2)
	m.requests.SetSize(width-half-4, height-2)
	return m
}

// selectFile parses the currently highlighted file in the files pane and, on
// success, populates the requests pane and moves focus to it. A file that
// fails to parse entirely (I/O error, or every block malformed) leaves focus
// on the files pane; a file where only some blocks fail still populates the
// requests pane with whatever parsed successfully.
func (m browserModel) selectFile() browserModel {
	item, ok := m.files.SelectedItem().(fileItem)
	if !ok {
		return m
	}
	m.requests.Title = "Requests" // selecting a file always replaces any Recent list

	full := filepath.Join(m.root, item.rel)
	m.selectedFile = full

	data, err := os.ReadFile(full)
	if err != nil {
		m.parseErr = err
		m.parsedFile = nil
		m.requests.SetItems(nil)
		return m
	}

	f, parseErr := httpfile.Parse(data)
	items := make([]list.Item, len(f.Requests))
	for i, r := range f.Requests {
		items[i] = requestItem{req: r}
	}
	m.requests.SetItems(items)

	if len(f.Requests) == 0 && parseErr != nil {
		// Every block failed to parse: nothing to show.
		m.parseErr = parseErr
		m.parsedFile = nil
		return m
	}

	// Either some/all requests parsed, or the file legitimately has none
	// (empty/comment-only) with no errors at all -- either way, focus moves
	// to the requests pane, same as before this task's change.
	m.parseErr = nil
	m.parsedFile = f
	m.focus = paneRequests
	return m
}

func (m browserModel) Update(msg tea.Msg) (browserModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(keyMsg, keys.Tab):
			if m.focus == paneFiles && (m.parsedFile != nil || len(m.requests.Items()) > 0) {
				m.focus = paneRequests
			} else {
				m.focus = paneFiles
			}
			return m, nil
		case key.Matches(keyMsg, keys.Back):
			if m.focus == paneRequests {
				m.focus = paneFiles
			}
			return m, nil
		case key.Matches(keyMsg, keys.Enter):
			if m.focus == paneFiles {
				return m.selectFile(), nil
			}
			switch item := m.requests.SelectedItem().(type) {
			case requestItem:
				filePath, file := m.selectedFile, m.parsedFile
				return m, func() tea.Msg {
					return openRequestMsg{filePath: filePath, file: file, req: item.req}
				}
			case entryItem:
				entry := item.entry
				return m, func() tea.Msg {
					return openRequestFromEntryMsg{entry: entry}
				}
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	if m.focus == paneFiles {
		m.files, cmd = m.files.Update(msg)
	} else {
		m.requests, cmd = m.requests.Update(msg)
	}
	return m, cmd
}

// titleStyles reports which title style each pane should use, based on
// which pane currently has focus -- the focused pane's title gets the
// eye-catching accent-color pill, the other pane's fades to plain muted
// text (see paneTitleActiveStyle/paneTitleInactiveStyle in styles.go).
func (m browserModel) titleStyles() (files, requests lipgloss.Style) {
	if m.focus == paneFiles {
		return paneTitleActiveStyle, paneTitleInactiveStyle
	}
	return paneTitleInactiveStyle, paneTitleActiveStyle
}

func (m browserModel) View() string {
	// Local copies so the title-highlight swap below never touches the
	// actual m.files/m.requests (which carry cursor/selection state).
	files := m.files
	requests := m.requests
	files.Styles.Title, requests.Styles.Title = m.titleStyles()

	filesPane := paneStyle
	requestsPane := paneStyle
	if m.focus == paneFiles {
		filesPane = browserActivePaneStyle
	} else {
		requestsPane = browserActivePaneStyle
	}

	left := files.View()
	if m.rootErr != nil {
		left = errorTextStyle.Render("cannot scan directory:\n" + m.rootErr.Error())
	}

	right := requests.View()
	switch {
	case m.parseErr != nil:
		right = errorTextStyle.Render("parse error:\n" + m.parseErr.Error())
	case m.parsedFile != nil && len(m.parsedFile.ParseErrors) > 0:
		right = errorTextStyle.Render("parse warning: "+joinParseErrors(m.parsedFile.ParseErrors)) + "\n\n" + right
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		filesPane.Render(left),
		requestsPane.Render(right),
	)
}

// joinParseErrors formats the blocks that failed to parse as a single
// semicolon-separated line for the warning banner above the requests pane.
func joinParseErrors(errs []*httpfile.ParseError) string {
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}
