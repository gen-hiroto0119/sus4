// Package highlight wraps Chroma to produce ANSI-colored source for the
// terminal. UI layers consume strings; they don't see Chroma directly.
//
// A (path, mtime, size) LRU is on the roadmap. v0.1 keeps it simple:
// callers run Highlight from a tea.Cmd, and the result is small enough
// to discard between focuses. A cache lands when profiling shows it
// helps the "1 second to reflect a Claude Code edit" budget.
package highlight

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/glamour"
)

// MaxBytes is the cutoff above which we render the file as plain text
// rather than running Chroma.
const MaxBytes = 1 << 20 // 1 MiB

// Result captures both the rendered text and metadata the UI needs to
// decide whether to display banners ("Binary file", "Highlighting
// skipped (large file)").
type Result struct {
	Text   string
	Plain  bool   // true when highlighting was skipped
	Reason string // human-readable note when Plain is true
	Binary bool   // true when content was detected as binary
}

// Highlight returns ANSI-colored output for content keyed by filename.
// Filename drives lexer detection; an empty filename triggers content
// analysis. terminalTrueColor selects the 24-bit formatter when true.
// dark picks a chroma + glamour style that reads well on a dark vs.
// light terminal background.
func Highlight(filename string, content []byte, terminalTrueColor, dark bool) Result {
	if isBinary(content) {
		return Result{Text: "Binary file", Plain: true, Binary: true, Reason: "binary"}
	}
	if len(content) > MaxBytes {
		return Result{Text: string(content), Plain: true, Reason: "large file (>1 MiB), highlighting skipped"}
	}

	// Markdown gets a rendered preview rather than a syntax-coloured
	// view of the raw source — tetra is a viewer for non-writers, so a
	// reading-mode display is what actually helps.
	if isMarkdown(filename) {
		if out, err := renderMarkdown(content, dark); err == nil {
			return Result{Text: out}
		}
		// Fall through to chroma if glamour fails for any reason —
		// the raw source highlighted is still better than an error.
	}

	lexer := pickLexer(filename, content)
	style := styles.Get(chromaStyleName(dark))
	if style == nil {
		style = styles.Fallback
	}
	formatter := formatters.Get("terminal256")
	if terminalTrueColor {
		if f := formatters.Get("terminal16m"); f != nil {
			formatter = f
		}
	}
	if formatter == nil {
		formatter = formatters.Fallback
	}

	iter, err := lexer.Tokenise(nil, string(content))
	if err != nil {
		return Result{Text: string(content), Plain: true, Reason: "tokenise failed"}
	}
	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iter); err != nil {
		return Result{Text: string(content), Plain: true, Reason: "format failed"}
	}
	return Result{Text: buf.String()}
}

// chromaStyleName picks a chroma palette suited to the host background.
// monokai is the long-standing dark default; github reads well on a
// near-white field without pushing accents into the high-contrast zone.
func chromaStyleName(dark bool) string {
	if dark {
		return "monokai"
	}
	return "github"
}

func pickLexer(filename string, content []byte) chroma.Lexer {
	var l chroma.Lexer
	if filename != "" {
		l = lexers.Match(filename)
	}
	if l == nil {
		l = lexers.Analyse(string(content))
	}
	if l == nil {
		l = lexers.Fallback
	}
	return chroma.Coalesce(l)
}

// isBinary follows the heuristic: NUL byte in the
// first 8 KiB.
func isBinary(content []byte) bool {
	limit := len(content)
	if limit > 8192 {
		limit = 8192
	}
	return bytes.IndexByte(content[:limit], 0) >= 0
}

// TerminalSupportsTrueColor inspects the environment as Chroma users
// commonly do.
func TerminalSupportsTrueColor() bool {
	v := strings.ToLower(os.Getenv("COLORTERM"))
	return v == "truecolor" || v == "24bit"
}

// markdownWrapWidth is the column glamour wraps paragraphs at. Picking
// a relatively wide value lets the mainview's own ansi.Hardwrap handle
// the actual viewport width — glamour just needs to break ridiculously
// long lines so its formatter doesn't choke.
const markdownWrapWidth = 100

// NewLineHighlighter returns a function that ANSI-highlights a single
// line of source as if it came from `filename`. It is meant for the
// diff renderer: each diff body row carries one line of code, and we
// want the same lexer / theme as the file view but applied per-row.
//
// Multi-line constructs (block comments, here-docs) lose context — that
// is the trade-off of single-line highlighting. Acceptable for now;
// move to whole-hunk highlighting later if it shows in real use.
func NewLineHighlighter(filename string, trueColor, dark bool) func(string) string {
	lexer := lexers.Match(filename)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get(chromaStyleName(dark))
	if style == nil {
		style = styles.Fallback
	}
	formatter := formatters.Get("terminal256")
	if trueColor {
		if f := formatters.Get("terminal16m"); f != nil {
			formatter = f
		}
	}
	if formatter == nil {
		formatter = formatters.Fallback
	}

	return func(line string) string {
		if line == "" {
			return ""
		}
		iter, err := lexer.Tokenise(nil, line)
		if err != nil {
			return line
		}
		var buf bytes.Buffer
		if err := formatter.Format(&buf, style, iter); err != nil {
			return line
		}
		return buf.String()
	}
}

func isMarkdown(filename string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return true
	}
	return false
}

func renderMarkdown(content []byte, dark bool) (string, error) {
	style := "dark"
	if !dark {
		style = "light"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(markdownWrapWidth),
	)
	if err != nil {
		return "", err
	}
	return r.Render(string(content))
}
