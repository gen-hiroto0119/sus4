package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenReturnsErrNotARepoOutsideGit(t *testing.T) {
	dir := t.TempDir()
	_, err := Open(context.Background(), dir)
	if !errors.Is(err, ErrNotARepo) {
		t.Fatalf("got %v, want ErrNotARepo", err)
	}
}

func TestOpenResolvesRoot(t *testing.T) {
	dir := initRepo(t)
	sub := filepath.Join(dir, "nested")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := Open(context.Background(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Root() != evalSymlinks(t, dir) {
		t.Errorf("Root() = %q, want %q", repo.Root(), evalSymlinks(t, dir))
	}
}

func TestStatusBucketsModifiedAddedDeletedUntracked(t *testing.T) {
	dir := initRepo(t)
	commitFile(t, dir, "tracked.txt", "v1\n")
	commitFile(t, dir, "to-delete.txt", "bye\n")

	// Modify a tracked file.
	mustWrite(t, dir, "tracked.txt", "v2\n")
	// Delete one.
	mustRemove(t, dir, "to-delete.txt")
	// Stage a brand-new file.
	mustWrite(t, dir, "added.txt", "new\n")
	mustGit(t, dir, "add", "added.txt")
	// Leave one untracked.
	mustWrite(t, dir, "untracked.txt", "?\n")

	repo, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := repo.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]StatusKind{}
	for _, e := range entries {
		got[e.Path] = e.Kind
	}
	want := map[string]StatusKind{
		"tracked.txt":   StatusModified,
		"to-delete.txt": StatusDeleted,
		"added.txt":     StatusAdded,
		"untracked.txt": StatusUntracked,
	}
	for path, kind := range want {
		if got[path] != kind {
			t.Errorf("status[%s] = %v, want %v", path, got[path], kind)
		}
	}
}

func TestStatusEmptyForCleanRepo(t *testing.T) {
	dir := initRepo(t)
	commitFile(t, dir, "a.txt", "x\n")

	repo, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := repo.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected clean status, got %+v", entries)
	}
}

func TestDiffWorkingIncludesModifiedHunk(t *testing.T) {
	dir := initRepo(t)
	commitFile(t, dir, "a.txt", "one\n")
	mustWrite(t, dir, "a.txt", "one\ntwo\n")

	repo, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	d, err := repo.DiffWorking(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d, "+two") {
		t.Errorf("diff missing +two:\n%s", d)
	}
	if !strings.Contains(d, "diff --git") {
		t.Errorf("diff missing header:\n%s", d)
	}
}

func TestDiffWorkingHandlesEmptyRepo(t *testing.T) {
	dir := initRepo(t)
	mustWrite(t, dir, "a.txt", "x\n")
	mustGit(t, dir, "add", "a.txt")

	repo, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	// No HEAD yet — should not error.
	d, err := repo.DiffWorking(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(d, "a.txt") {
		t.Errorf("expected a.txt in diff, got:\n%s", d)
	}
}

func TestParseStatusZHandlesRenames(t *testing.T) {
	// Synthetic input: a rename "old.txt" -> "new.txt".
	raw := []byte("R  new.txt\x00old.txt\x00")
	entries := parseStatusZ(raw)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Kind != StatusRenamed || e.Path != "new.txt" || e.OrigPath != "old.txt" {
		t.Errorf("entry = %+v", e)
	}
}

func TestParseStatusZSkipsShortRecords(t *testing.T) {
	if got := parseStatusZ(nil); got != nil {
		t.Errorf("nil input: got %v", got)
	}
	if got := parseStatusZ([]byte("XY")); got != nil {
		t.Errorf("short record: got %v", got)
	}
}

// --- helpers ---

func initRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	mustGit(t, dir, "init", "-q", "-b", "main")
	mustGit(t, dir, "config", "user.email", "test@example.com")
	mustGit(t, dir, "config", "user.name", "test")
	mustGit(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

func commitFile(t *testing.T, dir, name, body string) {
	t.Helper()
	mustWrite(t, dir, name, body)
	mustGit(t, dir, "add", name)
	mustGit(t, dir, "commit", "-q", "-m", "add "+name)
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2020-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2020-01-01T00:00:00Z")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func mustWrite(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRemove(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
}

// macOS resolves /var -> /private/var; git's --show-toplevel returns the
// canonicalized form. Match that in assertions.
func evalSymlinks(t *testing.T, p string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p
	}
	return resolved
}
