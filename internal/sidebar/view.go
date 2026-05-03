package sidebar

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/gen-hiroto0119/sus4/internal/filetree"
	"github.com/gen-hiroto0119/sus4/internal/git"
	"github.com/gen-hiroto0119/sus4/internal/icons"
	"github.com/gen-hiroto0119/sus4/internal/theme"
)

// Render produces the sidebar body. The caller wraps it in a bordered pane.
func (m *Model) Render(t theme.Theme, focused bool, innerWidth, innerHeight int) string {
	if innerWidth <= 0 || innerHeight <= 0 {
		return ""
	}

	header := m.renderHeader(t, focused, innerWidth)
	body := m.renderBody(t, innerWidth, innerHeight-1)

	if m.err != nil {
		body = t.ErrorStyle().Render(m.err.Error())
	}

	// Defensive clamp: every line must be ≤ innerWidth so the bordered
	// pane outside doesn't wrap and overflow its requested height.
	out := lipgloss.JoinVertical(lipgloss.Left, header, body)
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, innerWidth, "")
	}
	// Pin to exactly innerHeight rows so the sibling pane stays aligned
	// — lipgloss extends past Height(h) when content is longer.
	if len(lines) > innerHeight {
		lines = lines[:innerHeight]
	}
	for len(lines) < innerHeight {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderHeader(t theme.Theme, focused bool, w int) string {
	label := "files"
	if m.mode == ModeChanges {
		label = "changes"
	}
	hint := "←/→ switch"
	style := t.DimStyle()
	if focused {
		style = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	}
	left := style.Render(label)
	right := t.DimStyle().Render(hint)

	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) renderBody(t theme.Theme, w, h int) string {
	if h <= 0 {
		return ""
	}
	rows := m.rows
	cursor := m.CursorIndex()

	if m.mode == ModeChanges && !m.repoOK {
		return t.DimStyle().Render("Not a git repository")
	}
	if len(rows) == 0 {
		if m.mode == ModeTree && m.rootChildren == nil {
			return t.DimStyle().Render("loading…")
		}
		if m.mode == ModeChanges {
			return t.DimStyle().Render("(working tree clean)")
		}
		return t.DimStyle().Render("(empty)")
	}

	first, last := visibleWindow(cursor, len(rows), h)
	var b strings.Builder
	for i := first; i < last; i++ {
		line := m.renderRow(t, rows[i], i == cursor, w)
		b.WriteString(line)
		if i < last-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m *Model) renderRow(t theme.Theme, r row, selected bool, w int) string {
	var prefix, name string
	switch {
	case r.tree != nil:
		expanded := r.tree.node.Kind == filetree.NodeDir && m.expanded[r.tree.node.Path]
		prefix, name = treeRowParts(*r.tree, expanded, m.iconsOn)
	case r.change != nil:
		prefix, name = changeRowParts(*r.change, m.iconsOn)
	}
	// Reserve one extra cell of margin when icons are on. Nerd Font PUA
	// glyphs are *counted* as 1 cell by ansi/uniseg but several fonts
	// render them at 2 — a 1-cell mismatch is enough to push the line
	// past innerW visually and corrupt the layout below.
	safeW := w
	if m.iconsOn && safeW > 1 {
		safeW--
	}
	full := truncate(prefix+name, safeW)
	if selected {
		// Full-row tinted background bar. iconGlyph emits a
		// foreground-only reset so the outer Background extends through
		// the icon position without a visible gap.
		return t.SelectedStyle().Width(w).Render(full)
	}
	return full
}

// treeRowParts splits a tree row into the prefix (indent + arrow + icon)
// and the filename. Selection styling can be applied to the prefix only,
// leaving the filename on its native background.
func treeRowParts(tr treeRow, expanded, withIcon bool) (prefix, name string) {
	indent := strings.Repeat("  ", tr.depth)
	if tr.node.Kind == filetree.NodeTruncated {
		return indent, fmt.Sprintf("… (+%d more)", tr.node.HiddenCount)
	}
	expandArrow := "  "
	if tr.node.Kind == filetree.NodeDir {
		if expanded {
			expandArrow = "▾ "
		} else {
			expandArrow = "▸ "
		}
	}
	if !withIcon {
		return indent + expandArrow, tr.node.Name
	}
	return indent + expandArrow + iconGlyph(icons.For(tr.node, expanded)) + " ", tr.node.Name
}

func changeRowParts(cr changeRow, withIcon bool) (prefix, name string) {
	if !withIcon {
		return statusGlyph(cr.entry.Kind) + "  ", cr.entry.Path
	}
	node := filetree.Node{Name: filepath.Base(cr.entry.Path), Kind: filetree.NodeFile}
	return fmt.Sprintf("%s %s ", statusGlyph(cr.entry.Kind), iconGlyph(icons.For(node, false))), cr.entry.Path
}

// iconGlyph styles the icon with its colour but emits a foreground-only
// reset (\x1b[39m) at the end instead of a full SGR reset (\x1b[0m). The
// full reset would clear an outer background mid-line, leaving the
// selection bar fragmented at the icon position; the foreground-only
// reset lets the outer background extend continuously across the prefix.
func iconGlyph(ic icons.Icon) string {
	glyph := lipgloss.NewStyle().Foreground(ic.Color).Render(ic.Glyph)
	return strings.ReplaceAll(glyph, "\x1b[0m", "\x1b[39m")
}

func statusGlyph(k git.StatusKind) string {
	switch k {
	case git.StatusModified:
		return "M"
	case git.StatusAdded:
		return "A"
	case git.StatusDeleted:
		return "D"
	case git.StatusRenamed:
		return "R"
	case git.StatusUntracked:
		return "?"
	case git.StatusUnmerged:
		return "U"
	}
	return " "
}

// visibleWindow returns [first, last) such that the cursor stays in
// view. We keep it dead simple: a sliding window pinned to the cursor.
func visibleWindow(cursor, total, height int) (int, int) {
	if total <= height {
		return 0, total
	}
	first := cursor - height/2
	if first < 0 {
		first = 0
	}
	last := first + height
	if last > total {
		last = total
		first = last - height
	}
	return first, last
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	// ANSI-aware visual-width truncation. We cannot use byte length: a
	// row wider than w (e.g. multi-byte filenames or long deeply-nested
	// paths) would cause Lipgloss to wrap inside the bordered pane and
	// blow past the requested height — Bubble Tea then trims the top of
	// the View, hiding the top border.
	return ansi.Truncate(s, w, "…")
}
