package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gen-hiroto0119/sus4/internal/app"
	"github.com/gen-hiroto0119/sus4/internal/config"
)

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "sus4:", err)
		os.Exit(2)
	}

	p := tea.NewProgram(app.New(opts), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "sus4:", err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (app.Options, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return app.Options{}, err
	}

	fs := flag.NewFlagSet("sus4", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to config.toml (default: $XDG_CONFIG_HOME/sus4/config.toml)")
	if err := fs.Parse(args); err != nil {
		return app.Options{}, err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		// A bad config file is non-fatal: warn and proceed with defaults.
		fmt.Fprintf(os.Stderr, "sus4: config load failed (%v); using defaults\n", err)
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

	// v0.1: accept the argument shape but always fall back to dir mode
	// (Design.md §5: "v0.1 では引数を受けても警告を出して no arg 経路にフォールバックする").
	// File / commit dispatching is implemented in v0.2.
	opts.Target = app.StartupTarget{Kind: app.StartupDir, Arg: rest[0]}
	fmt.Fprintf(os.Stderr, "sus4: argument %q ignored in v0.1 (file/commit modes land in v0.2)\n", rest[0])
	return opts, nil
}
