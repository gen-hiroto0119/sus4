package app

import (
	"github.com/gen-hiroto0119/sus4/internal/diffview"
	"github.com/gen-hiroto0119/sus4/internal/filetree"
	"github.com/gen-hiroto0119/sus4/internal/git"
	"github.com/gen-hiroto0119/sus4/internal/watcher"
)

// Messages produced by async Cmds and consumed by Update.
//
// Per Design.md §4, messages are kept small and many. Each carries
// only the data Update needs to decide the next state transition.

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
