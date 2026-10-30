package env

import (
	"math/rand/v2"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Ahngbeom/rest-tui/internal/httpfile"
)

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
	resolved.URL = merge(req.URL)
	if len(req.Headers) > 0 {
		resolved.Headers = make([]httpfile.Header, len(req.Headers))
		for i, h := range req.Headers {
			resolved.Headers[i] = httpfile.Header{Name: h.Name, Value: merge(h.Value)}
		}
	}
	resolved.Body = merge(req.Body)
	resolved.Unsupported = slices.Clone(req.Unsupported)

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
