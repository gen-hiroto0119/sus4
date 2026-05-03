// Package diffview parses a unified-diff text into a typed line stream
// so a renderer can apply theme colors instead of relying on raw ANSI
// from `git diff` (which is suppressed via --no-color upstream).
package diffview

import (
	"regexp"
	"strconv"
	"strings"
)

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

// Line carries one row of a unified diff. OldLine and NewLine are
// 1-based file line numbers — 0 means "not applicable" (e.g. an Add
// line has no Old number, a hunk header has neither). Renderers use
// these to draw a two-column gutter without re-parsing the hunk.
type Line struct {
	Kind    LineKind
	Text    string
	OldLine int
	NewLine int
}

// Parse splits diff text into typed lines. The result preserves order.
func Parse(diff string) []Line {
	if diff == "" {
		return nil
	}
	raw := strings.Split(strings.TrimRight(diff, "\n"), "\n")
	out := make([]Line, 0, len(raw))
	inHunk := false
	var oldLine, newLine int
	for _, l := range raw {
		out = append(out, classify(l, &inHunk, &oldLine, &newLine))
	}
	return out
}

// hunkRe captures the start line numbers from "@@ -A[,B] +C[,D] @@".
var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func classify(s string, inHunk *bool, oldLine, newLine *int) Line {
	switch {
	case strings.HasPrefix(s, "diff --git "):
		*inHunk = false
		return Line{Kind: LineFileHeader, Text: s}
	case strings.HasPrefix(s, "@@"):
		*inHunk = true
		if m := hunkRe.FindStringSubmatch(s); m != nil {
			if v, err := strconv.Atoi(m[1]); err == nil {
				*oldLine = v
			}
			if v, err := strconv.Atoi(m[2]); err == nil {
				*newLine = v
			}
		}
		return Line{Kind: LineHunk, Text: s}
	case strings.HasPrefix(s, "Binary files "):
		return Line{Kind: LineBinary, Text: s}
	case !*inHunk:
		// Pre-hunk metadata: index, mode, +++/---, similarity index, etc.
		return Line{Kind: LineMeta, Text: s}
	}
	if len(s) == 0 {
		l := Line{Kind: LineContext, Text: s, OldLine: *oldLine, NewLine: *newLine}
		*oldLine++
		*newLine++
		return l
	}
	switch s[0] {
	case '+':
		l := Line{Kind: LineAdd, Text: s, NewLine: *newLine}
		*newLine++
		return l
	case '-':
		l := Line{Kind: LineDel, Text: s, OldLine: *oldLine}
		*oldLine++
		return l
	case '\\':
		// "\ No newline at end of file"
		return Line{Kind: LineMeta, Text: s}
	default:
		l := Line{Kind: LineContext, Text: s, OldLine: *oldLine, NewLine: *newLine}
		*oldLine++
		*newLine++
		return l
	}
}
