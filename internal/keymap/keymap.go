package keymap

import tea "github.com/charmbracelet/bubbletea"

// Action is a high-level intent decoupled from concrete key bindings.
// Components consume actions, not raw KeyMsg, so v0.3 remapping is safe.
type Action int

const (
	ActionNone Action = iota
	ActionQuit
	ActionHelp
	ActionFocusToggle
	ActionUp
	ActionDown
	ActionLeft
	ActionRight
	ActionEnter
	ActionPageUp
	ActionPageDown
	ActionHome
	ActionEnd
)

func Resolve(msg tea.KeyMsg) Action {
	switch msg.String() {
	case "q", "ctrl+c":
		return ActionQuit
	// "?" is the post-shift character bubbletea emits when the user
	// presses Shift+/ (US/JIS layouts). Docs and on-screen hints label
	// the keystroke as "Shift+?" for clarity, but the event we actually
	// receive is the bare "?" — there is no separate "shift+?" string
	// from bubbletea for printable shifted characters.
	case "?":
		return ActionHelp
	case "tab":
		return ActionFocusToggle
	case "up", "k":
		return ActionUp
	case "down", "j":
		return ActionDown
	case "left", "h":
		return ActionLeft
	case "right", "l":
		return ActionRight
	case "enter":
		return ActionEnter
	case "pgup", "ctrl+b":
		return ActionPageUp
	case "pgdown", "ctrl+f", " ":
		return ActionPageDown
	case "g", "home":
		return ActionHome
	case "G", "end":
		return ActionEnd
	}
	return ActionNone
}
