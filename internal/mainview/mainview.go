// Package mainview renders the right-hand pane: file body, diff, or
// (v0.2) a specific commit's patch.
//
// The view holds rendered text only. It does not perform I/O; the app
// layer feeds it via Set* methods after Cmds resolve. Per Design.md
// §6.2, scroll positions are remembered per (kind, identifier).
package mainview

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/gen-hiroto0119/sus4/internal/diffview"
	"github.com/gen-hiroto0119/sus4/internal/theme"
)

type Kind int

const (
	KindEmpty Kind = iota
	KindFile
	KindDiff
	KindCommit
)

type Model struct {
	kind Kind

	// File content: highlighted lines (ANSI-coded).
	filePath   string
	fileLines  []string
	fileBanner string

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
}

type memKey struct {
	kind Kind
	id   string
}

func New() Model {
	return Model{kind: KindEmpty, memo: map[memKey]int{}}
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m *Model) Kind() Kind { return m.kind }

func (m *Model) ShowEmpty() {
	m.saveScroll()
	m.kind = KindEmpty
	m.scroll = 0
}

// ShowFile installs already-highlighted output as the file view.
// content may contain ANSI escapes (one screen line per element of
// strings.Split(content, "\n")).
func (m *Model) ShowFile(path, content, banner string) {
	m.saveScroll()
	m.kind = KindFile
	m.filePath = path
	m.fileBanner = banner
	m.fileLines = strings.Split(strings.TrimRight(content, "\n"), "\n")
	m.scroll = m.recallScroll(memKey{KindFile, path})
	m.err = nil
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
}

func (m *Model) SetError(err error) { m.err = err }

func (m *Model) Scroll(delta int) {
	m.scroll = clamp(m.scroll+delta, 0, m.maxScroll())
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

func (m *Model) ScrollHome() { m.scroll = 0 }
func (m *Model) ScrollEnd()  { m.scroll = m.maxScroll() }

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
	gutterW := gutterDigits + 1 // trailing space
	contentW := w - gutterW
	showGutter := contentW >= 1
	if !showGutter {
		contentW = w
	}

	gutterStyle := t.DimStyle()
	visible := slice(m.fileLines, m.scroll, bodyRows)
	clipped := make([]string, len(visible))
	for i, line := range visible {
		// Hard-truncate to contentW visible cols. Without this, Lipgloss
		// wraps long lines inside the bordered pane, the body grows past
		// h, and Bubble Tea's renderer trims the *top* of the View output
		// to fit (standard_renderer.go ~L186), erasing the top border.
		text := ansi.Truncate(line, contentW, "")
		if showGutter {
			lineNo := m.scroll + i + 1
			clipped[i] = gutterStyle.Render(fmt.Sprintf("%*d ", gutterDigits, lineNo)) + text
		} else {
			clipped[i] = text
		}
	}
	body := strings.Join(clipped, "\n")

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

	rendered := renderDiffLines(t, m.diffLines)
	visible := slice(rendered, m.scroll, bodyRows)
	clipped := make([]string, len(visible))
	for i, line := range visible {
		clipped[i] = ansi.Truncate(line, w, "")
	}
	body := strings.Join(clipped, "\n")
	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

func renderDiffLines(t theme.Theme, lines []diffview.Line) []string {
	out := make([]string, len(lines))
	addStyle := lipgloss.NewStyle().Foreground(t.DiffAdd)
	delStyle := lipgloss.NewStyle().Foreground(t.DiffDel)
	hunkStyle := lipgloss.NewStyle().Foreground(t.DiffHunk).Bold(true)
	metaStyle := t.DimStyle()
	for i, l := range lines {
		switch l.Kind {
		case diffview.LineAdd:
			out[i] = addStyle.Render(l.Text)
		case diffview.LineDel:
			out[i] = delStyle.Render(l.Text)
		case diffview.LineHunk:
			out[i] = hunkStyle.Render(l.Text)
		case diffview.LineFileHeader:
			out[i] = lipgloss.NewStyle().Bold(true).Render(l.Text)
		case diffview.LineMeta, diffview.LineBinary:
			out[i] = metaStyle.Render(l.Text)
		default:
			out[i] = l.Text
		}
	}
	return out
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
