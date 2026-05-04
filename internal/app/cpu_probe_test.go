package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gen-hiroto0119/tetra/internal/config"
	"github.com/gen-hiroto0119/tetra/internal/git"
	"github.com/gen-hiroto0119/tetra/internal/sidebar"
	"github.com/gen-hiroto0119/tetra/internal/watcher"
)

// BenchmarkUpdateFsEventBurst simulates the hot path: a file is open in
// the main view, fsEventMsg arrives at burst rate, Update runs the
// throttled-Cmd dance, View() is called after each Update. The bench
// loop is what `go test -cpuprofile` will sample, so the resulting
// profile is exactly the steady-state CPU shape.
func BenchmarkUpdateFsEventBurst(b *testing.B) {
	dir := b.TempDir()
	mustExecB(b, dir, "git", "init", "-q")
	mustExecB(b, dir, "git", "config", "user.email", "t@t")
	mustExecB(b, dir, "git", "config", "user.name", "t")

	// Realistic-ish source file: 200 lines of Go code.
	body := []byte("package main\n")
	for i := 0; i < 200; i++ {
		body = append(body, []byte("// some comment line that's a bit long to keep highlighter busy "+itoa(i)+"\n")...)
	}
	srcPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(srcPath, body, 0644); err != nil { b.Fatal(err) }
	mustExecB(b, dir, "git", "add", "main.go")
	mustExecB(b, dir, "git", "commit", "-q", "-m", "init")

	repo, err := git.Open(context.Background(), dir)
	if err != nil { b.Fatal(err) }
	opts := Options{RootDir: dir, Target: StartupTarget{Kind: StartupDir}, Config: config.Default()}
	var tm tea.Model = New(opts)
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	tm, _ = tm.Update(repoOpenedMsg{repo: repo})

	// Drive an open of main.go and pump the resulting batch synchronously.
	mm := tm.(Model)
	cmd := mm.dispatchOpenIntent(sidebar.OpenIntent{Kind: sidebar.OpenFile, Path: srcPath})
	tm = mm
	for _, msg := range drainBatch(cmd) {
		tm, _ = tm.Update(msg)
	}

	// Synthetic fs event for the active file. The bench measures
	// Update(fsEvent) + View() at 1 message per iter. This is what
	// happens N times per second when a real watcher is firing.
	fsMsg := fsEventMsg{ev: watcher.Event{Path: srcPath}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm, _ = tm.Update(fsMsg)
		_ = tm.View()
	}
}

func mustExecB(b *testing.B, dir, name string, args ...string) {
	b.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		b.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func itoa(n int) string {
	if n == 0 { return "0" }
	s := ""
	for n > 0 { s = string(rune('0'+n%10)) + s; n /= 10 }
	return s
}
