package env

import (
	"regexp"
	"sort"
)

var placeholderRE = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_.-]*)\s*\}\}`)

// Substitute replaces every {{name}} placeholder in text with vars[name].
// Placeholders with no matching entry in vars are left untouched in the
// result, and their names are returned (sorted, de-duplicated) in missing so
// the caller can fail fast instead of sending a partially-substituted request.
func Substitute(text string, vars map[string]string) (result string, missing []string) {
	missingSet := map[string]bool{}

	result = placeholderRE.ReplaceAllStringFunc(text, func(match string) string {
		name := placeholderRE.FindStringSubmatch(match)[1]
		if v, ok := vars[name]; ok {
			return v
		}
		missingSet[name] = true
		return match
	})

	if len(missingSet) == 0 {
		return result, nil
	}
	missing = make([]string, 0, len(missingSet))
	for name := range missingSet {
		missing = append(missing, name)
	}
	sort.Strings(missing)
	return result, missing
}
