package switaculous

import (
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
)

// A place in the project where a name is declared, sent to the editor for "Go to Definition".
type declarationSite struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
	Kind string `json:"kind"`
}

var swiftIdentifierPattern = regexp.MustCompile(`^[A-Za-z_]\w*$`)

// Types come first, so double-clicking BasketSheet lands on the struct rather
// than on a variable or extension that happens to share the name.
var declarationKindRank = map[string]int{
	"struct": 0, "class": 0, "enum": 0, "actor": 0, "protocol": 0, "typealias": 0,
	"extension": 1, "func": 2, "case": 3, "var": 4, "let": 4,
}

func (rt *Runtime) handleDefinition(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if !swiftIdentifierPattern.MatchString(name) {
		writeJSON(w, map[string][]declarationSite{"sites": {}})
		return
	}
	rt.mu.Lock()
	files := map[string]string{}
	for absPath := range rt.files {
		files[absPath] = rt.rel(absPath)
	}
	rt.mu.Unlock()

	pattern := regexp.MustCompile(`\b(struct|class|enum|actor|protocol|typealias|extension|func|case|var|let)\s+` + regexp.QuoteMeta(name) + `\b`)
	sites := []declarationSite{}
	for absPath, relPath := range files {
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		for lineIndex, line := range strings.Split(string(content), "\n") {
			match := pattern.FindStringSubmatchIndex(line)
			if match == nil {
				continue
			}
			if comment := strings.Index(line, "//"); comment >= 0 && comment < match[0] {
				continue
			}
			sites = append(sites, declarationSite{
				Path: relPath,
				Line: lineIndex + 1,
				Col:  match[1] - len(name) + 1,
				Kind: line[match[2]:match[3]],
			})
		}
	}
	sort.SliceStable(sites, func(i, j int) bool {
		a, b := sites[i], sites[j]
		if declarationKindRank[a.Kind] != declarationKindRank[b.Kind] {
			return declarationKindRank[a.Kind] < declarationKindRank[b.Kind]
		}
		aOwnFile := strings.HasSuffix(a.Path, "/"+name+".swift") || a.Path == name+".swift"
		bOwnFile := strings.HasSuffix(b.Path, "/"+name+".swift") || b.Path == name+".swift"
		if aOwnFile != bOwnFile {
			return aOwnFile
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Line < b.Line
	})
	writeJSON(w, map[string][]declarationSite{"sites": sites})
}
