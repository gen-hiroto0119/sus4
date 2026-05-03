package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherCoalescesBurstIntoSingleEvent(t *testing.T) {
	dir := t.TempDir()
	w := mustStart(t, dir)
	defer w.Close()

	path := filepath.Join(dir, "a.txt")
	// Three rapid writes within the debounce window must surface as one
	// event (this is the "atomic save burst" case from Design.md §7.2).
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(path, []byte{byte(i)}, 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	ev := mustReceiveEvent(t, w, 500*time.Millisecond)
	if filepath.Clean(ev.Path) != filepath.Clean(path) {
		t.Errorf("event path = %q, want %q", ev.Path, path)
	}

	// Verify no second event piles up.
	select {
	case extra := <-w.Events():
		t.Errorf("expected no second event, got %+v", extra)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestWatcherSkipsExcludedDirectories(t *testing.T) {
	dir := t.TempDir()
	excluded := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(excluded, 0o755); err != nil {
		t.Fatal(err)
	}
	w := mustStart(t, dir)
	defer w.Close()

	// Mutate inside an excluded subtree — should not produce events.
	if err := os.WriteFile(filepath.Join(excluded, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-w.Events():
		t.Errorf("excluded dir leaked event: %+v", ev)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestWatcherFollowsNewlyCreatedSubdirectory(t *testing.T) {
	dir := t.TempDir()
	w := mustStart(t, dir)
	defer w.Close()

	sub := filepath.Join(dir, "fresh")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// Drain the create-of-fresh event itself.
	mustReceiveEvent(t, w, 500*time.Millisecond)

	// Now write inside the new subdir; it must reach us, which only
	// works if the watcher auto-Added it on the create event.
	target := filepath.Join(sub, "child.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ev := mustReceiveEvent(t, w, 500*time.Millisecond)
	if filepath.Clean(ev.Path) != filepath.Clean(target) {
		t.Errorf("event for %q, want %q", ev.Path, target)
	}
}

func TestWatcherEmitsGitMetaEventsForHEAD(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	headPath := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := mustStart(t, dir)
	defer w.Close()

	if err := os.WriteFile(headPath, []byte("ref: refs/heads/dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-w.GitMeta():
		if ev.Kind != GitMetaHEAD {
			t.Errorf("kind = %v, want GitMetaHEAD", ev.Kind)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for GitMeta event")
	}
}

// --- helpers ---

func mustStart(t *testing.T, dir string) *Watcher {
	t.Helper()
	w, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	return w
}

func mustReceiveEvent(t *testing.T, w *Watcher, timeout time.Duration) Event {
	t.Helper()
	select {
	case ev := <-w.Events():
		return ev
	case <-time.After(timeout):
		t.Fatal("timed out waiting for Event")
		return Event{}
	}
}
