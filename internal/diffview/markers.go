package diffview

// ChangeKind labels a single line in the **new** revision of a file
// (i.e. the on-disk version) according to how it relates to HEAD. The
// renderer in mainview consumes this to draw a one-cell gutter marker
// next to each affected line — VSCode's "git gutter" treatment.
type ChangeKind int

const (
	ChangeNone ChangeKind = iota
	// ChangeAdd marks a line that exists in the new file but not in the
	// old (a contiguous + run with no preceding - run).
	ChangeAdd
	// ChangeMod marks a line that replaces one or more old lines (a + run
	// preceded by a - run within the same hunk).
	ChangeMod
	// ChangeDel marks the surviving line that sits *immediately after* a
	// deletion. It is the closest visual proxy for "lines were removed
	// here" given that deleted text has no row in the new file.
	ChangeDel
)

// Markers walks a parsed unified diff and returns a 1-based map from
// new-file line number to ChangeKind. Multi-file diffs are folded into
// one map: callers using this for a single-file view should pass the
// result of Parse on a per-file diff (e.g. `git diff -- <path>`).
//
// Algorithm: track contiguous - and + runs inside a hunk. On hunk
// boundaries (LineContext, LineHunk, LineFileHeader, end-of-input) the
// pending run is flushed:
//   - + run with no preceding - → ChangeAdd
//   - + run with preceding -    → ChangeMod (covers all the + lines)
//   - - run with no following + → ChangeDel placed on the next surviving
//     context line, when one exists
func Markers(lines []Line) map[int]ChangeKind {
	out := map[int]ChangeKind{}
	pendingDel := false
	addRun := []int(nil)

	flush := func(boundary Line) {
		switch {
		case len(addRun) > 0 && pendingDel:
			for _, n := range addRun {
				out[n] = ChangeMod
			}
		case len(addRun) > 0:
			for _, n := range addRun {
				out[n] = ChangeAdd
			}
		case pendingDel && boundary.Kind == LineContext && boundary.NewLine > 0:
			// A pure deletion: stamp the surviving line below it.
			// Don't overwrite an existing marker (Mod/Add wins — the
			// deletion is visually adjacent to a more meaningful edit).
			if _, exists := out[boundary.NewLine]; !exists {
				out[boundary.NewLine] = ChangeDel
			}
		}
		pendingDel = false
		addRun = nil
	}

	for _, l := range lines {
		switch l.Kind {
		case LineDel:
			pendingDel = true
		case LineAdd:
			if l.NewLine > 0 {
				addRun = append(addRun, l.NewLine)
			}
		case LineContext, LineHunk, LineFileHeader:
			flush(l)
		}
	}
	flush(Line{})
	return out
}
