package env

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Ahngbeom/rest-tui/internal/httpfile"
)

// MissingFilePrefix marks entries in ResolveRequest's missing list that are
// unreadable `< path` body includes rather than variable names.
const MissingFilePrefix = "missing file: "

var schemeRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)

// EnsureScheme prefixes http:// to a URL that has no scheme. URLs still
// containing a {{ placeholder are left alone.
func EnsureScheme(rawURL string) string {
	if rawURL == "" || schemeRE.MatchString(rawURL) || strings.Contains(rawURL, "{{") {
		return rawURL
	}
	return "http://" + rawURL
}

// ResolveOptions supplies the context for resolving a request beyond its
// variables. A zero Now means time.Now(); a nil Rand means a freshly seeded
// generator.
type ResolveOptions struct {
	BaseDir string
	Now     time.Time
	Rand    *rand.Rand
}

// ResolveRequest substitutes {{var}} placeholders in req's URL, header
// values, and body. Names starting with "$" are IntelliJ dynamic variables
// generated per occurrence (never looked up in vars); all others come from
// vars. It returns the resolved request plus the sorted, de-duplicated list
// of names that could not be resolved anywhere in the request.
func ResolveRequest(req httpfile.Request, vars map[string]string, opts ResolveOptions) (httpfile.Request, []string) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	r := opts.Rand
	if r == nil {
		r = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	lookup := func(name string) (string, bool) {
		if strings.HasPrefix(name, "$") {
			return dynamicValue(name, now, r)
		}
		v, ok := vars[name]
		return v, ok
	}

	missingSet := map[string]bool{}
	merge := func(text string) string {
		result, missing := substituteFunc(text, lookup)
		for _, m := range missing {
			missingSet[m] = true
		}
		return result
	}

	resolved := req
	resolved.URL = EnsureScheme(merge(req.URL))
	if len(req.Headers) > 0 {
		resolved.Headers = make([]httpfile.Header, len(req.Headers))
		for i, h := range req.Headers {
			resolved.Headers[i] = httpfile.Header{Name: h.Name, Value: merge(h.Value)}
		}
	}
	resolved.Body = merge(req.Body)
	var missingFiles []string
	if opts.BaseDir != "" {
		resolved.Body, missingFiles = includeFiles(resolved.Body, opts.BaseDir)
	}
	resolved.Unsupported = slices.Clone(req.Unsupported)

	if len(missingSet) == 0 && len(missingFiles) == 0 {
		return resolved, nil
	}
	missing := make([]string, 0, len(missingSet))
	for name := range missingSet {
		missing = append(missing, name)
	}
	sort.Strings(missing)
	return resolved, append(missing, missingFiles...)
}

// includeFiles replaces each `< path` line of body with the file's contents.
// A file followed by more body lines loses one trailing newline, since the
// line join already supplies it; the last line keeps the file's bytes.
// Unreadable files leave the line untouched and are reported as
// MissingFilePrefix+path, de-duplicated in body order.
func includeFiles(body, baseDir string) (string, []string) {
	lines := strings.Split(body, "\n")
	var missing []string
	seen := map[string]bool{}
	for i, line := range lines {
		path, ok := includePath(line)
		if !ok {
			continue
		}
		full := path
		if !filepath.IsAbs(path) {
			full = filepath.Join(baseDir, path)
		}
		data, err := os.ReadFile(full)
		if err != nil {
			if !seen[path] {
				seen[path] = true
				missing = append(missing, MissingFilePrefix+path)
			}
			continue
		}
		content := string(data)
		if i < len(lines)-1 {
			if strings.HasSuffix(content, "\r\n") {
				content = content[:len(content)-2]
			} else {
				content = strings.TrimSuffix(content, "\n")
			}
		}
		lines[i] = content
	}
	return strings.Join(lines, "\n"), missing
}

// IncludePaths returns the path of every `< path` line in body, in order.
func IncludePaths(body string) []string {
	var paths []string
	for _, line := range strings.Split(body, "\n") {
		if path, ok := includePath(line); ok {
			paths = append(paths, path)
		}
	}
	return paths
}

func includePath(line string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimLeft(line, " \t"), "<")
	if !ok || rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
		return "", false
	}
	path := strings.TrimSpace(rest)
	return path, path != ""
}
