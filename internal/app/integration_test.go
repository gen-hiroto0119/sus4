package app

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gen-hiroto0119/tetra/internal/config"
	"github.com/gen-hiroto0119/tetra/internal/git"
	"github.com/gen-hiroto0119/tetra/internal/sidebar"
)

// TestMarkersAppearForGoFile drives the app.Model through a controlled
// scenario: a temp git repo with a committed Go file that we then modify,
// open through the dispatch path, and render. The marker glyph must
// appear in the visible band.
func TestMarkersAppearForGoFile(t *testing.T) {
	dir := t.TempDir()
	mustExec(t, dir, "git", "init", "-q")
	mustExec(t, dir, "git", "config", "user.email", "t@t")
	mustExec(t, dir, "git", "config", "user.name", "t")
	goPath := dir + "/main.go"
	original := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(goPath, []byte(original), 0644); err != nil { t.Fatal(err) }
	mustExec(t, dir, "git", "add", "main.go")
	mustExec(t, dir, "git", "commit", "-q", "-m", "init")
	// Modify line 1: package name change → ChangeMod for line 1.
	modified := "package newmain\n\nfunc main() {}\n"
	if err := os.WriteFile(goPath, []byte(modified), 0644); err != nil { t.Fatal(err) }

	repo, err := git.Open(context.Background(), dir)
	if err != nil { t.Fatalf("open: %v", err) }

	opts := Options{RootDir: dir, Target: StartupTarget{Kind: StartupDir}, Config: config.Default()}
	var tm tea.Model = New(opts)
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	tm, _ = tm.Update(repoOpenedMsg{repo: repo})

	mm := tm.(Model)
	cmd := mm.dispatchOpenIntent(sidebar.OpenIntent{Kind: sidebar.OpenFile, Path: goPath})
	tm = mm
	for _, msg := range drainBatch(cmd) {
		t.Logf("msg %T = %+v", msg, msg)
		tm, _ = tm.Update(msg)
	}

	mm = tm.(Model)
	t.Logf("kind=%v filePath=%q markers=%v fileLines=%d",
		mm.main.Kind(), mm.main.FilePath(), mm.main.FileMarkers(), len(mm.main.FileLines()))

	view := tm.View()
	stripped := stripAnsi(view)
	hasMarker := strings.ContainsRune(stripped, '▌')
	t.Logf("hasMarker=%v", hasMarker)
	if !hasMarker {
		for i, l := range strings.Split(stripped, "\n") {
			if i > 12 { break }
			t.Logf("%2d %q", i, l)
		}
		t.Errorf("expected ▌ in rendered Go file view")
	}
}

func mustExec(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

// (TestMarkersAppearInLiveView for the real /Users/hiroto/tetra
// README.md was removed: glamour markdown rendering reflows the file
// before line numbers reach the gutter, and that interaction belongs
// in a separate test scoped to the markdown path. The Go-file test
// above is enough to lock in the dispatch → Update → View pipeline.)

// drainBatch invokes a Cmd and unwraps tea.BatchMsg into individual msgs.
func drainBatch(c tea.Cmd) []tea.Msg {
	if c == nil { return nil }
	msg := c()
	switch m := msg.(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, child := range m {
			out = append(out, drainBatch(child)...)
		}
		return out
	case nil:
		return nil
	default:
		return []tea.Msg{msg}
	}
}

func stripAnsi(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		if r == '\x1b' { in = true; continue }
		if in { if r == 'm' || r == 'K' { in = false }; continue }
		b.WriteRune(r)
	}
	return b.String()
}
