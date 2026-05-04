package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gen-hiroto0119/tetra/internal/app"
	"github.com/gen-hiroto0119/tetra/internal/config"
)

// modulePath is the canonical go-install target used by `tetra update`.
// Pinned here rather than re-derived from runtime/debug so a user-built
// fork still self-updates against the upstream release stream by default;
// fork users can rebuild with their own ldflags or skip update entirely.
const modulePath = "github.com/gen-hiroto0119/tetra/cmd/tetra"

// version is set at link-time by goreleaser via `-X main.version=...`.
// "dev" is what unreleased local builds report (e.g. `go build`); the
// `go install module@vX.Y.Z` path is handled separately below by
// reading the embedded module version from runtime/debug.
var version = "dev"

// resolveVersion picks the best available version string.
//   - If goreleaser injected one at link time, use it (e.g. v0.1.5).
//   - Otherwise, ask runtime/debug for the module version baked in by
//     `go install ...@vX.Y.Z` — that path doesn't run goreleaser's
//     ldflags, so users would otherwise see "dev" for a tagged install.
//   - "(devel)" means there was no module version (a `go build` from
//     a local checkout); keep that as "dev" so it's obvious.
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		v := info.Main.Version
		if v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}

func main() {
	// --version short-circuits before any TUI setup so it's safe for
	// brew test (system "#{bin}/tetra", "--version") and for users
	// scripting an install verification check.
	for _, a := range os.Args[1:] {
		if a == "--version" || a == "-v" {
			fmt.Printf("tetra %s\n", resolveVersion())
			return
		}
	}

	// `tetra update` shells out to `go install ...@latest` so the user
	// can refresh the binary in place without remembering the module
	// path. It runs before parseArgs because positional args are
	// reserved for v0.2's <file> / <commit> dispatch and we don't want
	// "update" to be misread as a filename. Subcommand only — no flag
	// equivalent — keeping the surface intentionally tiny.
	if len(os.Args) >= 2 && os.Args[1] == "update" {
		os.Exit(runUpdate(os.Args[2:]))
	}

	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "tetra:", err)
		os.Exit(2)
	}

	p := tea.NewProgram(app.New(opts), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tetra:", err)
		os.Exit(1)
	}
}

// runUpdate execs `go install <modulePath>@latest`, streaming output
// directly to the user's terminal. Returns the exit code main should
// propagate. Args after "update" are reserved for future flags (e.g.
// `--version vX.Y.Z`); for v0.1 any extra arg fails fast with usage.
func runUpdate(args []string) int {
	target := modulePath + "@latest"
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "tetra: `update` takes no arguments (got %v)\n", args)
		fmt.Fprintf(os.Stderr, "       run `go install %s` directly to pin a version\n", modulePath+"@vX.Y.Z")
		return 2
	}
	fmt.Printf("tetra: running `go install %s`...\n", target)
	cmd := exec.Command("go", "install", target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// errors.Is(*exec.Error) trips when `go` itself isn't on PATH.
		// That's the most common failure mode for users who installed
		// via raw binary download, so the message points them at the
		// next-best fallback.
		var execErr *exec.Error
		if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
			fmt.Fprintln(os.Stderr, "tetra: `go` is not in PATH — install Go (https://go.dev/dl/) or")
			fmt.Fprintln(os.Stderr, "       grab a binary from https://github.com/gen-hiroto0119/tetra/releases")
			return 1
		}
		fmt.Fprintln(os.Stderr, "tetra: update failed:", err)
		return 1
	}
	fmt.Println("tetra: done. run `tetra --version` to verify.")
	return 0
}

func parseArgs(args []string) (app.Options, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return app.Options{}, err
	}

	fs := flag.NewFlagSet("tetra", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to config.toml (default: $XDG_CONFIG_HOME/tetra/config.toml)")
	if err := fs.Parse(args); err != nil {
		return app.Options{}, err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		// A bad config file is non-fatal: warn and proceed with defaults.
		fmt.Fprintf(os.Stderr, "tetra: config load failed (%v); using defaults\n", err)
		cfg = config.Default()
	}

	opts := app.Options{
		RootDir: cwd,
		Target:  app.StartupTarget{Kind: app.StartupDir},
		Config:  cfg,
	}

	rest := fs.Args()
	if len(rest) == 0 {
		return opts, nil
	}

	// v0.1: accept the argument shape but always fall back to dir mode.
	// File / commit dispatching is implemented in v0.2.
	opts.Target = app.StartupTarget{Kind: app.StartupDir, Arg: rest[0]}
	fmt.Fprintf(os.Stderr, "tetra: argument %q ignored in v0.1 (file/commit modes land in v0.2)\n", rest[0])
	return opts, nil
}
