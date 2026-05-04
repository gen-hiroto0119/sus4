package app

import (
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gen-hiroto0119/tetra/internal/keymap"
	"github.com/gen-hiroto0119/tetra/internal/mainview"
	"github.com/gen-hiroto0119/tetra/internal/sidebar"
)

// statusThrottle caps git status invocations to once per this window.
// A burst of fs events on a busy repo otherwise spawns one `git status`
// subprocess per debounced event — that drives the CPU through the roof
// (Design.md §14).
const statusThrottle = 200 * time.Millisecond

func (m Model) Init() tea.Cmd {
	// tea.ClearScreen forces a CSI 2J right after the alt-screen switch so
	// the first paint can't overlap residue from the user's prior shell
	// session — some terminals/tmux configs leak the primary buffer
	// through. Without it the top rows can read like a half-cut UI.
	return tea.Batch(
		tea.ClearScreen,
		readDirCmd(m.opts.RootDir),
		openRepoCmd(m.opts.RootDir),
		startWatcherCmd(m.opts.RootDir),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Reclamp the main-view scroll: a previous offset can point past
		// the end of the new (potentially smaller) viewport, leaving the
		// pane blank or rendering stale tail content.
		m.main.ClampScroll()
		// Wipe leftovers from the pre-resize layout — Bubble Tea's
		// incremental renderer otherwise leaves stale cells where panes
		// shrank.
		return m, tea.ClearScreen

	case tea.KeyMsg:
		return m.handleKey(msg)

	case repoOpenedMsg:
		m.repo = msg.repo
		m.sidebar.SetRepoAvailable(msg.repo != nil)
		if msg.repo != nil {
			cmds := []tea.Cmd{gitStatusCmd(msg.repo)}
			// If the user opened a file before the repo handle resolved
			// (the two Cmds in Init race), the marker column will be
			// stuck blank. Now that we have the repo, fire markers for
			// the active file so the gutter catches up.
			if m.main.Kind() == mainview.KindFile && m.activeFile != "" {
				cmds = append(cmds, loadFileMarkersCmd(msg.repo, m.activeFile))
			}
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case treeBuiltMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.sidebar.SetRootChildren(msg.nodes)
		return m, nil

	case childrenLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.sidebar.SetExpandedChildren(msg.dir, msg.nodes)
		return m, nil

	case gitStatusMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.sidebar.SetChanges(msg.entries)
		return m, nil

	case fileLoadedMsg:
		if msg.err != nil {
			m.main.SetError(msg.err)
			return m, nil
		}
		m.main.ShowFile(msg.path, msg.text, msg.banner)
		m.activeFile = msg.path
		return m, nil

	case fileMarkersLoadedMsg:
		// Drop a markers result whose path no longer matches the
		// active file — happens when the user navigated away while the
		// git diff Cmd was still in flight. The activeFile field is
		// stamped synchronously in dispatchOpenIntent so this check is
		// reliable even when the markers Cmd resolves before the
		// matching loadFileCmd.
		if msg.path != m.activeFile {
			return m, nil
		}
		if msg.cleared {
			m.main.ClearMarkers(msg.path)
		} else {
			m.main.SetMarkers(msg.path, msg.markers)
		}
		return m, nil

	case diffLoadedMsg:
		if msg.err != nil {
			m.main.SetError(msg.err)
			return m, nil
		}
		m.main.ShowDiff(msg.title, msg.lines)
		return m, nil

	case watcherStartedMsg:
		if msg.err != nil {
			// Non-fatal: the app remains useful without auto-follow.
			m.err = msg.err
			return m, nil
		}
		m.watcher = msg.w
		return m, pumpWatcherCmd(msg.w)

	case fsEventMsg:
		// Re-arm the pump first so we never miss the next event.
		cmds := []tea.Cmd{pumpWatcherCmd(m.watcher)}
		// Only reload the file when the user is actually looking at it —
		// otherwise a background fs event would yank a diff view back to
		// the file view.
		if m.main.Kind() == mainview.KindFile &&
			m.activeFile != "" &&
			msg.ev.Path == m.activeFile {
			cmds = append(cmds,
				loadFileCmd(m.activeFile, m.trueColor, m.theme.IsDark),
				loadFileMarkersCmd(m.repo, m.activeFile),
			)
		}
		// Re-list the parent directory when something appeared / went
		// away there, so the sidebar tree picks up files Claude Code
		// (or any tool) creates without the user re-expanding.
		if msg.ev.IsStructural() {
			parent := filepath.Dir(msg.ev.Path)
			if parent == m.opts.RootDir {
				cmds = append(cmds, readDirCmd(parent))
			} else if m.sidebar.IsExpanded(parent) {
				cmds = append(cmds, loadChildrenCmd(parent))
			}
		}
		// Any fs change might affect the working tree status, but a
		// burst can fire dozens of events per second — throttle so we
		// don't fork `git status` on every one.
		if c := m.maybeStatusCmd(); c != nil {
			cmds = append(cmds, c)
		}
		// Same trick for an active diff view: re-fetch on fs activity
		// so the working-tree diff stays in step with the file pane.
		if c := m.maybeDiffReloadCmd(); c != nil {
			cmds = append(cmds, c)
		}
		return m, tea.Batch(cmds...)

	case gitMetaMsg:
		cmds := []tea.Cmd{pumpWatcherCmd(m.watcher)}
		if c := m.maybeStatusCmd(); c != nil {
			cmds = append(cmds, c)
		}
		if c := m.maybeDiffReloadCmd(); c != nil {
			cmds = append(cmds, c)
		}
		// HEAD/index churn changes the diff against HEAD, so the
		// marker column needs a refresh even when the file body itself
		// is byte-identical (e.g. after `git commit`).
		if m.main.Kind() == mainview.KindFile && m.activeFile != "" {
			cmds = append(cmds, loadFileMarkersCmd(m.repo, m.activeFile))
		}
		return m, tea.Batch(cmds...)
	}

	return m, nil
}

// maybeStatusCmd returns a gitStatusCmd if the last request is older
// than statusThrottle, nil otherwise. The Update method that owns m is
// a value receiver, so it returns a (potentially mutated) m alongside;
// here we stamp lastStatusReq via the same return path by mutating the
// receiver in the caller via assignment. Practically this is invoked on
// `m` in Update where the resulting m is returned to Bubble Tea.
func (m *Model) maybeStatusCmd() tea.Cmd {
	if m.repo == nil {
		return nil
	}
	now := time.Now()
	if now.Sub(m.lastStatusReq) < statusThrottle {
		return nil
	}
	m.lastStatusReq = now
	return gitStatusCmd(m.repo)
}

// maybeDiffReloadCmd re-fires the load Cmd that originally produced the
// active diff, throttled to statusThrottle. Returns nil when no diff is
// on screen or the throttle is still warm.
func (m *Model) maybeDiffReloadCmd() tea.Cmd {
	if m.repo == nil || m.activeDiffKind == DiffSourceNone {
		return nil
	}
	now := time.Now()
	if now.Sub(m.lastDiffReq) < statusThrottle {
		return nil
	}
	m.lastDiffReq = now
	switch m.activeDiffKind {
	case DiffSourceFile:
		return loadFileDiffCmd(m.repo, m.activeDiffPath)
	case DiffSourceWorking:
		return loadWorkingDiffCmd(m.repo)
	}
	return nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := keymap.Resolve(msg)

	// While help is showing, only quit and close-help keys do anything.
	// Everything else is swallowed so the user can't accidentally drive
	// the (now invisible) panes underneath.
	if m.helpOpen {
		switch action {
		case keymap.ActionQuit:
			return m, tea.Quit
		case keymap.ActionHelp:
			m.helpOpen = false
		}
		if msg.String() == "esc" {
			m.helpOpen = false
		}
		return m, nil
	}

	// Global keys win regardless of focus.
	switch action {
	case keymap.ActionQuit:
		return m, tea.Quit
	case keymap.ActionHelp:
		m.helpOpen = !m.helpOpen
		return m, nil
	case keymap.ActionFocusToggle:
		if m.focus == FocusSidebar {
			m.focus = FocusMain
		} else {
			m.focus = FocusSidebar
		}
		return m, nil
	}

	if m.focus == FocusSidebar {
		return m.handleSidebarKey(action)
	}
	return m.handleMainKey(action)
}

func (m Model) handleSidebarKey(action keymap.Action) (tea.Model, tea.Cmd) {
	switch action {
	case keymap.ActionUp:
		m.sidebar.MoveCursor(-1)
	case keymap.ActionDown:
		m.sidebar.MoveCursor(1)
	case keymap.ActionLeft:
		m.sidebar.CycleMode(-1)
	case keymap.ActionRight:
		m.sidebar.CycleMode(1)
	case keymap.ActionEnter:
		intent, expandDir := m.sidebar.Activate()
		if expandDir != "" {
			return m, loadChildrenCmd(expandDir)
		}
		return m, m.dispatchOpenIntent(intent)
	}
	return m, nil
}

func (m Model) handleMainKey(action keymap.Action) (tea.Model, tea.Cmd) {
	switch action {
	case keymap.ActionUp:
		m.main.Scroll(-1)
	case keymap.ActionDown:
		m.main.Scroll(1)
	case keymap.ActionPageUp:
		m.main.ScrollPage(-1)
	case keymap.ActionPageDown:
		m.main.ScrollPage(1)
	case keymap.ActionHome:
		m.main.ScrollHome()
	case keymap.ActionEnd:
		m.main.ScrollEnd()
	}
	return m, nil
}

// dispatchOpenIntent records what kind of view the new Cmd will install
// (so fs events can later refresh it) and returns the Cmd. Pointer
// receiver because we stamp activeDiff* on the model.
func (m *Model) dispatchOpenIntent(intent sidebar.OpenIntent) tea.Cmd {
	switch intent.Kind {
	case sidebar.OpenFile:
		m.activeDiffKind = DiffSourceNone
		m.activeDiffPath = ""
		// Stamp activeFile synchronously so a fast-resolving markers
		// Cmd (which may beat loadFileCmd to the inbox) can be matched
		// against the right path in fileMarkersLoadedMsg.
		m.activeFile = intent.Path
		return tea.Batch(
			loadFileCmd(intent.Path, m.trueColor, m.theme.IsDark),
			loadFileMarkersCmd(m.repo, intent.Path),
		)
	case sidebar.OpenDiffFile:
		m.activeDiffKind = DiffSourceFile
		m.activeDiffPath = intent.Path
		return loadFileDiffCmd(m.repo, intent.Path)
	case sidebar.OpenDiffWorking:
		m.activeDiffKind = DiffSourceWorking
		m.activeDiffPath = ""
		return loadWorkingDiffCmd(m.repo)
	}
	return nil
}
