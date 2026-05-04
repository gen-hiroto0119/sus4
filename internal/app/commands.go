package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gen-hiroto0119/tetra/internal/diffview"
	"github.com/gen-hiroto0119/tetra/internal/filetree"
	"github.com/gen-hiroto0119/tetra/internal/git"
	"github.com/gen-hiroto0119/tetra/internal/highlight"
	"github.com/gen-hiroto0119/tetra/internal/mainview"
	"github.com/gen-hiroto0119/tetra/internal/watcher"
)

// tabExpansion is what every TAB gets rewritten to before content
// reaches the highlighter or the diff parser. uniseg/ansi count a TAB
// as one cell, but terminals render it at 4–8 — that mismatch breaks
// our wrap math (lines silently overflow innerWidth, the terminal
// performs a visual wrap, and the pane height grows out of sync with
// the sibling). Spelling tabs as four spaces up front keeps counted
// width and rendered width in lock-step.
const tabExpansion = "    "

// gitCmdTimeout caps individual git invocations so a hung child can't
// freeze the UI thread (Cmds run in a goroutine but a blocked syscall
// would still pile up).
const gitCmdTimeout = 5 * time.Second

func openRepoCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), gitCmdTimeout)
		defer cancel()
		repo, err := git.Open(ctx, dir)
		if err == git.ErrNotARepo {
			return repoOpenedMsg{}
		}
		return repoOpenedMsg{repo: repo, err: err}
	}
}

func readDirCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		nodes, err := filetree.Read(dir)
		return treeBuiltMsg{dir: dir, nodes: nodes, err: err}
	}
}

func loadChildrenCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		nodes, err := filetree.Read(dir)
		return childrenLoadedMsg{dir: dir, nodes: nodes, err: err}
	}
}

func gitStatusCmd(repo *git.Repo) tea.Cmd {
	if repo == nil {
		return func() tea.Msg { return gitStatusMsg{} }
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), gitCmdTimeout)
		defer cancel()
		entries, err := repo.Status(ctx)
		return gitStatusMsg{entries: entries, err: err}
	}
}

func loadFileCmd(path string, trueColor, dark bool) tea.Cmd {
	return func() tea.Msg {
		content, err := os.ReadFile(path)
		if err != nil {
			return fileLoadedMsg{path: path, err: err}
		}
		content = bytes.ReplaceAll(content, []byte{'\t'}, []byte(tabExpansion))
		r := highlight.Highlight(path, content, trueColor, dark)
		banner := ""
		if r.Plain {
			banner = r.Reason
		}
		return fileLoadedMsg{path: path, text: r.Text, banner: banner}
	}
}

func loadFileDiffCmd(repo *git.Repo, repoRelPath string) tea.Cmd {
	if repo == nil {
		return func() tea.Msg { return diffLoadedMsg{err: fmt.Errorf("no git repository")} }
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), gitCmdTimeout)
		defer cancel()
		raw, err := repo.DiffFile(ctx, repoRelPath)
		if err != nil {
			return diffLoadedMsg{err: err}
		}
		raw = strings.ReplaceAll(raw, "\t", tabExpansion)
		return diffLoadedMsg{
			title: mainview.TitleFor("diff", repoRelPath),
			lines: diffview.Parse(raw),
		}
	}
}

func startWatcherCmd(rootDir string) tea.Cmd {
	return func() tea.Msg {
		w, err := watcher.New(rootDir)
		if err != nil {
			return watcherStartedMsg{err: err}
		}
		if err := w.Start(); err != nil {
			_ = w.Close()
			return watcherStartedMsg{err: err}
		}
		return watcherStartedMsg{w: w}
	}
}

