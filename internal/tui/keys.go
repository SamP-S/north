package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap holds all key bindings used across the TUI. It excludes q, ctrl+c,
// tab, r, and ? — those are global meta-keys with their own modal-aware
// branching in Model.Update, handled by a plain switch on msg.String()
// rather than key.Matches.
type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Left   key.Binding
	Right  key.Binding
	Top    key.Binding
	Bottom key.Binding
	Enter  key.Binding
	Esc    key.Binding
	Create key.Binding
	Edit   key.Binding
	Move   key.Binding
	Sort   key.Binding
	State  key.Binding
	Delete key.Binding
	Search key.Binding
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
	Top: key.NewBinding(
		key.WithKeys("g", "home"),
		key.WithHelp("g", "top"),
	),
	Bottom: key.NewBinding(
		key.WithKeys("G", "end"),
		key.WithHelp("G", "bottom"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Esc: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel"),
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
		key.WithHelp("m", "set status"),
	),
	Sort: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "sort order"),
	),
	State: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "set state"),
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
