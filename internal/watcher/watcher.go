// Package watcher wraps fsnotify to feed coalesced filesystem events
// into a Bubble Tea Update loop.
//
// Per Design.md §7:
//   - the entire root tree is watched recursively (excluding the
//     hardcoded set: .git, node_modules, vendor),
//   - each path's events are debounced with a 50 ms window so atomic
//     saves (CREATE → RENAME → REMOVE) collapse into a single signal,
//   - .git/HEAD and .git/index are watched separately and emit their
//     own message types so the app can route them straight to git
//     status / commit reload paths.
//
// All output flows through Events(); the watcher never touches a UI
// model directly. Cmds invoke this via tea.Cmd, the resulting goroutine
// is owned by Watcher.Start until Close.
package watcher

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/gen-hiroto0119/tetra/internal/filetree"
)

// debounceWindow is short enough to feel instant (Design.md §7.2: 50 ms)
// while wide enough to merge editor save bursts.
const debounceWindow = 50 * time.Millisecond

// Event is the coalesced output. Op is the union of all fsnotify ops
// observed in the debounce window.
type Event struct {
	Path string
	Op   fsnotify.Op
}

// IsStructural reports whether the event represents a directory-level
// change (create / remove / rename) — i.e. one that may have added or
// dropped a child node, so the sidebar tree needs to re-list the parent.
func (e Event) IsStructural() bool {
	return e.Op.Has(fsnotify.Create) || e.Op.Has(fsnotify.Remove) || e.Op.Has(fsnotify.Rename)
}

// GitMetaKind narrows the kind of git metadata change.
type GitMetaKind int

const (
	GitMetaHEAD GitMetaKind = iota
	GitMetaIndex
)

type GitMetaEvent struct {
	Kind GitMetaKind
}

// Watcher owns the underlying fsnotify watcher and the debounce timers.
type Watcher struct {
	rootDir string
	gitDir  string

	w        *fsnotify.Watcher
	events   chan Event
	gitMeta  chan GitMetaEvent
	errs     chan error
	done     chan struct{}

	mu     sync.Mutex
	timers map[string]*time.Timer
	ops    map[string]fsnotify.Op
}

// New creates a watcher rooted at rootDir but does not start it. Call
// Start to launch the background goroutine.
func New(rootDir string) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		rootDir: rootDir,
		gitDir:  filepath.Join(rootDir, ".git"),
		w:       w,
		events:  make(chan Event, 32),
		gitMeta: make(chan GitMetaEvent, 8),
		errs:    make(chan error, 4),
		done:    make(chan struct{}),
		timers:  map[string]*time.Timer{},
		ops:     map[string]fsnotify.Op{},
	}, nil
}

// Events is the coalesced file event stream.
func (w *Watcher) Events() <-chan Event { return w.events }

// GitMeta carries .git/HEAD and .git/index changes.
func (w *Watcher) GitMeta() <-chan GitMetaEvent { return w.gitMeta }

// Errors carries non-fatal watcher errors.
func (w *Watcher) Errors() <-chan error { return w.errs }

// Start adds the initial watch list and begins forwarding events. The
// returned error covers only setup; runtime errors flow through Errors().
func (w *Watcher) Start() error {
	if err := w.addRecursive(w.rootDir); err != nil {
		return err
	}
	// .git metadata is best-effort: a non-git directory simply means
	// these Add calls fail and we ignore them.
	_ = w.w.Add(filepath.Join(w.gitDir, "HEAD"))
	_ = w.w.Add(filepath.Join(w.gitDir, "index"))

	go w.loop()
	return nil
}

// Close stops the watcher and releases OS resources. It is safe to
// call multiple times.
func (w *Watcher) Close() error {
	select {
	case <-w.done:
		return nil
	default:
		close(w.done)
	}
	return w.w.Close()
}

func (w *Watcher) loop() {
	defer close(w.events)
	defer close(w.gitMeta)
	defer close(w.errs)

	for {
		select {
		case <-w.done:
			return
		case err, ok := <-w.w.Errors:
			if !ok {
				return
			}
			select {
			case w.errs <- err:
			default:
			}
		case ev, ok := <-w.w.Events:
			if !ok {
				return
			}
			w.handle(ev)
		}
	}
}

func (w *Watcher) handle(ev fsnotify.Event) {
	// .git metadata is dispatched immediately — no debounce. These
	// signals are rare and we want the changes pane to refresh asap.
	switch filepath.Clean(ev.Name) {
	case filepath.Join(w.gitDir, "HEAD"):
		w.send(GitMetaEvent{Kind: GitMetaHEAD})
		return
	case filepath.Join(w.gitDir, "index"):
		w.send(GitMetaEvent{Kind: GitMetaIndex})
		return
	}

	// Newly created directory? Walk it so subsequent edits inside fire
	// too — but only when the new dir is *not* on the exclude list.
	// Without this check, a build tool that creates `.next` / `dist` /
	// etc. mid-session would have its entire output tree silently added
	// to the watch set, bypassing the recursive-add filter that only
	// fires for descendants. That single oversight contributed to the
	// 200%+ CPU regression seen on Next.js-style projects.
	if ev.Has(fsnotify.Create) {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			if !filetree.IsExcluded(filepath.Base(ev.Name)) {
				_ = w.addRecursive(ev.Name)
			}
		}
	}

	w.coalesce(ev)
}

// coalesce merges per-path events within debounceWindow. Each path
// gets at most one outstanding timer; the union of ops is reported.
func (w *Watcher) coalesce(ev fsnotify.Event) {
	path := ev.Name
	w.mu.Lock()
	w.ops[path] |= ev.Op
	if t, ok := w.timers[path]; ok {
		t.Reset(debounceWindow)
		w.mu.Unlock()
		return
	}
	w.timers[path] = time.AfterFunc(debounceWindow, func() { w.flush(path) })
	w.mu.Unlock()
}

func (w *Watcher) flush(path string) {
	w.mu.Lock()
	op := w.ops[path]
	delete(w.ops, path)
	delete(w.timers, path)
	w.mu.Unlock()

	// Atomic save: the original inode is gone after RENAME/REMOVE.
	// Re-Add when the path still exists so the next save lands on
	// our radar (Design.md §7.3 best-effort).
	if op&(fsnotify.Rename|fsnotify.Remove) != 0 {
		if _, err := os.Stat(path); err == nil {
			_ = w.w.Add(path)
		}
	}

	select {
	case <-w.done:
		return
	case w.events <- Event{Path: path, Op: op}:
	}
}

func (w *Watcher) send(ev GitMetaEvent) {
	select {
	case <-w.done:
	case w.gitMeta <- ev:
	default:
		// Drop on full buffer: a duplicate HEAD/index notification
		// is harmless; we only need the next one to arrive.
	}
}

// addRecursive walks the tree and watches every directory not on the
// hardcoded exclude list. Files are not added individually because
// fsnotify reports their changes through their parent directory.
func (w *Watcher) addRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Permission errors etc. are non-fatal: skip the subtree.
			if errors.Is(err, os.ErrPermission) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && filetree.IsExcluded(d.Name()) {
			return filepath.SkipDir
		}
		if err := w.w.Add(path); err != nil {
			return nil
		}
		return nil
	})
}
