package app

import (
	"time"

	"github.com/gen-hiroto0119/tetra/internal/config"
	"github.com/gen-hiroto0119/tetra/internal/git"
	"github.com/gen-hiroto0119/tetra/internal/highlight"
	"github.com/gen-hiroto0119/tetra/internal/mainview"
	"github.com/gen-hiroto0119/tetra/internal/sidebar"
	"github.com/gen-hiroto0119/tetra/internal/theme"
	"github.com/gen-hiroto0119/tetra/internal/watcher"
)

// Focus identifies which pane currently receives navigation keys.
type Focus int

const (
	FocusSidebar Focus = iota
	FocusMain
)

// DiffSource identifies what flavour of diff is currently shown so the
// fs-event reloader knows which Cmd to fire to refresh it.
type DiffSource int

const (
	DiffSourceNone DiffSource = iota
	DiffSourceFile
	DiffSourceWorking
)

// StartupKind tells Init() what initial Cmd batch to fire.
type StartupKind int

const (
	StartupDir StartupKind = iota
	StartupFile
	StartupCommit
)

type StartupTarget struct {
	Kind StartupKind
	// Arg is the raw CLI argument; it is interpreted lazily so v0.1 can
	// fall back to StartupDir without the parser knowing the difference.
	Arg string
}

type Options struct {
	RootDir string
	Target  StartupTarget
	Config  config.Config
}

// Model is the root Bubble Tea model. It aggregates child component
// state but never mutates it directly outside Update — see Design.md §3.
type Model struct {
	opts     Options
	theme    theme.Theme
	focus    Focus
	width    int
	height   int
	helpOpen bool

	sidebar  sidebar.Model
	main     mainview.Model

	repo      *git.Repo
	watcher   *watcher.Watcher
	trueColor bool
	err       error

	// activeFile tracks the currently open file path (absolute) so the
	// watcher's coalesced fs events can decide whether to reload.
	activeFile string

	// activeDiffKind / activeDiffPath remember which flavour of diff is
	// on screen so fs events can re-fire the matching load Cmd.
	activeDiffKind DiffSource
	activeDiffPath string // repo-relative; empty for working-tree diff

	// lastStatusReq / lastDiffReq / lastFileReloadReq throttle git
	// status, git diff, and the active-file body+markers reload chain
	// (Design.md §14: at most one per 200ms) so a fs-event burst can't
	// fork-bomb the git process pool. Without the file-reload throttle,
	// a build watcher rewriting the open file at 10–20 Hz had been
	// driving tetra to 200%+ CPU.
	lastStatusReq     time.Time
	lastDiffReq       time.Time
	lastFileReloadReq time.Time
}

func New(opts Options) Model {
	trueColor := highlight.TerminalSupportsTrueColor()
	if opts.Config.TrueColor != nil {
		trueColor = *opts.Config.TrueColor
	}
	return Model{
		opts:      opts,
		theme:     theme.ByName(opts.Config.Theme),
		focus:     FocusSidebar,
		sidebar:   sidebar.New(opts.RootDir, opts.Config.Icons),
		main:      mainview.New(trueColor),
		trueColor: trueColor,
	}
}
