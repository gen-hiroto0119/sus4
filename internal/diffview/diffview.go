// Package diffview parses a unified-diff text into a typed line stream
// so a renderer can apply theme colors instead of relying on raw ANSI
// from `git diff` (which is suppressed via --no-color upstream).
package diffview

import "strings"

type LineKind int

const (
	LineFileHeader LineKind = iota // "diff --git a/foo b/foo"
	LineMeta                       // "index ...", "new file mode ...", "+++", "---"
	LineHunk                       // "@@ -1,4 +1,5 @@"
	LineAdd
	LineDel
	LineContext
	LineBinary // "Binary files a/x and b/x differ"
)

type Line struct {
	Kind LineKind
	Text string // raw text, no trailing newline
}

// Parse splits diff text into typed lines. The result preserves order.
func Parse(diff string) []Line {
	if diff == "" {
		return nil
	}
	raw := strings.Split(strings.TrimRight(diff, "\n"), "\n")
	out := make([]Line, 0, len(raw))
	inHunk := false
	for _, l := range raw {
		out = append(out, classify(l, &inHunk))
	}
	return out
}

func classify(s string, inHunk *bool) Line {
	switch {
	case strings.HasPrefix(s, "diff --git "):
		*inHunk = false
		return Line{Kind: LineFileHeader, Text: s}
	case strings.HasPrefix(s, "@@"):
		*inHunk = true
		return Line{Kind: LineHunk, Text: s}
	case strings.HasPrefix(s, "Binary files "):
		return Line{Kind: LineBinary, Text: s}
	case !*inHunk:
		// Pre-hunk metadata: index, mode, +++/---, similarity index, etc.
		return Line{Kind: LineMeta, Text: s}
	}
	if len(s) == 0 {
		return Line{Kind: LineContext, Text: s}
	}
	switch s[0] {
	case '+':
		return Line{Kind: LineAdd, Text: s}
	case '-':
		return Line{Kind: LineDel, Text: s}
	case '\\':
		// "\ No newline at end of file"
		return Line{Kind: LineMeta, Text: s}
	default:
		return Line{Kind: LineContext, Text: s}
	}
}
