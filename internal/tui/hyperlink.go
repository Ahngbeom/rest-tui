package tui

import (
	"regexp"
	"strings"
)

var (
	ansiCSIPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")
	urlPattern     = regexp.MustCompile(`https?://[^"\s]+`)
)

// hyperlinkURLs finds URL substrings in original (pre-wrap ANSI text, one
// logical line per header/JSON value -- so a bare, unquoted URL is still
// correctly bounded by the real newline that ends its line) and wraps their
// corresponding span in wrapped (the same text after ansi.Hardwrap, where a
// match may now be split across several physical lines by inserted '\n')
// in an OSC 8 hyperlink escape, so the whole thing opens as one link in the
// terminal regardless of which wrapped row is clicked.
//
// This only works because ansi.Hardwrap never drops, adds, or reorders a
// content byte -- it only ever inserts '\n' and leaves SGR codes exactly
// where they were -- so the sequence of bytes in original and wrapped is
// identical once ANSI escapes and newlines are excluded from both. That
// lets a byte position found in original (via the regexp) be translated
// into the corresponding position in wrapped.
func hyperlinkURLs(original, wrapped string) string {
	plain := ansiCSIPattern.ReplaceAllString(original, "")
	matches := urlPattern.FindAllStringIndex(plain, -1)
	if len(matches) == 0 {
		return wrapped
	}

	// vIndex counts non-newline bytes before bytePos in a string that has
	// already had ANSI escapes stripped.
	vIndex := func(s string, bytePos int) int {
		return bytePos - strings.Count(s[:bytePos], "\n")
	}

	// rawStarts[v]/rawEnds[v] are the byte span in wrapped of the v-th byte
	// that is neither part of an ANSI CSI sequence nor a newline. This
	// counts one entry per *byte*, not per rune -- vIndex above (and the
	// regexp match offsets it's fed) also count in bytes, since Go's
	// regexp package reports byte offsets, so both sides of the mapping
	// must use the same unit. A multi-byte rune (e.g. the "─" divider, or
	// Korean text elsewhere in the body) never actually gets looked up
	// here since URL matches are pure ASCII, so counting it as several
	// byte-positions instead of one rune-position is harmless -- it only
	// needs to keep the running count in sync with vIndex.
	//
	// Tracking each byte's own end (rather than jumping to the next
	// entry's start) matters when a match ends right before skipped
	// content -- the next content byte after, say, a run of newlines and
	// a re-emitted color code could be arbitrarily far away, which would
	// place the closing marker way past where the match actually ends.
	var rawStarts, rawEnds []int
	for i := 0; i < len(wrapped); {
		if loc := ansiCSIPattern.FindStringIndex(wrapped[i:]); loc != nil && loc[0] == 0 {
			i += loc[1]
			continue
		}
		if wrapped[i] == '\n' {
			i++
			continue
		}
		rawStarts = append(rawStarts, i)
		rawEnds = append(rawEnds, i+1)
		i++
	}

	var b strings.Builder
	cursor := 0
	for _, m := range matches {
		vStart, vEnd := vIndex(plain, m[0]), vIndex(plain, m[1])
		if vStart >= len(rawStarts) || vEnd < 1 || vEnd-1 >= len(rawEnds) || vStart >= vEnd {
			continue
		}
		rawStart, rawEnd := rawStarts[vStart], rawEnds[vEnd-1]
		if rawStart < cursor || rawEnd < rawStart {
			continue
		}
		b.WriteString(wrapped[cursor:rawStart])
		b.WriteString("\x1b]8;;" + plain[m[0]:m[1]] + "\x1b\\")
		b.WriteString(wrapped[rawStart:rawEnd])
		b.WriteString("\x1b]8;;\x1b\\")
		cursor = rawEnd
	}
	b.WriteString(wrapped[cursor:])
	return b.String()
}
