package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gen-hiroto0119/sus4/internal/app"
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

	opts := app.Options{
		RootDir: cwd,
		Target:  app.StartupTarget{Kind: app.StartupDir},
	}

	if len(args) == 0 {
		return opts, nil
	}

	// v0.1: accept the argument shape but always fall back to dir mode
	// (Design.md §5: "v0.1 では引数を受けても警告を出して no arg 経路にフォールバックする").
	// File / commit dispatching is implemented in v0.2.
	opts.Target = app.StartupTarget{Kind: app.StartupDir, Arg: args[0]}
	fmt.Fprintf(os.Stderr, "sus4: argument %q ignored in v0.1 (file/commit modes land in v0.2)\n", args[0])
	return opts, nil
}