// pumpWatcherCmd reads one event from the watcher channels and turns
// it into a tea.Msg. We re-issue the Cmd from Update to keep pumping —
// this matches how Bubble Tea idiomatically wraps long-lived sources
// (see https://github.com/charmbracelet/bubbletea#commands).
func pumpWatcherCmd(w *watcher.Watcher) tea.Cmd {
	return func() tea.Msg {
		select {
		case ev, ok := <-w.Events():
			if !ok {
				return nil
			}
			return fsEventMsg{ev: ev}
		case ev, ok := <-w.GitMeta():
			if !ok {
				return nil
			}
			return gitMetaMsg{ev: ev}
		}
	}
}

// loadFileMarkersCmd computes the per-line ChangeKind map for absPath
// so the file view can paint a gutter strip à la VSCode's git gutter.
//
// Three outcomes:
//   1. Tracked file with a diff vs HEAD → parse `git diff HEAD --` and
//      run it through diffview.Markers.
//   2. Untracked file (claude wrote a new file, never staged) →
//      synthesize an all-Add map by reading the file directly. No git
//      diff command can produce this cleanly (the file isn't in HEAD),
//      so we count newlines ourselves.
//   3. Tracked but unchanged, or any soft failure (no repo, path
//      outside the working tree, git error) → cleared=true tells the
//      renderer to drop the marker column entirely.
func loadFileMarkersCmd(repo *git.Repo, absPath string) tea.Cmd {
	if repo == nil || absPath == "" {
		return func() tea.Msg { return fileMarkersLoadedMsg{path: absPath, cleared: true} }
	}
	return func() tea.Msg {
		// EvalSymlinks both sides so a path like /var/... (symlink to
		// /private/var/...) doesn't compute as ../../../private/var/...
		// — that "../"-prefixed result was tripping the outside-the-tree
		// guard for files under macOS temp dirs and any path traversed
		// via a symlinked ancestor.
		root, err := filepath.EvalSymlinks(repo.Root())
		if err != nil { root = repo.Root() }
		real, err := filepath.EvalSymlinks(absPath)
		if err != nil { real = absPath }
		rel, err := filepath.Rel(root, real)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fileMarkersLoadedMsg{path: absPath, cleared: true}
		}
		ctx, cancel := context.WithTimeout(context.Background(), gitCmdTimeout)
		defer cancel()
		relSlash := filepath.ToSlash(rel)

		// Untracked: git diff HEAD returns empty for paths git doesn't
		// know about, so we have to detect this case explicitly. Mark
		// every line as ChangeAdd — semantically this *is* a fully-
		// added file from HEAD's point of view.
		if !repo.IsTracked(ctx, relSlash) {
			data, err := os.ReadFile(absPath)
			if err != nil {
				return fileMarkersLoadedMsg{path: absPath, cleared: true}
			}
			n := countLines(data)
			markers := make(map[int]diffview.ChangeKind, n)
			for i := 1; i <= n; i++ {
				markers[i] = diffview.ChangeAdd
			}
			return fileMarkersLoadedMsg{path: absPath, markers: markers}
		}

		raw, err := repo.DiffFile(ctx, relSlash)
		if err != nil {
			return fileMarkersLoadedMsg{path: absPath, cleared: true}
		}
		markers := diffview.Markers(diffview.Parse(raw))
		return fileMarkersLoadedMsg{path: absPath, markers: markers}
	}
}

// countLines returns the line count for the synthetic all-Add path.
// Mirrors the convention text editors use: a final \n does not introduce
// a phantom empty line, but a file ending without \n still counts its
// last line.
func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		n++
	}
	return n
}

func loadWorkingDiffCmd(repo *git.Repo) tea.Cmd {
	if repo == nil {
		return func() tea.Msg { return diffLoadedMsg{err: fmt.Errorf("no git repository")} }
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), gitCmdTimeout)
		defer cancel()
		raw, err := repo.DiffWorking(ctx)
		if err != nil {
			return diffLoadedMsg{err: err}
		}
		raw = strings.ReplaceAll(raw, "\t", tabExpansion)
		return diffLoadedMsg{
			title: "diff · working tree",
			lines: diffview.Parse(raw),
		}
	}
}
