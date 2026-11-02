package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ahngbeom/rest-tui/internal/history"
	"github.com/Ahngbeom/rest-tui/internal/httpfile"
	"github.com/Ahngbeom/rest-tui/internal/output"
)

// discoverHTTPFiles recursively finds *.http files under root and returns
// their paths relative to root, sorted. Unreadable subdirectories are skipped
// and counted; only a failure on root itself is an error.
func discoverHTTPFiles(root string) (rels []string, skipped int, err error) {
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			skipped++
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".http") {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			rels = append(rels, rel)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(rels)
	return rels, skipped, nil
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

func (r requestItem) FilterValue() string {
	return r.req.Name + " " + r.req.Method + " " + r.req.URL
}

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

	// scanNotice reports a partially successful scan (unreadable
	// subdirectories were skipped); the App shows it in the status line.
	scanNotice string

	width, height int
}

func newBrowserModel(root string, store *history.Store) browserModel {
	fileDelegate := list.NewDefaultDelegate()
	fileDelegate.ShowDescription = false
	fileDelegate.SetSpacing(0)
	files := list.New(nil, fileDelegate, 0, 0)
	files.Title = "HTTP Files"
	files.SetShowHelp(false)
	files.SetFilteringEnabled(true)

	requests := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	requests.Title = "Requests"
	requests.SetShowHelp(false)
	requests.SetFilteringEnabled(true)

	m := browserModel{root: root, store: store, files: files, requests: requests}

	if rels, skipped, err := discoverHTTPFiles(root); err == nil {
		if skipped > 0 {
			m.scanNotice = fmt.Sprintf("skipped %d unreadable dirs", skipped)
		}
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
	m.requests.SetDelegate(entryRowDelegate{})
	m.requests.ResetFilter()
	m.requests.SetItems(items)
	m.requests.Select(0)
	return m
}

// rescan rebuilds the browser from disk, keeping the cursor, the open file
// and the focused pane where they were when those still exist. A filter is
// not carried over.
func (m browserModel) rescan() browserModel {
	filesRel := ""
	if it, ok := m.files.SelectedItem().(fileItem); ok {
		filesRel = it.rel
	}
	openRel := ""
	if m.parsedFile != nil {
		if rel, err := filepath.Rel(m.root, m.selectedFile); err == nil {
			openRel = rel
		}
	}
	reqIdx, focus := m.requests.Index(), m.focus

	n := newBrowserModel(m.root, m.store).SetSize(m.width, m.height)
	indexOf := func(rel string) int {
		for i, it := range n.files.Items() {
			if f, ok := it.(fileItem); ok && f.rel == rel {
				return i
			}
		}
		return -1
	}
	if openRel != "" {
		if i := indexOf(openRel); i >= 0 {
			n.files.Select(i)
			n = n.selectFile()
			if len(n.requests.Items()) == 0 {
				n.focus = paneFiles
				return n
			}
			n.requests.Select(min(reqIdx, len(n.requests.Items())-1))
			n.focus = focus
			return n
		}
	}
	if i := indexOf(filesRel); i >= 0 {
		n.files.Select(i)
	}
	n.focus = paneFiles
	return n
}

func (m browserModel) Init() tea.Cmd {
	return nil
}

func (m browserModel) SetSize(width, height int) browserModel {
	m.width, m.height = width, height
	filesW, reqW := m.paneWidths()
	frameW, frameH := paneStyle.GetHorizontalFrameSize(), paneStyle.GetVerticalFrameSize()
	m.files.SetSize(max(0, filesW-frameW), max(0, height-frameH))

	warnRows := 0
	if lines := m.parseWarningLines(max(0, reqW-frameW)); len(lines) > 0 {
		warnRows = len(lines) + 1
	}
	m.requests.SetSize(max(0, reqW-frameW), max(0, height-frameH-warnRows))
	return m
}

func (m browserModel) paneWidths() (filesW, reqW int) {
	filesW = m.width * 40 / 100
	return filesW, m.width - filesW
}

func (m browserModel) parseWarningLines(width int) []string {
	if m.parsedFile == nil || len(m.parsedFile.ParseErrors) == 0 {
		return nil
	}
	text := "parse warning: " + joinParseErrors(m.parsedFile.ParseErrors)
	lines := strings.Split(ansi.Wrap(text, width, ""), "\n")
	if len(lines) <= 3 {
		return lines
	}
	return []string{lines[0], lines[1], fmt.Sprintf("… +%d more", len(lines)-2)}
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
	m.requests.SetDelegate(list.NewDefaultDelegate())
	m.requests.ResetFilter()
	m.requests.Title = "Requests" // selecting a file always replaces any Recent list

	full := filepath.Join(m.root, item.rel)
	m.selectedFile = full

	data, err := os.ReadFile(full)
	if err != nil {
		m.parseErr = err
		m.parsedFile = nil
		m.requests.SetItems(nil)
		m.requests.Select(0)
		return m.SetSize(m.width, m.height)
	}

	f, parseErr := httpfile.Parse(data)
	items := make([]list.Item, len(f.Requests))
	for i, r := range f.Requests {
		items[i] = requestItem{req: r}
	}
	m.requests.SetItems(items)
	m.requests.Select(0)

	if len(f.Requests) == 0 && parseErr != nil {
		// Every block failed to parse: nothing to show.
		m.parseErr = parseErr
		m.parsedFile = nil
		return m.SetSize(m.width, m.height)
	}

	// Either some/all requests parsed, or the file legitimately has none
	// (empty/comment-only) with no errors at all -- either way, focus moves
	// to the requests pane, same as before this task's change.
	m.parseErr = nil
	m.parsedFile = f
	m.focus = paneRequests
	return m.SetSize(m.width, m.height)
}

// listCapturesKey reports whether l's built-in filter handling must see msg
// before the screen's own bindings: every key while the filter input is
// active, and esc once a filter is applied (it clears the filter).
func listCapturesKey(l list.Model, msg tea.KeyMsg) bool {
	switch l.FilterState() {
	case list.Filtering:
		return true
	case list.FilterApplied:
		return msg.Type == tea.KeyEsc
	}
	return false
}

func (m browserModel) focusedList() list.Model {
	if m.focus == paneFiles {
		return m.files
	}
	return m.requests
}

func (m browserModel) isFiltering() bool {
	return m.focusedList().FilterState() == list.Filtering
}

func (m browserModel) Update(msg tea.Msg) (browserModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && !listCapturesKey(m.focusedList(), keyMsg) {
		switch {
		case key.Matches(keyMsg, keys.Tab):
			if m.focus == paneFiles && len(m.requests.Items()) > 0 {
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

	filesW, reqW := m.paneWidths()
	frameW, frameH := paneStyle.GetHorizontalFrameSize(), paneStyle.GetVerticalFrameSize()
	borderW, borderH := paneStyle.GetHorizontalBorderSize(), paneStyle.GetVerticalBorderSize()
	innerH := max(0, m.height-frameH)

	left := files.View()
	if m.rootErr != nil {
		left = errorTextStyle.Render(fitLines("cannot scan directory:\n"+m.rootErr.Error(), max(0, filesW-frameW), innerH))
	}

	if m.rootErr == nil && len(m.files.Items()) == 0 {
		left = emptyPane(files, "No .http files under "+output.Sanitize(m.root)+" — d recent dirs · o open path", max(0, filesW-frameW), innerH)
	}

	right := requests.View()
	switch {
	case m.parsedFile == nil && m.parseErr == nil && len(m.requests.Items()) == 0:
		right = emptyPane(requests, "Select a file and press enter", max(0, reqW-frameW), innerH)
	case m.parseErr != nil:
		right = errorTextStyle.Render(fitLines("parse error:\n"+m.parseErr.Error(), max(0, reqW-frameW), innerH))
	default:
		if lines := m.parseWarningLines(max(0, reqW-frameW)); len(lines) > 0 {
			for i, ln := range lines {
				lines[i] = errorTextStyle.Render(ln)
			}
			right = strings.Join(lines, "\n") + "\n\n" + right
		}
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		filesPane.Width(max(0, filesW-borderW)).Height(max(0, m.height-borderH)).Render(left),
		requestsPane.Width(max(0, reqW-borderW)).Height(max(0, m.height-borderH)).Render(right),
	)
}

// emptyPane renders l's title with a muted guidance line in place of the list.
func emptyPane(l list.Model, text string, width, height int) string {
	title := l.Styles.TitleBar.Render(l.Styles.Title.Render(l.Title))
	body := mutedTextStyle.Render(fitLines(text, width, max(0, height-lipgloss.Height(title))))
	return title + "\n" + body
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
