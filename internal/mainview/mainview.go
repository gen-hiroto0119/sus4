// Package mainview renders the right-hand pane: file body, diff, or
// (v0.2) a specific commit's patch.
//
// The view holds rendered text only. It does not perform I/O; the app
// layer feeds it via Set* methods after Cmds resolve. Scroll positions
// are remembered per (kind, identifier).
package mainview

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/gen-hiroto0119/tetra/internal/diffview"
	"github.com/gen-hiroto0119/tetra/internal/highlight"
	"github.com/gen-hiroto0119/tetra/internal/theme"
)

type Kind int

const (
	KindEmpty Kind = iota
	KindFile
	KindDiff
	KindCommit
)

type Model struct {
	// revision is bumped on every state change that affects the
	// rendered output, so the app-level view cache (app.viewCache) can
	// short-circuit rendering when nothing has actually changed. Pure
	// reads (Kind, FilePath, etc.) leave it alone.
	revision int

	kind Kind

	// File content: highlighted lines (ANSI-coded).
	filePath   string
	fileLines  []string
	fileBanner string
	// fileMarkers maps 1-based new-file line numbers to a ChangeKind so
	// the renderer can paint a one-cell git-gutter column. nil disables
	// the column entirely (non-git directories, untracked files, or
	// markers not yet computed); an empty map keeps the column visible
	// but draws no glyphs (clean tree).
	fileMarkers map[int]diffview.ChangeKind

	// Diff content.
	diffTitle string
	diffLines []diffview.Line

	// Scroll position for the active view.
	scroll int
	// Memoized positions, keyed by a composite (kind, ident).
	memo map[memKey]int

	width  int
	height int
	err    error

	// trueColor selects chroma's 24-bit formatter for diff body
	// highlighting. Same value the file view path uses.
	trueColor bool
}

type memKey struct {
	kind Kind
	id   string
}

func New(trueColor bool) Model {
	return Model{kind: KindEmpty, memo: map[memKey]int{}, trueColor: trueColor}
}

func (m *Model) SetSize(w, h int) {
	if m.width != w || m.height != h {
		m.revision++
	}
	m.width = w
	m.height = h
}

func (m *Model) Kind() Kind { return m.kind }

// Revision returns the current state-version counter. Increments on
// every mutating method (ShowFile, SetMarkers, Scroll*, SetSize, etc.).
func (m *Model) Revision() int { return m.revision }

func (m *Model) ShowEmpty() {
	m.saveScroll()
	m.kind = KindEmpty
	m.scroll = 0
	m.filePath = ""
	m.fileMarkers = nil
	m.revision++
}

// ShowFile installs already-highlighted output as the file view.
// content may contain ANSI escapes (one screen line per element of
// strings.Split(content, "\n")).
//
// Markers are kept across re-loads of the same path so a marker Cmd
// that resolved early (before the body Cmd) survives. They are dropped
// only when the path itself changes from one tracked file to another —
// the previous file's line numbering doesn't apply anymore.
func (m *Model) ShowFile(path, content, banner string) {
	m.saveScroll()
	if m.filePath != "" && m.filePath != path {
		m.fileMarkers = nil
	}
	m.kind = KindFile
	m.filePath = path
	m.fileBanner = banner
	m.fileLines = strings.Split(strings.TrimRight(content, "\n"), "\n")
	m.scroll = m.recallScroll(memKey{KindFile, path})
	m.err = nil
	m.revision++
}

// SetMarkers attaches the per-line change classification map. Caller
// (app layer) is responsible for staleness checking against its own
// activeFile bookkeeping — by the time this method is reached, the
// path-vs-active-file check has already passed.
//
// Storing markers before ShowFile arrives is intentionally allowed: a
// fast-resolving markers Cmd can populate m.fileMarkers and the next
// ShowFile (for the same path) will draw them instead of clearing.
// markers may be empty to indicate "tracked but no pending changes".
func (m *Model) SetMarkers(path string, markers map[int]diffview.ChangeKind) {
	m.fileMarkers = markers
	m.revision++
}

