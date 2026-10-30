package httpfile

import (
	"regexp"
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
	var pre []string
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
		if strings.HasPrefix(trimmed, "< {%") {
			pre = addUnique(pre, "pre-request script")
			for !strings.Contains(trimmed, "%}") && i+1 < len(block) {
				i++
				trimmed = strings.TrimSpace(block[i])
			}
			continue
		}
		if strings.HasPrefix(trimmed, "< ") || strings.HasPrefix(trimmed, "<\t") {
			pre = addUnique(pre, "pre-request script: "+strings.TrimSpace(trimmed[1:]))
			continue
		}

		method, url, ok := parseRequestLine(trimmed)
		if !ok {
			return &ParseError{Line: lineOffset + i + 1, Msg: "expected HTTP method and URL, got " + quote(trimmed)}
		}
		req = &Request{Name: name, Method: method, URL: url, Line: lineOffset + i + 1, Unsupported: pre}
		i++
		break
	}

	if req == nil {
		// Block had no method/URL line at all (e.g. only comments/blank
		// lines, or empty trailing block) -- nothing to add.
		return nil
	}

	// Indented "?a=b" / "&c=d" continuation lines extend the URL.
	for ; i < len(block); i++ {
		raw := block[i]
		if raw == "" || (raw[0] != ' ' && raw[0] != '\t') {
			break
		}
		trimmed := strings.TrimSpace(raw)
		if !strings.HasPrefix(trimmed, "?") && !strings.HasPrefix(trimmed, "&") {
			break
		}
		req.URL += trimmed
	}

	// Headers: consume "Name: value" lines until a blank line or EOF.
	for ; i < len(block); i++ {
		if strings.HasPrefix(block[i], ">") {
			break
		}
		trimmed := strings.TrimSpace(block[i])
		if trimmed == "" {
			i++
			break
		}
		if _, ok := stripCommentPrefix(trimmed); ok {
			continue
		}
		name, value, ok := parseHeaderLine(trimmed)
		if !ok {
			return &ParseError{Line: lineOffset + i + 1, Msg: "expected header \"Name: value\", got " + quote(trimmed)}
		}
		req.Headers = append(req.Headers, Header{Name: name, Value: value})
	}

	// Body: remainder of the block, trimmed of surrounding blank lines.
	if i < len(block) {
		body, unsupported := splitResponseHandlers(block[i:])
		req.Body = strings.TrimRight(strings.Join(body, "\n"), "\n \t")
		for _, u := range unsupported {
			req.Unsupported = addUnique(req.Unsupported, u)
		}
	}

	f.Requests = append(f.Requests, *req)
	return nil
}

// splitResponseHandlers separates the request body from the trailing response
// handler / output redirect section, which starts at the first line whose
// column 0 is ">". Handler scripts are never run; they are only reported.
func splitResponseHandlers(lines []string) (body []string, unsupported []string) {
	start := len(lines)
	for idx, l := range lines {
		if strings.HasPrefix(l, ">") {
			start = idx
			break
		}
	}
	body = lines[:start]
	for j := start; j < len(lines); j++ {
		l := strings.TrimSpace(lines[j])
		switch {
		case strings.HasPrefix(l, ">>!"):
			unsupported = addUnique(unsupported, "output redirect: "+strings.TrimSpace(l[3:]))
		case strings.HasPrefix(l, ">>"):
			unsupported = addUnique(unsupported, "output redirect: "+strings.TrimSpace(l[2:]))
		case strings.HasPrefix(l, "> {%"):
			unsupported = addUnique(unsupported, "response handler script")
			for !strings.Contains(l, "%}") && j+1 < len(lines) {
				j++
				l = lines[j]
			}
		case strings.HasPrefix(l, ">"):
			unsupported = addUnique(unsupported, "response handler script: "+strings.TrimSpace(l[1:]))
		}
	}
	return body, unsupported
}

func addUnique(list []string, s string) []string {
	for _, v := range list {
		if v == s {
			return list
		}
	}
	return append(list, s)
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

// parseRequestLine parses "[METHOD] URL [HTTP-VERSION]". The method may be
// omitted, in which case it defaults to GET.
func parseRequestLine(line string) (method, url string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		if m := strings.ToUpper(fields[0]); knownMethods[m] {
			return m, fields[1], true
		}
	}
	if len(fields) == 1 || (len(fields) == 2 && strings.HasPrefix(strings.ToUpper(fields[1]), "HTTP/")) {
		if looksLikeURL(fields[0]) {
			return "GET", fields[0], true
		}
	}
	return "", "", false
}

var (
	schemeRe   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)
	hostPathRe = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*(:[0-9]+)?([/?]\S*)?$`)
)

// looksLikeURL reports whether s is plausibly a request target rather than a
// stray word or a "Name:value" header.
func looksLikeURL(s string) bool {
	if schemeRe.MatchString(s) || strings.HasPrefix(s, "{{") {
		return true
	}
	return hostPathRe.MatchString(s) && strings.ContainsAny(s, ".:/")
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
