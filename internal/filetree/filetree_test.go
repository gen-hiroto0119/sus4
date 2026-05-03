package filetree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSortsDirsFirstThenFiles(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, dir, "zeta")
	mustMkdir(t, dir, "alpha")
	mustWrite(t, dir, "readme.md")
	mustWrite(t, dir, "Apple.go")

	nodes, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}

	want := []struct {
		name string
		kind NodeKind
	}{
		{"alpha", NodeDir},
		{"zeta", NodeDir},
		{"Apple.go", NodeFile},
		{"readme.md", NodeFile},
	}
	if len(nodes) != len(want) {
		t.Fatalf("got %d nodes, want %d: %#v", len(nodes), len(want), nodes)
	}
	for i, w := range want {
		if nodes[i].Name != w.name || nodes[i].Kind != w.kind {
			t.Errorf("nodes[%d] = %+v, want name=%s kind=%v", i, nodes[i], w.name, w.kind)
		}
	}
}

func TestReadExcludesHardcodedDirs(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, dir, ".git")
	mustMkdir(t, dir, "node_modules")
	mustMkdir(t, dir, "vendor")
	mustMkdir(t, dir, "src")
	mustWrite(t, dir, "go.mod")

	nodes, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, n := range nodes {
		if n.Name == ".git" || n.Name == "node_modules" || n.Name == "vendor" {
			t.Errorf("excluded dir leaked: %s", n.Name)
		}
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 visible nodes, got %d: %#v", len(nodes), nodes)
	}
}

func TestReadDotfilesFlagged(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, ".env")
	mustWrite(t, dir, "main.go")

	nodes, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("got %d, want 2", len(nodes))
	}
	for _, n := range nodes {
		if n.Name == ".env" && !n.Hidden {
			t.Error(".env should be marked Hidden")
		}
		if n.Name == "main.go" && n.Hidden {
			t.Error("main.go should not be marked Hidden")
		}
	}
}

func TestReadTruncatesLargeDirectory(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < MaxEntriesPerDir+25; i++ {
		mustWrite(t, dir, filepath.Join("f"+pad(i)))
	}
	nodes, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != MaxEntriesPerDir+1 {
		t.Fatalf("got %d nodes, want %d", len(nodes), MaxEntriesPerDir+1)
	}
	last := nodes[len(nodes)-1]
	if last.Kind != NodeTruncated {
		t.Errorf("last node = %v, want NodeTruncated", last.Kind)
	}
	if last.HiddenCount != 25 {
		t.Errorf("HiddenCount = %d, want 25", last.HiddenCount)
	}
}

func TestReadReturnsErrorForMissingDir(t *testing.T) {
	_, err := Read(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing dir")
	}
}

func TestIsExcluded(t *testing.T) {
	for _, name := range []string{".git", "node_modules", "vendor"} {
		if !IsExcluded(name) {
			t.Errorf("IsExcluded(%q) = false, want true", name)
		}
	}
	for _, name := range []string{".github", "src", "main.go", ""} {
		if IsExcluded(name) {
			t.Errorf("IsExcluded(%q) = true, want false", name)
		}
	}
}

func mustMkdir(t *testing.T, parent, name string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(parent, name), 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, parent, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(parent, name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func pad(i int) string {
	const digits = "0123456789"
	if i < 10 {
		return "00" + string(digits[i])
	}
	if i < 100 {
		return "0" + string(digits[i/10]) + string(digits[i%10])
	}
	return string(digits[i/100]) + string(digits[(i/10)%10]) + string(digits[i%10])
}
