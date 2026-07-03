// Package httpfile parses IntelliJ HTTP Client (.http) scratch files.
package httpfile

import "strconv"

// Header is a single HTTP header line, order-preserved (duplicate names allowed).
type Header struct {
	Name  string
	Value string
}

// Request is one ### block parsed out of a .http file.
type Request struct {
	Name    string
	Method  string
	URL     string
	Headers []Header
	Body    string
	// Line is the 1-indexed source line where this request's method/URL line starts.
	Line int
}

// File is the parsed contents of a .http file.
type File struct {
	// Vars holds file-scoped variables declared via bare `@name = value` lines.
	Vars     map[string]string
	Requests []Request
}

// ParseError reports a malformed .http file with the source line it occurred on.
type ParseError struct {
	Line int
	Msg  string
}

func (e *ParseError) Error() string {
	return "line " + strconv.Itoa(e.Line) + ": " + e.Msg
}
