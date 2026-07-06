package httpfile

import (
	"strings"
)

var knownMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true,
	"DELETE": true, "HEAD": true, "OPTIONS": true, "TRACE": true, "CONNECT": true,
}

// Parse reads an IntelliJ HTTP Client (.http) scratch file and returns its
// file-scoped variables and request blocks. Blocks are separated by lines
// starting with "###"; a file with no such line is treated as a single block.
// A block that fails to parse is skipped rather than aborting the whole
// file: its error is recorded in File.ParseErrors, and every other block
// still parses normally. The first recorded error (if any) is also
// returned as err for callers that only care whether something went wrong.
func Parse(data []byte) (*File, error) {
	lines := strings.Split(string(data), "\n")

	f := &File{Vars: map[string]string{}}

	// blockStart[i] is the first line index (0-based) of block i; blocks are
	// split on lines beginning with "###". Everything before the first "###"
	// (or the whole file, if there is none) is block 0.
	var blockStarts []int
	if len(lines) > 0 && !strings.HasPrefix(strings.TrimSpace(lines[0]), "###") {
		blockStarts = append(blockStarts, 0)
	}
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "###") {
			blockStarts = append(blockStarts, i)
		}
	}

	for bi, start := range blockStarts {
		end := len(lines)
		if bi+1 < len(blockStarts) {
			end = blockStarts[bi+1]
		}
		block := lines[start:end]
		delimiterName := ""
		bodyOffset := start
		if strings.HasPrefix(strings.TrimSpace(block[0]), "###") {
			delimiterName = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(block[0]), "###"))
			block = block[1:]
			bodyOffset++
		}

		if err := parseBlock(f, block, bodyOffset, delimiterName); err != nil {
			f.ParseErrors = append(f.ParseErrors, err)
		}
	}

	var firstErr error
	if len(f.ParseErrors) > 0 {
		firstErr = f.ParseErrors[0]
	}
	return f, firstErr
}

// parseBlock parses one ###-delimited section (with the "###" line itself
// already stripped) and, if it contains a request, appends it to f.Requests.
// lineOffset is the 0-based source line number of block[0]. On failure it
// returns the error without touching f.Requests for this block; the caller
// is expected to record it and move on to the next block.
func parseBlock(f *File, block []string, lineOffset int, delimiterName string) *ParseError {
	name := delimiterName
	var req *Request
	i := 0

	// Skip/consume leading comments, blank lines, and file-scoped @var
	// declarations until we find the method/URL line.
	for ; i < len(block); i++ {
		line := block[i]
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}
		if directive, ok := stripCommentPrefix(trimmed); ok {
			if n, ok := parseNameDirective(directive); ok {
				name = n
			}
			continue
		}
		if k, v, ok := parseVarDecl(trimmed); ok {
			f.Vars[k] = v
			continue
		}

		method, url, ok := parseRequestLine(trimmed)
		if !ok {
			return &ParseError{Line: lineOffset + i + 1, Msg: "expected HTTP method and URL, got " + quote(trimmed)}
		}
		req = &Request{Name: name, Method: method, URL: url, Line: lineOffset + i + 1}
		i++
		break
	}

	if req == nil {
		// Block had no method/URL line at all (e.g. only comments/blank
		// lines, or empty trailing block) -- nothing to add.
		return nil
	}

	// Headers: consume "Name: value" lines until a blank line or EOF.
	for ; i < len(block); i++ {
		trimmed := strings.TrimSpace(block[i])
		if trimmed == "" {
			i++
			break
		}
		name, value, ok := parseHeaderLine(trimmed)
		if !ok {
			return &ParseError{Line: lineOffset + i + 1, Msg: "expected header \"Name: value\", got " + quote(trimmed)}
		}
		req.Headers = append(req.Headers, Header{Name: name, Value: value})
	}

	// Body: remainder of the block, trimmed of surrounding blank lines.
	if i < len(block) {
		req.Body = strings.TrimRight(strings.Join(block[i:], "\n"), "\n \t")
	}

	f.Requests = append(f.Requests, *req)
	return nil
}

// stripCommentPrefix reports whether line is a "#" or "//" comment, returning
// the text after the marker.
func stripCommentPrefix(line string) (rest string, ok bool) {
	if strings.HasPrefix(line, "#") {
		return strings.TrimSpace(strings.TrimPrefix(line, "#")), true
	}
	if strings.HasPrefix(line, "//") {
		return strings.TrimSpace(strings.TrimPrefix(line, "//")), true
	}
	return "", false
}

// parseNameDirective parses a comment directive body of the form "@name value".
func parseNameDirective(directive string) (name string, ok bool) {
	if !strings.HasPrefix(directive, "@name") {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(directive, "@name"))
	if rest == "" {
		return "", false
	}
	return rest, true
}

// parseVarDecl parses a bare file-scoped variable declaration: "@name = value".
func parseVarDecl(line string) (name, value string, ok bool) {
	if !strings.HasPrefix(line, "@") {
		return "", "", false
	}
	eq := strings.Index(line, "=")
	if eq < 0 {
		return "", "", false
	}
	name = strings.TrimSpace(line[1:eq])
	value = strings.TrimSpace(line[eq+1:])
	if name == "" {
		return "", "", false
	}
	return name, value, true
}

// parseRequestLine parses "METHOD URL [HTTP-VERSION]".
func parseRequestLine(line string) (method, url string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", "", false
	}
	method = strings.ToUpper(fields[0])
	if !knownMethods[method] {
		return "", "", false
	}
	url = fields[1]
	return method, url, true
}

// parseHeaderLine parses "Name: value".
func parseHeaderLine(line string) (name, value string, ok bool) {
	colon := strings.Index(line, ":")
	if colon <= 0 {
		return "", "", false
	}
	name = strings.TrimSpace(line[:colon])
	if strings.ContainsAny(name, " \t") {
		return "", "", false
	}
	value = strings.TrimSpace(line[colon+1:])
	return name, value, true
}

func quote(s string) string {
	return "\"" + s + "\""
}
