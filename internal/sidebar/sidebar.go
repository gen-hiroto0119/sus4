// Package sidebar manages the left pane: a file tree (Mode == ModeTree)
// or a list of working-tree changes (Mode == ModeChanges).
//
// The sidebar is intentionally output-only with respect to file content:
// it never reads files itself. When the user presses Enter, it returns
// an OpenIntent describing what the main view should display.
package sidebar

import (
	"sort"

	"github.com/gen-hiroto0119/tetra/internal/filetree"
	"github.com/gen-hiroto0119/tetra/internal/git"
)

type Mode int

const (
	ModeTree Mode = iota
	ModeChanges
)

// OpenKind classifies what the user requested when pressing Enter.
type OpenKind int

const (
	OpenNone OpenKind = iota
	OpenFile
	OpenDiffFile     // diff for a single working-tree path
	OpenDiffWorking  // entire working-tree diff (no item selected)
)

type OpenIntent struct {
	Kind OpenKind
	// Path is absolute for OpenFile, repo-relative for OpenDiffFile.
	Path string
}

// row is the unit the sidebar renders. A row may be a tree node or a
// status entry, depending on Mode. Indices into Model.rows are also
// indices the cursor moves through.
type row struct {
	tree   *treeRow   // set in ModeTree
	change *changeRow // set in ModeChanges
}

type treeRow struct {
	node  filetree.Node
	depth int
}

type changeRow struct {
	entry git.StatusEntry
	// aggregate marks the synthetic "all changes" overview row that
	// sits above the per-file list. Selecting it opens the entire
	// working-tree diff (OpenDiffWorking) instead of a single file.
	aggregate bool
}

type Model struct {
	mode      Mode
	rootDir   string
	repoOK    bool
	iconsOn   bool

	// Tree state. expanded[path] indicates whether to show children.
	// children[path] is the cached child list for an expanded directory.
	expanded   map[string]bool
	children   map[string][]filetree.Node
	rootChildren []filetree.Node // nil until the initial Read completes
	treeCursor int

	// Changes state.
	changes       []git.StatusEntry
	changesCursor int

	// Cached flattened view, recomputed on state changes.
	rows []row

	// Layout.
	width  int
	height int
	err    error
}

func New(rootDir string, iconsEnabled bool) Model {
	return Model{
		rootDir:  rootDir,
		expanded: make(map[string]bool),
		children: make(map[string][]filetree.Node),
		mode:     ModeTree,
		iconsOn:  iconsEnabled,
	}
}

// SetSize reports terminal dimensions for the sidebar pane.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// SetRepoAvailable toggles whether ModeChanges is functional.
func (m *Model) SetRepoAvailable(ok bool) { m.repoOK = ok }

// IsExpanded reports whether dir is currently expanded in the tree
// view. Used by app to decide whether an fs event under dir warrants
// re-listing its children.
func (m *Model) IsExpanded(dir string) bool { return m.expanded[dir] }

// RootDir returns the directory the sidebar was created with.
func (m *Model) RootDir() string { return m.rootDir }

// Mode returns the active sidebar mode.
func (m *Model) Mode() Mode { return m.mode }

// CycleMode advances to the next mode (← / → from Concept.md keymap).
func (m *Model) CycleMode(delta int) {
	const n = 2 // tree, changes
	v := (int(m.mode) + delta) % n
	if v < 0 {
		v += n
	}
	m.mode = Mode(v)
	m.rebuildRows()
}

// SetRootChildren stores the result of filetree.Read(rootDir).
func (m *Model) SetRootChildren(nodes []filetree.Node) {
	m.rootChildren = nodes
	m.children[m.rootDir] = nodes
	m.expanded[m.rootDir] = true
	if m.treeCursor >= len(nodes) {
		m.treeCursor = 0
	}
	m.rebuildRows()
}

// SetExpandedChildren caches children for a directory the user expanded.
func (m *Model) SetExpandedChildren(dir string, nodes []filetree.Node) {
	m.children[dir] = nodes
	m.expanded[dir] = true
	m.rebuildRows()
}

