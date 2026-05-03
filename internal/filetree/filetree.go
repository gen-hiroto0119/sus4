// Package filetree builds and navigates an in-memory directory tree.
// It is UI-agnostic: nodes carry no styling, just structural data.
//
// Loading is lazy — Read(dir) returns only the immediate children of dir.
// Callers (typically the sidebar) decide when to expand a directory and
// invoke Read again on the chosen child. This matches Design.md §11
// ("構築は遅延展開：起動時にはルート直下のみ列挙。展開時に子をロード").
package filetree

import (
	"os"
	"path/filepath"
	"sort"
)

// Excluded directories for v0.1. .gitignore parsing is deferred to v0.3.
var defaultExcludes = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"vendor":       {},
}

// MaxEntriesPerDir caps how many entries we surface from a single directory.
// Anything beyond is folded into a single Truncated node so a 50k-entry
// dir cannot hang the UI. Threshold matches Design.md §11.
const MaxEntriesPerDir = 500

type NodeKind int

const (
	NodeFile NodeKind = iota
	NodeDir
	// NodeTruncated represents the "+ N more" placeholder when a directory
	// exceeds MaxEntriesPerDir. It is not selectable.
	NodeTruncated
)

type Node struct {
	Name string
	Path string // absolute path
	Kind NodeKind
	// Hidden is true for dotfiles. The UI may choose to dim them.
	Hidden bool
	// HiddenCount is only meaningful for NodeTruncated.
	HiddenCount int
}

// Read returns the immediate children of dir, sorted directories-first
// then files, both alphabetically (case-insensitive). Excluded entries
// are dropped silently.
func Read(dir string) ([]Node, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	nodes := make([]Node, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if isExcluded(name) {
			continue
		}
		kind := NodeFile
		if e.IsDir() {
			kind = NodeDir
		}
		nodes = append(nodes, Node{
			Name:   name,
			Path:   filepath.Join(dir, name),
			Kind:   kind,
			Hidden: len(name) > 0 && name[0] == '.',
		})
	}

	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Kind != nodes[j].Kind {
			return nodes[i].Kind == NodeDir
		}
		return foldLower(nodes[i].Name) < foldLower(nodes[j].Name)
	})

	if len(nodes) > MaxEntriesPerDir {
		hidden := len(nodes) - MaxEntriesPerDir
		nodes = nodes[:MaxEntriesPerDir]
		nodes = append(nodes, Node{
			Name:        "…",
			Path:        dir,
			Kind:        NodeTruncated,
			HiddenCount: hidden,
		})
	}

	return nodes, nil
}

// IsExcluded reports whether name is on the v0.1 hardcoded exclude list.
// Exposed so callers (e.g. the watcher) can apply the same filter.
func IsExcluded(name string) bool { return isExcluded(name) }

func isExcluded(name string) bool {
	_, ok := defaultExcludes[name]
	return ok
}

func foldLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
