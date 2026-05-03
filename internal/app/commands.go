package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gen-hiroto0119/sus4/internal/diffview"
	"github.com/gen-hiroto0119/sus4/internal/filetree"
	"github.com/gen-hiroto0119/sus4/internal/git"
	"github.com/gen-hiroto0119/sus4/internal/highlight"
	"github.com/gen-hiroto0119/sus4/internal/mainview"
	"github.com/gen-hiroto0119/sus4/internal/watcher"
)

// tabExpansion is what every TAB gets rewritten to before content
// reaches the highlighter or the diff parser. uniseg/ansi count a TAB
// as one cell, but terminals render it at 4–8 — that mismatch breaks
// our wrap math (lines silently overflow innerWidth, the terminal
// performs a visual wrap, and the pane height grows out of sync with
// the sibling). Spelling tabs as four spaces up front keeps counted
// width and rendered width in lock-step.
const tabExpansion = "    "

// Per Design.md §8 we cap individual git invocations so a hung
// child can't freeze the UI thread (Cmds run in a goroutine but a
// blocked syscall would still pile up).
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

func loadFileCmd(path string, trueColor bool) tea.Cmd {
	return func() tea.Msg {
		content, err := os.ReadFile(path)
		if err != nil {
			return fileLoadedMsg{path: path, err: err}
		}
		content = bytes.ReplaceAll(content, []byte{'\t'}, []byte(tabExpansion))
		r := highlight.Highlight(path, content, trueColor)
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