// SetChanges stores the result of git status.
func (m *Model) SetChanges(entries []git.StatusEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].Path < entries[j].Path
	})
	m.changes = entries
	if m.changesCursor >= len(entries) {
		m.changesCursor = 0
	}
	m.rebuildRows()
}

// SetError shows a one-line error banner inside the pane.
func (m *Model) SetError(err error) { m.err = err }

// MoveCursor moves the active cursor by delta and clamps. Returns true
// if anything changed.
func (m *Model) MoveCursor(delta int) bool {
	switch m.mode {
	case ModeTree:
		if len(m.rows) == 0 {
			return false
		}
		next := clamp(m.treeCursor+delta, 0, len(m.rows)-1)
		if next == m.treeCursor {
			return false
		}
		m.treeCursor = next
	case ModeChanges:
		if len(m.rows) == 0 {
			return false
		}
		next := clamp(m.changesCursor+delta, 0, len(m.rows)-1)
		if next == m.changesCursor {
			return false
		}
		m.changesCursor = next
	}
	return true
}

// Activate handles Enter on the current row. It either toggles a
// directory (returning the directory path to expand if not yet cached)
// or returns an OpenIntent.
//
// expandDir is the dir whose children must be loaded before the next
// rebuild — empty when no load is needed.
func (m *Model) Activate() (intent OpenIntent, expandDir string) {
	switch m.mode {
	case ModeTree:
		if m.treeCursor < 0 || m.treeCursor >= len(m.rows) {
			return OpenIntent{}, ""
		}
		r := m.rows[m.treeCursor]
		if r.tree == nil {
			return OpenIntent{}, ""
		}
		n := r.tree.node
		switch n.Kind {
		case filetree.NodeFile:
			return OpenIntent{Kind: OpenFile, Path: n.Path}, ""
		case filetree.NodeDir:
			if m.expanded[n.Path] {
				delete(m.expanded, n.Path)
				m.rebuildRows()
				return OpenIntent{}, ""
			}
			if _, cached := m.children[n.Path]; cached {
				m.expanded[n.Path] = true
				m.rebuildRows()
				return OpenIntent{}, ""
			}
			return OpenIntent{}, n.Path
		}
	case ModeChanges:
		if m.changesCursor < 0 || m.changesCursor >= len(m.rows) {
			return OpenIntent{Kind: OpenDiffWorking}, ""
		}
		r := m.rows[m.changesCursor]
		if r.change == nil {
			return OpenIntent{}, ""
		}
		if r.change.aggregate {
			return OpenIntent{Kind: OpenDiffWorking}, ""
		}
		return OpenIntent{Kind: OpenDiffFile, Path: r.change.entry.Path}, ""
	}
	return OpenIntent{}, ""
}

// CursorIndex returns the active cursor for the current mode.
func (m *Model) CursorIndex() int {
	if m.mode == ModeChanges {
		return m.changesCursor
	}
	return m.treeCursor
}

func (m *Model) rebuildRows() {
	switch m.mode {
	case ModeTree:
		m.rows = m.flattenTree()
		if m.treeCursor >= len(m.rows) {
			m.treeCursor = max(0, len(m.rows)-1)
		}
	case ModeChanges:
		rows := make([]row, 0, len(m.changes)+1)
		// Overview row first when there's anything to diff.
		if len(m.changes) > 0 {
			rows = append(rows, row{change: &changeRow{aggregate: true}})
		}
		for i := range m.changes {
			rows = append(rows, row{change: &changeRow{entry: m.changes[i]}})
		}
		m.rows = rows
		if m.changesCursor >= len(m.rows) {
			m.changesCursor = max(0, len(m.rows)-1)
		}
	}
}

// flattenTree walks the cached children and emits a depth-tagged list.
// Only directories present in m.expanded are descended into.
func (m *Model) flattenTree() []row {
	var out []row
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		nodes := m.children[parent]
		for _, n := range nodes {
			out = append(out, row{tree: &treeRow{node: n, depth: depth}})
			if n.Kind == filetree.NodeDir && m.expanded[n.Path] {
				walk(n.Path, depth+1)
			}
		}
	}
	walk(m.rootDir, 0)
	return out
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
