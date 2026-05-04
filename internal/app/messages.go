package app

import (
	"github.com/gen-hiroto0119/tetra/internal/diffview"
	"github.com/gen-hiroto0119/tetra/internal/filetree"
	"github.com/gen-hiroto0119/tetra/internal/git"
	"github.com/gen-hiroto0119/tetra/internal/watcher"
)

// Messages produced by async Cmds and consumed by Update.
//
// Messages are kept small and many. Each carries only the data Update
// needs to decide the next state transition.

type repoOpenedMsg struct {
	repo *git.Repo
	err  error
}

type treeBuiltMsg struct {
	dir   string
	nodes []filetree.Node
	err   error
}

type childrenLoadedMsg struct {
	dir   string
	nodes []filetree.Node
	err   error
}

type gitStatusMsg struct {
	entries []git.StatusEntry
	err     error
}

type fileLoadedMsg struct {
	path    string
	text    string
	banner  string
	err     error
}

type diffLoadedMsg struct {
	title string
	lines []diffview.Line
	err   error
}

// fileMarkersLoadedMsg carries the per-line change classification for
// the (absolute) path it was computed for. The Update handler matches
// path against m.activeFile before forwarding to mainview so that a
// stale Cmd resolving after the user navigated away is dropped.
type fileMarkersLoadedMsg struct {
	path    string
	markers map[int]diffview.ChangeKind
	// cleared signals "file is outside any repo / untracked / errored":
	// the renderer should drop the marker column entirely instead of
	// drawing an empty one.
	cleared bool
}

type watcherStartedMsg struct {
	w   *watcher.Watcher
	err error
}

type fsEventMsg struct {
	ev watcher.Event
}

type gitMetaMsg struct {
	ev watcher.GitMetaEvent
}
