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
	root string

	files    list.Model
	requests list.Model
	focus    focusPane

	selectedFile string
	parsedFile   *httpfile.File
	parseErr     error

	width, height int
}

func newBrowserModel(root string) browserModel {
	files := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	files.Title = "HTTP Files"
	files.SetShowHelp(false)
	files.SetFilteringEnabled(false)

	requests := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	requests.Title = "Requests"
	requests.SetShowHelp(false)
	requests.SetFilteringEnabled(false)

	m := browserModel{root: root, files: files, requests: requests}

	if rels, err := discoverHTTPFiles(root); err == nil {
		items := make([]list.Item, len(rels))
		for i, rel := range rels {
			items[i] = fileItem{rel: rel}
		}
		m.files.SetItems(items)
	} else {
		m.parseErr = err
	}

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
	m.parsedFile = f
	items := make([]list.Item, len(f.Requests))
	for i, r := range f.Requests {
		items[i] = requestItem{req: r}
	}
	m.requests.SetItems(items)

	if len(f.Requests) == 0 {
		m.parseErr = parseErr
		return m
	}
	m.parseErr = nil
	m.focus = paneRequests
	return m
}

func (m browserModel) Update(msg tea.Msg) (browserModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(keyMsg, keys.Tab):
			if m.focus == paneFiles && m.parsedFile != nil {
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
			if item, ok := m.requests.SelectedItem().(requestItem); ok {
				filePath, file := m.selectedFile, m.parsedFile
				return m, func() tea.Msg {
					return openRequestMsg{filePath: filePath, file: file, req: item.req}
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

func (m browserModel) View() string {
	filesPane := paneStyle
	requestsPane := paneStyle
	if m.focus == paneFiles {
		filesPane = paneFocusedStyle
	} else {
		requestsPane = paneFocusedStyle
	}

	right := m.requests.View()
	switch {
	case m.parseErr != nil:
		right = errorTextStyle.Render("parse error:\n" + m.parseErr.Error())
	case m.parsedFile != nil && len(m.parsedFile.ParseErrors) > 0:
		right = errorTextStyle.Render("parse warning: "+joinParseErrors(m.parsedFile.ParseErrors)) + "\n\n" + right
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		filesPane.Render(m.files.View()),
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
