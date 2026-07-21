package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHyperlinkURLs_QuotedJSONURLWrapsAsOneLink(t *testing.T) {
	longURL := "https://manager.payssam.kr/partner/redirect/8460/eyJ0b2tlblR5cGUiOiJCZWFyZXIiLCJhbGciOiJIUzI1NiJ9.eyJtZW1iZXJJZHgiOiJVUzgyNUY0RUJFODBEMTExRjFBNUJGMEE2RkZFMDU3QTEzIiwiZXhwIjoxNzg0NjkzMDQ1LCJpYXQiOjE3ODQ2MDY2NDV9.o3H9JSrxSvtlVLdH8VBR3qDRKxKlk00lJGDtOk2YV7Q/Y"
	original := `  "url": "` + longURL + `"` + "\x1b[0m"
	wrapped := ansi.Hardwrap(original, 40, false)

	got := hyperlinkURLs(original, wrapped)

	open := "\x1b]8;;" + longURL + "\x1b\\"
	const closeSeq = "\x1b]8;;\x1b\\"
	if n := strings.Count(got, open); n != 1 {
		t.Errorf("open marker count = %d, want 1; got:\n%s", n, got)
	}
	if n := strings.Count(got, closeSeq); n != 1 {
		t.Errorf("close marker count = %d, want 1; got:\n%s", n, got)
	}
	if strings.Index(got, open) > strings.Index(got, closeSeq) {
		t.Errorf("open marker should appear before close marker; got:\n%s", got)
	}
	// The URL is genuinely split across several physical lines.
	if lines := strings.Count(got, "\n"); lines < 2 {
		t.Fatalf("expected the URL to wrap across multiple lines, got %d newlines", lines)
	}
}

func TestHyperlinkURLs_BareHeaderURLDoesNotSwallowNextLine(t *testing.T) {
	original := "Location: https://manager.payssam.kr/partner/redirect/8460/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/end\n" +
		"Content-Type: application/json\n"
	wrapped := ansi.Hardwrap(original, 30, false)

	got := hyperlinkURLs(original, wrapped)

	if !strings.Contains(got, "Content-Type: application/json") {
		t.Errorf("Content-Type header got corrupted, want it intact:\n%s", got)
	}
	openIdx := strings.Index(got, "\x1b]8;;https://manager.payssam.kr")
	if openIdx == -1 {
		t.Fatalf("missing OSC8 open for the header URL; got:\n%s", got)
	}
	if strings.Contains(got[openIdx:strings.Index(got, "\x1b]8;;\x1b\\")], "Content-Type") {
		t.Errorf("hyperlink span swallowed the next header's name; got:\n%s", got)
	}
}

func TestHyperlinkURLs_MultipleURLsEachGetOwnPair(t *testing.T) {
	original := "X-First: https://first.example.com/a/very/long/path/segment/to/force/a/wrap/here\n" +
		"X-Second: https://second.example.com/another/quite/long/path/to/force/a/wrap/too\n"
	wrapped := ansi.Hardwrap(original, 30, false)

	got := hyperlinkURLs(original, wrapped)

	for _, url := range []string{
		"https://first.example.com/a/very/long/path/segment/to/force/a/wrap/here",
		"https://second.example.com/another/quite/long/path/to/force/a/wrap/too",
	} {
		if n := strings.Count(got, "\x1b]8;;"+url+"\x1b\\"); n != 1 {
			t.Errorf("open marker count for %q = %d, want 1", url, n)
		}
	}
	if n := strings.Count(got, "\x1b]8;;\x1b\\"); n != 2 {
		t.Errorf("close marker count = %d, want 2; got:\n%s", n, got)
	}
}

func TestHyperlinkURLs_NoURLReturnsWrappedUnchanged(t *testing.T) {
	original := "  \x1b[1m\x1b[94m\"msg\"\x1b[0m\x1b[1m:\x1b[0m \x1b[32m\"개시된 매장이 존재하지 않습니다.\"\x1b[0m"
	wrapped := ansi.Hardwrap(original, 20, false)

	got := hyperlinkURLs(original, wrapped)

	if got != wrapped {
		t.Errorf("expected text without a URL to pass through unchanged;\ngot:  %q\nwant: %q", got, wrapped)
	}
}

func TestHyperlinkURLs_MultiByteContentBeforeMatchDoesNotDesyncOffsets(t *testing.T) {
	// The "─" divider and Korean text are multi-byte UTF-8 runes. The regexp
	// match offsets urlPattern reports are byte offsets (Go's regexp package
	// always reports bytes), so any multi-byte content appearing before a
	// URL match must not shift the two sides of the offset mapping out of
	// sync relative to each other -- this reproduces the shape of
	// renderEntryDetail's actual output (request line, blank line, a
	// 40-rune/120-byte "─" divider, then a response body).
	original := "GET https://example.com/a\n\n" +
		"\x1b[38;5;245m" + strings.Repeat("─", 40) + "\x1b[0m" + "\n\n" +
		"개시된 매장이 존재하지 않습니다\n" +
		`{"url": "https://second.example.com/long/enough/path/to/force/a/wrap/for/this/case"}`

	wrapped := ansi.Hardwrap(original, 40, false)
	got := hyperlinkURLs(original, wrapped)

	for _, url := range []string{
		"https://example.com/a",
		"https://second.example.com/long/enough/path/to/force/a/wrap/for/this/case",
	} {
		open := "\x1b]8;;" + url + "\x1b\\"
		if n := strings.Count(got, open); n != 1 {
			t.Errorf("open marker count for %q = %d, want 1; got:\n%s", url, n, got)
		}
		openIdx := strings.Index(got, open)
		if openIdx == -1 || !strings.Contains(got[openIdx:], "\x1b]8;;\x1b\\") {
			t.Errorf("open marker for %q has no matching close after it; got:\n%s", url, got)
		}
	}
	if !strings.Contains(got, "개시된 매장이 존재하지 않습니다") {
		t.Errorf("Korean text got corrupted; got:\n%s", got)
	}
}

func TestHyperlinkURLs_URLThatFitsOnOneLineStillGetsWrapped(t *testing.T) {
	original := `"url": "https://short.example.com/x"`
	wrapped := ansi.Hardwrap(original, 200, false) // wide enough: no actual wrapping occurs

	got := hyperlinkURLs(original, wrapped)

	want := "\x1b]8;;https://short.example.com/x\x1b\\https://short.example.com/x\x1b]8;;\x1b\\"
	if !strings.Contains(got, want) {
		t.Errorf("got:\n%s\nwant substring:\n%s", got, want)
	}
}
