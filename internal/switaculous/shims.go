package switaculous

import (
	"embed"
	"regexp"
	"sort"
)

// Framework shims: small Swift files standing in for Apple frameworks the
// preview cannot really run (AlarmKit, StoreKit, UserNotifications, ...).
// They define the types an app touches so its views still render; the real
// behaviour (ringing alarms, charging money) is a no-op in the preview.
//
// A shim is only loaded when a project file imports its framework.
//
//go:embed shims/*.swift
var shimFS embed.FS

var importRe = regexp.MustCompile(`(?m)^\s*(?:@[\w.]+\s+)*import\s+(?:(?:class|struct|enum|protocol|func|var|let|typealias)\s+)?([A-Za-z_][\w]*)`)

func importedFrameworks(sources map[string]string) []string {
	seen := map[string]bool{}
	for _, src := range sources {
		for _, m := range importRe.FindAllStringSubmatch(src, -1) {
			seen[m[1]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func shimSource(framework string) (string, bool) {
	b, err := shimFS.ReadFile("shims/" + framework + ".swift")
	if err != nil {
		return "", false
	}
	return string(b), true
}

// Shims are parsed under a bracketed pseudo file name so errors inside them
// are recognisable as framework stubs rather than project files.
func shimPath(framework string) string {
	return "<" + framework + ">.swift"
}
