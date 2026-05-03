package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/gen-hiroto0119/sus4/internal/keymap"
	"github.com/gen-hiroto0119/sus4/internal/mainview"
	"github.com/gen-hiroto0119/sus4/internal/sidebar"
)

func (m Model) Init() tea.Cmd {
	return tea.Batch(
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
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case repoOpenedMsg:
		m.repo = msg.repo
		m.sidebar.SetRepoAvailable(msg.repo != nil)
		if msg.repo != nil {
			return m, gitStatusCmd(msg.repo)
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
			cmds = append(cmds, loadFileCmd(m.activeFile, m.trueColor))
		}
		// Any fs change might affect the working tree status. Refreshing
		// is cheap and the watcher already coalesced bursts.
		if m.repo != nil {
			cmds = append(cmds, gitStatusCmd(m.repo))
		}
		return m, tea.Batch(cmds...)

	case gitMetaMsg:
		cmds := []tea.Cmd{pumpWatcherCmd(m.watcher)}
		if m.repo != nil {
			cmds = append(cmds, gitStatusCmd(m.repo))
		}
		return m, tea.Batch(cmds...)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := keymap.Resolve(msg)

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

func (m Model) dispatchOpenIntent(intent sidebar.OpenIntent) tea.Cmd {
	switch intent.Kind {
	case sidebar.OpenFile:
		return loadFileCmd(intent.Path, m.trueColor)
	case sidebar.OpenDiffFile:
		// intent.Path is repo-relative — that's what git wants.
		return loadFileDiffCmd(m.repo, intent.Path)
	case sidebar.OpenDiffWorking:
		return loadWorkingDiffCmd(m.repo)
	}
	return nil
}