// ClearMarkers drops the gutter marker column entirely (non-git path,
// untracked file, or git-diff failure). Same caveat as SetMarkers re
// staleness: app layer pre-validates path.
func (m *Model) ClearMarkers(path string) {
	if m.fileMarkers == nil {
		return
	}
	m.fileMarkers = nil
	m.revision++
}

// ShowDiff installs a parsed unified diff. title is shown in a header
// row inside the pane.
func (m *Model) ShowDiff(title string, lines []diffview.Line) {
	m.saveScroll()
	m.kind = KindDiff
	m.diffTitle = title
	m.diffLines = lines
	m.scroll = m.recallScroll(memKey{KindDiff, title})
	m.err = nil
	m.revision++
}

func (m *Model) SetError(err error) {
	m.err = err
	m.revision++
}

func (m *Model) Scroll(delta int) {
	prev := m.scroll
	m.scroll = clamp(m.scroll+delta, 0, m.maxScroll())
	if prev != m.scroll {
		m.revision++
	}
}

func (m *Model) ScrollPage(delta int) {
	page := m.height
	if page <= 1 {
		page = 1
	} else {
		page--
	}
	m.Scroll(delta * page)
}

func (m *Model) ScrollHome() {
	if m.scroll != 0 {
		m.scroll = 0
		m.revision++
	}
}
func (m *Model) ScrollEnd() {
	end := m.maxScroll()
	if m.scroll != end {
		m.scroll = end
		m.revision++
	}
}

// ClampScroll re-pins scroll into [0, maxScroll] using the current m.height.
// Call after a window resize so the previous scroll offset doesn't point
// past the end of the (now smaller) visible window.
func (m *Model) ClampScroll() {
	prev := m.scroll
	m.scroll = clamp(m.scroll, 0, m.maxScroll())
	if prev != m.scroll {
		m.revision++
	}
}

func (m *Model) maxScroll() int {
	total := m.contentLen()
	if total <= m.height {
		return 0
	}
	return total - m.height
}

func (m *Model) contentLen() int {
	switch m.kind {
	case KindFile:
		return len(m.fileLines)
	case KindDiff, KindCommit:
		return len(m.diffLines) + 1 // +1 for title row
	}
	return 0
}

func (m *Model) saveScroll() {
	switch m.kind {
	case KindFile:
		m.memo[memKey{KindFile, m.filePath}] = m.scroll
	case KindDiff:
		m.memo[memKey{KindDiff, m.diffTitle}] = m.scroll
	}
}

func (m *Model) recallScroll(k memKey) int {
	if v, ok := m.memo[k]; ok {
		return v
	}
	return 0
}

