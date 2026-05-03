package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	statusBarHeight = 1
	sidebarPercent  = 30
	sidebarMinWidth = 18
	sidebarMaxWidth = 40
	// Below mainOnlyWidth the sidebar is dropped entirely (Design.md §13).
	mainOnlyWidth = 60
)

func (m Model) View() string {
	if m.width < 10 || m.height < 3 {
		return "resize…"
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
	left := fmt.Sprintf("sus4  •  %s", focusName(m.focus))
	if m.err != nil {
		left = m.theme.ErrorStyle().Render(m.err.Error())
	}
	right := "?: help   tab: focus   q: quit"
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
	return m.theme.StatusStyle().Width(m.width).Render(line)
}

func focusName(f Focus) string {
	if f == FocusSidebar {
		return "sidebar"
	}
	return "main"
}
