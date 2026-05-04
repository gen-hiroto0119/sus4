package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Theme drives every coloured surface in the UI. IsDark mirrors the
// background mode the colour set is tuned for, so call sites that need
// to pick a *different* palette (e.g. chroma syntax styles, glamour
// markdown renderer) can branch off it without re-detecting.
type Theme struct {
	IsDark         bool
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

// ByName resolves a config-supplied theme name to a Theme value.
//
// Recognised values:
//   - "auto" / "" — query the terminal's background via OSC 11 (termenv)
//     and pick Default (dark) or Light accordingly.
//   - "dark" / "default" — force the dark palette.
//   - "light" — force the light palette.
//
// Anything else falls back to Default. Detection runs once per call, so
// callers should cache the result for the session.
func ByName(name string) Theme {
	switch strings.ToLower(name) {
	case "light":
		return Light()
	case "dark", "default":
		return Default()
	case "auto", "":
		return AutoDetect()
	}
	return Default()
}

// AutoDetect queries the host terminal's background colour and returns
// the matching theme. Falls back to dark when the query fails — that is
// the safer default for code-on-screen scenarios.
func AutoDetect() Theme {
	if termenv.HasDarkBackground() {
		return Default()
	}
	return Light()
}

func Default() Theme {
	return Theme{
		IsDark:        true,
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

// Light is the daylight counterpart to Default. The palette assumes a
// near-white terminal background; selection / diff bars use very pale
// tints so they sit gently on the bright field instead of punching out.
func Light() Theme {
	return Theme{
		IsDark:        false,
		Background:    lipgloss.Color("255"),
		Foreground:    lipgloss.Color("232"),
		Dim:           lipgloss.Color("245"),
		Border:        lipgloss.Color("250"),
		BorderFocused: lipgloss.Color("33"),
		Accent:        lipgloss.Color("33"),
		Selected:      lipgloss.Color("232"),
		SelectedBg:    lipgloss.Color("254"),
		DiffAdd:       lipgloss.Color("28"),
		DiffAddBg:     lipgloss.Color("194"), // very light green
		DiffDel:       lipgloss.Color("124"),
		DiffDelBg:     lipgloss.Color("224"), // very light red/pink
		DiffHunk:      lipgloss.Color("62"),
		DiffFileBg:    lipgloss.Color("253"),
		StatusBar:     lipgloss.Color("232"),
		StatusBarBg:   lipgloss.Color("253"),
		Error:         lipgloss.Color("160"),
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