// Render produces the body string. Caller wraps it in a bordered pane.
func (m *Model) Render(t theme.Theme, innerWidth, innerHeight int) string {
	m.width = innerWidth
	m.height = innerHeight
	if innerWidth <= 0 || innerHeight <= 0 {
		return ""
	}
	var out string
	switch {
	case m.err != nil:
		out = t.ErrorStyle().Render(m.err.Error())
	case m.kind == KindEmpty:
		out = t.DimStyle().Render("Select a file or change.")
	case m.kind == KindFile:
		out = m.renderFile(t, innerWidth, innerHeight)
	case m.kind == KindDiff, m.kind == KindCommit:
		out = m.renderDiff(t, innerWidth, innerHeight)
	}
	// Defensive clamp: every emitted line must be ≤ innerWidth, otherwise
	// the bordered pane wraps it and Bubble Tea trims the top of View().
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, innerWidth, "")
	}
	// Pin the result to exactly innerHeight rows. Lipgloss's Height(h)
	// pads when content is shorter, but extends past h when longer — a
	// single extra row makes this pane taller than the sibling sidebar
	// pane, breaking the JoinHorizontal alignment (the sidebar's bottom
	// border ends up one row above main's).
	if len(lines) > innerHeight {
		lines = lines[:innerHeight]
	}
	for len(lines) < innerHeight {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderFile(t theme.Theme, w, h int) string {
	if h <= 0 {
		return ""
	}
	header := t.DimStyle().Render(truncate(m.filePath, w))
	bodyRows := h - 1
	if m.fileBanner != "" {
		bodyRows--
	}
	if bodyRows < 1 {
		return header
	}
	gutterDigits := len(strconv.Itoa(len(m.fileLines)))
	if gutterDigits < 1 {
		gutterDigits = 1
	}
	markerCol := 0
	if m.fileMarkers != nil {
		markerCol = 1 // one cell for the change-marker bar
	}
	gutterW := markerCol + gutterDigits + 1 // [marker][digits][space]
	contentW := w - gutterW
	showGutter := contentW >= 1
	if !showGutter {
		contentW = w
	}

	gutterStyle := t.DimStyle()
	// Bold + LEFT HALF BLOCK (▌) so the marker fills more of the cell
	// than ▎ (one-quarter block) did and reads clearly even in dim
	// terminal palettes. The colours match the unified-diff view so a
	// reader recognises +/-/mod at a glance.
	addStyle := lipgloss.NewStyle().Foreground(t.DiffAdd).Bold(true)
	modStyle := lipgloss.NewStyle().Foreground(t.DiffHunk).Bold(true)
	delStyle := lipgloss.NewStyle().Foreground(t.DiffDel).Bold(true)
	emptyMarker := " "
	emptyGutter := strings.Repeat(" ", markerCol) + gutterStyle.Render(strings.Repeat(" ", gutterDigits)+" ")
	rows := make([]string, 0, bodyRows)
	for i, line := range slice(m.fileLines, m.scroll, len(m.fileLines)) {
		if len(rows) >= bodyRows {
			break
		}
		// Hard-wrap the (possibly ANSI-styled) line to contentW. Each
		// segment becomes its own visual row so long lines no longer get
		// silently truncated on the right edge.
		segments := strings.Split(ansi.Hardwrap(line, contentW, false), "\n")
		for j, seg := range segments {
			if len(rows) >= bodyRows {
				break
			}
			if showGutter {
				var prefix string
				if j == 0 {
					lineNo := m.scroll + i + 1
					marker := emptyMarker
					if markerCol > 0 {
						switch m.fileMarkers[lineNo] {
						case diffview.ChangeAdd:
							marker = addStyle.Render("▌")
						case diffview.ChangeMod:
							marker = modStyle.Render("▌")
						case diffview.ChangeDel:
							marker = delStyle.Render("▌")
						}
					}
					prefix = marker + gutterStyle.Render(fmt.Sprintf("%*d ", gutterDigits, lineNo))
				} else {
					prefix = emptyGutter
				}
				rows = append(rows, prefix+seg)
			} else {
				rows = append(rows, seg)
			}
		}
	}
	body := strings.Join(rows, "\n")

	if m.fileBanner != "" {
		banner := t.DimStyle().Render("· " + m.fileBanner)
		return lipgloss.JoinVertical(lipgloss.Left, header, banner, body)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

func (m *Model) renderDiff(t theme.Theme, w, h int) string {
	if h <= 0 {
		return ""
	}
	header := t.DimStyle().Render(truncate(m.diffTitle, w))
	bodyRows := h - 1
	if bodyRows < 1 {
		return header
	}

	rendered := renderDiffLines(t, m.diffLines, w, m.trueColor)
	rows := make([]string, 0, bodyRows)
	for _, line := range slice(rendered, m.scroll, len(rendered)) {
		if len(rows) >= bodyRows {
			break
		}
		for _, seg := range strings.Split(ansi.Hardwrap(line, w, false), "\n") {
			if len(rows) >= bodyRows {
				break
			}
			rows = append(rows, seg)
		}
	}
	body := strings.Join(rows, "\n")
	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

// diffGitRe pulls the b-side path from "diff --git a/<old> b/<new>".
// We use the new path because it's what exists on disk after the change.
var diffGitRe = regexp.MustCompile(`^diff --git a/.+ b/(.+)$`)

func renderDiffLines(t theme.Theme, lines []diffview.Line, w int, trueColor bool) []string {
	addStyle := lipgloss.NewStyle().Foreground(t.DiffAdd).Background(t.DiffAddBg)
	delStyle := lipgloss.NewStyle().Foreground(t.DiffDel).Background(t.DiffDelBg)
	hunkStyle := lipgloss.NewStyle().Foreground(t.DiffHunk).Bold(true)
	fileStyle := lipgloss.NewStyle().Foreground(t.Accent).Background(t.DiffFileBg).Bold(true)
	metaStyle := t.DimStyle()
	gutterStyle := t.DimStyle()

	sepWidth := w
	if sepWidth < 1 {
		sepWidth = 1
	}
	sep := metaStyle.Render(strings.Repeat("─", sepWidth))

	maxLN := 0
	for _, l := range lines {
		if l.OldLine > maxLN {
			maxLN = l.OldLine
		}
		if l.NewLine > maxLN {
			maxLN = l.NewLine
		}
	}
	digits := len(strconv.Itoa(maxLN))
	if digits < 1 {
		digits = 1
	}
	emptyCol := strings.Repeat(" ", digits)

	// hl is the per-file syntax highlighter. It rebuilds whenever a
	// LineFileHeader switches us into a new file's lexer. nil means
	// "no highlighting" (e.g. before the first file header).
	var hl func(string) string

	out := make([]string, len(lines))
	for i, l := range lines {
		var gutter string
		switch l.Kind {
		case diffview.LineAdd, diffview.LineDel, diffview.LineContext:
			old := emptyCol
			newCol := emptyCol
			if l.OldLine > 0 {
				old = fmt.Sprintf("%*d", digits, l.OldLine)
			}
			if l.NewLine > 0 {
				newCol = fmt.Sprintf("%*d", digits, l.NewLine)
			}
			gutter = gutterStyle.Render(old + " " + newCol + " ")
		}

		var body string
		switch l.Kind {
		case diffview.LineAdd:
			body = addStyle.Render(highlightDiffBody(l.Text, hl))
		case diffview.LineDel:
			body = delStyle.Render(highlightDiffBody(l.Text, hl))
		case diffview.LineContext:
			body = highlightDiffBody(l.Text, hl)
		case diffview.LineHunk:
			body = hunkStyle.Render(l.Text)
		case diffview.LineFileHeader:
			if m := diffGitRe.FindStringSubmatch(l.Text); m != nil {
				hl = highlight.NewLineHighlighter(m[1], trueColor, t.IsDark)
			} else {
				hl = nil
			}
			body = sep + "\n" + fileStyle.Render(l.Text)
		case diffview.LineMeta, diffview.LineBinary:
			body = metaStyle.Render(l.Text)
		default:
			body = l.Text
		}
		out[i] = gutter + body
	}
	return out
}

// highlightDiffBody runs the per-file lexer over a diff body row while
// preserving the leading +/-/space prefix unstyled (so the outer
// addStyle / delStyle paint it cleanly). The chroma reset \x1b[0m is
// rewritten to \x1b[39m so any outer Background survives the inner
// colour resets.
func highlightDiffBody(text string, hl func(string) string) string {
	if hl == nil || text == "" {
		return text
	}
	prefix := text[:1]
	rest := text[1:]
	highlighted := hl(rest)
	highlighted = strings.ReplaceAll(highlighted, "\x1b[0m", "\x1b[39m")
	return prefix + highlighted
}

func slice[T any](s []T, offset, length int) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(s) {
		return nil
	}
	end := offset + length
	if end > len(s) {
		end = len(s)
	}
	return s[offset:end]
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if len(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	// Show the trailing portion of long paths so the filename stays visible.
	return "…" + s[len(s)-(w-1):]
}

// titleFor builds a short title for a file diff, matching what we put
// in the memo key. Exposed so tests / app code can stay consistent.
func TitleFor(prefix, path string) string {
	if path == "" {
		return prefix
	}
	return fmt.Sprintf("%s · %s", prefix, path)
}

// FilePath / FileMarkers / FileLines expose the active file's
// bookkeeping for the app-level integration tests. Production code
// does not need them.
func (m *Model) FilePath() string                         { return m.filePath }
func (m *Model) FileMarkers() map[int]diffview.ChangeKind { return m.fileMarkers }
func (m *Model) FileLines() []string                      { return m.fileLines }
