package output

import (
	"bytes"
	"mime"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Sanitize removes terminal escape sequences and control characters (all C0
// and C1 controls except newline and tab) so server-controlled text cannot
// drive the user's terminal.
func Sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || (r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, ansi.Strip(s))
}

// IsBinary reports whether body should be summarized instead of displayed.
func IsBinary(body []byte, contentType string) bool {
	if len(body) == 0 {
		return false
	}
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mt = ""
	}
	mt = strings.ToLower(mt)
	if mt == "image/svg+xml" {
		return false
	}
	for _, prefix := range []string{"image/", "audio/", "video/", "font/"} {
		if strings.HasPrefix(mt, prefix) {
			return true
		}
	}
	switch mt {
	case "application/octet-stream", "application/pdf", "application/zip", "application/gzip":
		return true
	}
	hasNUL := bytes.IndexByte(body, 0) >= 0
	if strings.HasPrefix(mt, "text/") || strings.HasSuffix(mt, "json") || strings.HasSuffix(mt, "xml") {
		return hasNUL
	}
	return hasNUL || !utf8.Valid(body)
}
