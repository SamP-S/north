package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap holds all key bindings used across the TUI.
type keyMap struct {
	Up      key.Binding
	Down    key.Binding
	Left    key.Binding
	Right   key.Binding
	Tab     key.Binding
	Enter   key.Binding
	Esc     key.Binding
	Quit    key.Binding
	Help    key.Binding
	Create  key.Binding
	Edit    key.Binding
	Move    key.Binding
	Promote key.Binding
	Archive key.Binding
	Delete  key.Binding
	Search  key.Binding
}

// keys is the global key map used throughout the TUI.
var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "left"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "right"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch view"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Esc: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Create: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "create task"),
	),
	Edit: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "edit task"),
	),
	Move: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "move status"),
	),
	Promote: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "promote/demote"),
	),
	Archive: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "archive/restore"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
}
