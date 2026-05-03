package theme

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	Background     lipgloss.Color
	Foreground     lipgloss.Color
	Dim            lipgloss.Color
	Border         lipgloss.Color
	BorderFocused  lipgloss.Color
	Accent         lipgloss.Color
	Selected       lipgloss.Color
	SelectedBg     lipgloss.Color
	DiffAdd        lipgloss.Color
	DiffAddBg      lipgloss.Color
	DiffDel        lipgloss.Color
	DiffDelBg      lipgloss.Color
	DiffHunk       lipgloss.Color
	DiffFileBg     lipgloss.Color
	StatusBar      lipgloss.Color
	StatusBarBg    lipgloss.Color
	Error          lipgloss.Color
}

// ByName returns the theme matching name. Unknown names — including the
// empty string — fall back to Default(). v0.1 ships only "default"; the
// switch is the seam future themes plug into.
func ByName(name string) Theme {
	switch name {
	default:
		return Default()
	}
}

func Default() Theme {
	return Theme{
		Background:    lipgloss.Color("0"),
		Foreground:    lipgloss.Color("252"),
		Dim:           lipgloss.Color("242"),
		Border:        lipgloss.Color("238"),
		BorderFocused: lipgloss.Color("99"),
		Accent:        lipgloss.Color("105"),
		Selected:      lipgloss.Color("231"),
		SelectedBg:    lipgloss.Color("237"),
		DiffAdd:       lipgloss.Color("34"),
		DiffAddBg:     lipgloss.Color("22"),  // dim green tint
		DiffDel:       lipgloss.Color("160"),
		DiffDelBg:     lipgloss.Color("52"),  // dim red tint
		DiffHunk:      lipgloss.Color("105"),
		DiffFileBg:    lipgloss.Color("236"), // bar behind file headers
		StatusBar:     lipgloss.Color("252"),
		StatusBarBg:   lipgloss.Color("236"),
		Error:         lipgloss.Color("203"),
	}
}

func (t Theme) PaneBorder(focused bool) lipgloss.Style {
	c := t.Border
	if focused {
		c = t.BorderFocused
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c)
}

// StatusStyle is intentionally padding-free: Lipgloss adds padding outside
// the width set via .Width(), so combining Padding with .Width(m.width)
// produces a line m.width+2 cols wide, which wraps and steals a line from
// the body above. Callers handle edge spacing inside the rendered string.
func (t Theme) StatusStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(t.StatusBar).
		Background(t.StatusBarBg)
}

func (t Theme) DimStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Dim)
}

func (t Theme) ErrorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Error).Bold(true)
}

// SelectedStyle marks the active tree row with a darker background only.
// Foreground and weight are deliberately left alone so each row's own
// styling (icon color, dim filenames, etc.) bleeds through — the bar of
// background colour is enough of a cursor cue without recolouring text.
func (t Theme) SelectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(t.SelectedBg)
}
