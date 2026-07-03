package env

import (
	"sort"

	"github.com/bahn/rest-tui/internal/httpfile"
)

// ResolveRequest substitutes {{var}} placeholders in req's URL, header
// values, and body using vars. It returns the resolved request plus the
// sorted, de-duplicated list of variable names that could not be resolved
// anywhere in the request.
func ResolveRequest(req httpfile.Request, vars map[string]string) (httpfile.Request, []string) {
	missingSet := map[string]bool{}
	merge := func(text string) string {
		result, missing := Substitute(text, vars)
		for _, m := range missing {
			missingSet[m] = true
		}
		return result
	}

	resolved := req
	resolved.URL = merge(req.URL)
	if len(req.Headers) > 0 {
		resolved.Headers = make([]httpfile.Header, len(req.Headers))
		for i, h := range req.Headers {
			resolved.Headers[i] = httpfile.Header{Name: h.Name, Value: merge(h.Value)}
		}
	}
	resolved.Body = merge(req.Body)

	if len(missingSet) == 0 {
		return resolved, nil
	}
	missing := make([]string, 0, len(missingSet))
	for name := range missingSet {
		missing = append(missing, name)
	}
	sort.Strings(missing)
	return resolved, missing
}
