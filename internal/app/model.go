package app

import (
	"github.com/gen-hiroto0119/sus4/internal/config"
	"github.com/gen-hiroto0119/sus4/internal/git"
	"github.com/gen-hiroto0119/sus4/internal/highlight"
	"github.com/gen-hiroto0119/sus4/internal/mainview"
	"github.com/gen-hiroto0119/sus4/internal/sidebar"
	"github.com/gen-hiroto0119/sus4/internal/theme"
	"github.com/gen-hiroto0119/sus4/internal/watcher"
)

// Focus identifies which pane currently receives navigation keys.
type Focus int

const (
	FocusSidebar Focus = iota
	FocusMain
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
		main:      mainview.New(),
		trueColor: trueColor,
	}
}
