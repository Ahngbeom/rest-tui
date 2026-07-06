// Package output turns a captured HTTP response into a human-readable string:
// status line, headers, and a pretty-printed (optionally colored) body. It
// has no dependency on any UI framework so it can be unit tested directly and
// reused inside the TUI's viewport.
package output

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Ahngbeom/rest-tui/internal/executor"
	"github.com/Ahngbeom/rest-tui/internal/httpfile"
	"github.com/tidwall/pretty"
)

// Options controls rendering.
type Options struct {
	// Color enables ANSI syntax highlighting of JSON bodies.
	Color bool
}

// RenderResponse formats resp as "status (duration)", followed by headers
// sorted by name, followed by the body (pretty-printed if it is valid JSON,
// passed through unchanged otherwise). Sections with nothing to show are
// omitted.
func RenderResponse(resp *executor.Response, opts Options) string {
	sections := []string{fmt.Sprintf("%s (%s)", resp.Status, resp.Duration.Round(time.Millisecond))}

	if headerLines := renderHeaders(resp.Headers); headerLines != "" {
		sections = append(sections, headerLines)
	}

	if bodySection := renderBody(resp.Body, opts); bodySection != "" {
		sections = append(sections, bodySection)
	}

	return strings.Join(sections, "\n\n")
}

func renderHeaders(headers []httpfile.Header) string {
	if len(headers) == 0 {
		return ""
	}
	sorted := make([]httpfile.Header, len(headers))
	copy(sorted, headers)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	lines := make([]string, len(sorted))
	for i, h := range sorted {
		lines[i] = h.Name + ": " + h.Value
	}
	return strings.Join(lines, "\n")
}

// PrettyBody renders body as indented (and optionally colored) JSON if it is
// valid JSON, or returns it unchanged otherwise.
func PrettyBody(body []byte, opts Options) string {
	return renderBody(body, opts)
}

func renderBody(body []byte, opts Options) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	if !json.Valid(body) {
		return trimmed
	}

	formatted := pretty.Pretty(body)
	if opts.Color {
		formatted = pretty.Color(formatted, nil)
	}
	return strings.TrimRight(string(formatted), "\n")
}
