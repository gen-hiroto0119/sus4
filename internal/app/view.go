package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	statusBarHeight = 1
	sidebarPercent  = 30
	sidebarMinWidth = 18
	sidebarMaxWidth = 40
	// Below mainOnlyWidth the sidebar is dropped entirely (Design.md §13).
	mainOnlyWidth = 60
)

// viewCache memoises the last-produced View() string so a fs-event
// burst (which forces an Update + View pass per event) doesn't re-run
// lipgloss border + ansi.StringWidth + chroma-output hardwrap on every
// frame. The cache is keyed on every input that View consumes — see
// viewCacheKey below — and lives behind a pointer so the value-receiver
// View() can mutate it.
//
// CPU profiling on the synthetic burst bench (see cpu_probe_test.go)
// pinpointed lipgloss.applyBorder and ansi.StringWidth as 35–40% of
// CPU once the os.Stat regression had been removed; this cache turns
// the steady-state cost from "redraw the whole pane each event" into
// "compare a few ints".
type viewCache struct {
	out string
	key viewCacheKey
}

type viewCacheKey struct {
	width      int
	height     int
	focus      Focus
	helpOpen   bool
	sidebarRev int
	mainRev    int
	errMsg     string
}

func (m Model) viewKey() viewCacheKey {
	errMsg := ""
	if m.err != nil {
		errMsg = m.err.Error()
	}
	return viewCacheKey{
		width:      m.width,
		height:     m.height,
		focus:      m.focus,
		helpOpen:   m.helpOpen,
		sidebarRev: m.sidebar.Revision(),
		mainRev:    m.main.Revision(),
		errMsg:     errMsg,
	}
}

func (m Model) View() string {
	if m.width < 10 || m.height < 3 {
		return "resize…"
	}

	if m.cache != nil && m.cache.out != "" {
		key := m.viewKey()
		if m.cache.key == key {
			return m.cache.out
		}
	}

	out := m.renderView()

	if m.cache != nil {
		m.cache.out = out
		m.cache.key = m.viewKey()
	}
	return out
}

// renderView is the previous View body. Split out so the cache layer
// in View can dispatch to it on a miss.
func (m Model) renderView() string {
	if m.helpOpen {
		return m.renderHelp()
	}

	var body string
	if m.width < mainOnlyWidth {
		body = m.renderMainPane()
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			m.renderSidebarPane(),
			m.renderMainPane(),
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, m.renderStatus())
}

// renderHelp paints a centered modal-style cheat sheet that takes over
// the whole screen while m.helpOpen is true. It is intentionally not an
// overlay: lipgloss has no real layering, so a full-screen takeover is
// the simplest path that keeps the layout pinned to (m.width, m.height).
func (m Model) renderHelp() string {
	rows := []string{
		"tetra — keymap",
		"",
		"  Tab                       Switch focus (sidebar ⇄ main)",
		"  ←  →    / h  l            Sidebar mode (files ⇄ changes)",
		"  ↑  ↓    / k  j            Move / scroll line",
		"  PgUp    / Ctrl-B          Scroll page up",
		"  PgDn    / Ctrl-F / Space  Scroll page down",
		"  g       / Home            Jump to top",
		"  G       / End             Jump to bottom",
		"  Enter                     Open file / expand directory",
		"  Shift+?                   Toggle this help",
		"  q       / Ctrl-C          Quit",
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.BorderFocused).
		Padding(1, 2).
		Foreground(m.theme.Foreground).
		Render(strings.Join(rows, "\n"))
	body := lipgloss.Place(m.width, m.height-statusBarHeight,
		lipgloss.Center, lipgloss.Center, box)
	return lipgloss.JoinVertical(lipgloss.Left, body, m.renderStatus())
}

func (m Model) sidebarWidth() int {
	w := m.width * sidebarPercent / 100
	if w < sidebarMinWidth {
		w = sidebarMinWidth
	}
	if w > sidebarMaxWidth {
		w = sidebarMaxWidth
	}
	if w > m.width-20 {
		w = m.width - 20
	}
	return w
}

func (m Model) bodyHeight() int {
	h := m.height - statusBarHeight
	if h < 1 {
		h = 1
	}
	return h
}

func (m Model) renderSidebarPane() string {
	w := m.sidebarWidth()
	h := m.bodyHeight()
	innerW := w - 2
	innerH := h - 2
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}
	body := m.sidebar.Render(m.theme, m.focus == FocusSidebar, innerW, innerH)
	return m.theme.PaneBorder(m.focus == FocusSidebar).
		Width(innerW).Height(innerH).Render(body)
}

func (m Model) renderMainPane() string {
	w := m.width
	if m.width >= mainOnlyWidth {
		w = m.width - m.sidebarWidth()
	}
	h := m.bodyHeight()
	innerW := w - 2
	innerH := h - 2
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}
	body := m.main.Render(m.theme, innerW, innerH)
	return m.theme.PaneBorder(m.focus == FocusMain).
		Width(innerW).Height(innerH).Render(body)
}

func (m Model) renderStatus() string {
	left := fmt.Sprintf("tetra  •  %s", focusName(m.focus))
	if m.err != nil {
		left = m.theme.ErrorStyle().Render(m.err.Error())
	}
	right := "Shift+?: help   tab: focus   q: quit"
	// Reserve 1 col of breathing room on each edge so the bar doesn't read
	// as flush against the terminal border.
	inner := m.width - 2
	if inner < 1 {
		inner = 1
	}
	gap := inner - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	line := " " + left + strings.Repeat(" ", gap) + right + " "
	// Hard-clamp to m.width: at narrow widths left+right can exceed the
	// terminal, and StatusStyle.Width(m.width).Render() would wrap into a
	// second row. The body above is sized assuming statusBarHeight==1, so
	// any wrap pushes total height past m.height and Bubble Tea trims the
	// top of View — manifesting as "上部が見切れる".
	line = ansi.Truncate(line, m.width, "")
	return m.theme.StatusStyle().Width(m.width).Render(line)
}

func focusName(f Focus) string {
	if f == FocusSidebar {
		return "sidebar"
	}
	return "main"
}
