package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/SamP-S/north/internal/models"
)

// theme groups every lipgloss style the TUI renders with. All render code
// reads from the package-level instance th; setTheme swaps the whole set.
type theme struct {
	// column card styles
	ColumnActive   lipgloss.Style
	ColumnInactive lipgloss.Style

	// task card / list row — selection is denoted by the ► prefix and bold
	// text only; colouring stays the same as unselected rows
	CardSelected lipgloss.Style
	CardNormal   lipgloss.Style

	// task ID — always dimmed
	ID lipgloss.Style

	// pane borders for the list view
	PaneActive   lipgloss.Style
	PaneInactive lipgloss.Style

	// modal overlay (status picker, confirm)
	Modal lipgloss.Style

	// header / footer bars
	Header lipgloss.Style
	Footer lipgloss.Style

	// state label colours (active stays the default foreground)
	StateDraft   lipgloss.Style
	StateArchive lipgloss.Style

	// status-bar notice levels
	NoticeSuccess lipgloss.Style
	NoticeWarn    lipgloss.Style
	NoticeError   lipgloss.Style

	// help overlay
	Help     lipgloss.Style
	HelpKey  lipgloss.Style
	HelpDesc lipgloss.Style

	// Status maps a status name to its label colour. A map so boards with
	// custom statuses can extend it later without a schema change.
	Status map[string]lipgloss.Style
}

// palette is the color slice of a theme; buildTheme applies it to the fixed
// border/bold/padding structure shared by every preset.
type palette struct {
	ready, inProgress, done, failed, blocked lipgloss.TerminalColor
	draft, dim                               lipgloss.TerminalColor
	activeBorder, inactiveBorder             lipgloss.TerminalColor
}

// defaultPalette inherits the terminal's palette: ANSI 0–15 only, so the
// user's terminal scheme is the theme.
func defaultPalette() palette {
	return palette{
		ready:        lipgloss.Color("12"), // bright blue
		inProgress:   lipgloss.Color("11"), // bright yellow
		done:         lipgloss.Color("10"), // bright green
		failed:       lipgloss.Color("9"),  // bright red
		blocked:      lipgloss.Color("13"), // bright magenta (16-color has no orange)
		draft:        lipgloss.Color("14"), // bright cyan
		activeBorder: lipgloss.Color("5"),  // magenta, distinct from status colors
		// ANSI 7 (light grey), not 8: many terminal schemes map bright black
		// at or near the background, which made dim text and unfocused
		// outlines invisible.
		dim:            lipgloss.Color("7"),
		inactiveBorder: lipgloss.Color("7"),
	}
}

// saturatedPalette is a fixed vivid truecolor palette, terminal-independent
// (termenv auto-degrades on non-truecolor terminals).
func saturatedPalette() palette {
	return palette{
		ready:          lipgloss.Color("#3b82f6"), // blue
		inProgress:     lipgloss.Color("#eab308"), // yellow
		done:           lipgloss.Color("#22c55e"), // green
		failed:         lipgloss.Color("#ef4444"), // red
		blocked:        lipgloss.Color("#f97316"), // orange
		draft:          lipgloss.Color("#06b6d4"), // cyan
		dim:            lipgloss.Color("#6b7280"), // grey
		activeBorder:   lipgloss.Color("#8b5cf6"), // violet
		inactiveBorder: lipgloss.Color("#4b5563"), // dark grey
	}
}

// highContrastPalette uses ANSI brights only, with no dim greys anywhere:
// everything dim in default keeps the terminal's default foreground instead.
func highContrastPalette() palette {
	return palette{
		ready:      lipgloss.Color("12"), // bright blue
		inProgress: lipgloss.Color("11"), // bright yellow
		done:       lipgloss.Color("10"), // bright green
		failed:     lipgloss.Color("9"),  // bright red
		blocked:    lipgloss.Color("13"), // bright magenta
		draft:      lipgloss.Color("14"), // bright cyan
		dim:        lipgloss.NoColor{},   // default foreground, never dimmed
		// bright magenta: inactive borders sit at the default foreground
		// (no dim allowed), so focus needs a real color, not white/black —
		// magenta keeps the border identity of the other presets.
		activeBorder:   lipgloss.Color("13"),
		inactiveBorder: lipgloss.NoColor{}, // default foreground
	}
}

// buildTheme applies a palette to the shared style structure.
func buildTheme(p palette) theme {
	return theme{
		ColumnActive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.activeBorder),
		ColumnInactive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.inactiveBorder),

		CardSelected: lipgloss.NewStyle().Bold(true),
		CardNormal:   lipgloss.NewStyle(),

		ID: lipgloss.NewStyle().Foreground(p.dim),

		PaneActive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.activeBorder),
		PaneInactive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.inactiveBorder),

		Modal: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.activeBorder).
			Padding(0, 1),

		Header: lipgloss.NewStyle().Bold(true),
		Footer: lipgloss.NewStyle().Foreground(p.dim),

		StateDraft:   lipgloss.NewStyle().Foreground(p.draft),
		StateArchive: lipgloss.NewStyle().Foreground(p.dim),

		NoticeSuccess: lipgloss.NewStyle().Foreground(p.done),
		NoticeWarn:    lipgloss.NewStyle().Foreground(p.inProgress),
		NoticeError:   lipgloss.NewStyle().Foreground(p.failed).Bold(true),

		Help: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.dim).
			Padding(1, 2),
		HelpKey:  lipgloss.NewStyle().Foreground(p.activeBorder).Bold(true),
		HelpDesc: lipgloss.NewStyle().Foreground(p.dim),

		Status: map[string]lipgloss.Style{
			"ready":       lipgloss.NewStyle().Foreground(p.ready),
			"in_progress": lipgloss.NewStyle().Foreground(p.inProgress),
			"done":        lipgloss.NewStyle().Foreground(p.done),
			"failed":      lipgloss.NewStyle().Foreground(p.failed),
			"blocked":     lipgloss.NewStyle().Foreground(p.blocked),
		},
	}
}

// defaultTheme returns the terminal-palette preset.
func defaultTheme() theme { return buildTheme(defaultPalette()) }

// th is the active theme; all render code reads from it.
var th = defaultTheme()

// setTheme installs the named preset into th. An empty name or "default"
// selects the default preset; an unknown name installs the default and
// returns a warning for the status bar (the caller prefixes file context).
func setTheme(name string) (warning string) {
	switch strings.TrimSpace(name) {
	case "", "default":
		th = defaultTheme()
	case "saturated":
		th = buildTheme(saturatedPalette())
	case "high-contrast":
		th = buildTheme(highContrastPalette())
	default:
		th = defaultTheme()
		return fmt.Sprintf("unknown theme %q, using default", name)
	}
	return ""
}

// noticeStyle returns the status-bar style for a notice level.
func noticeStyle(level noticeLevel) lipgloss.Style {
	switch level {
	case noticeSuccess:
		return th.NoticeSuccess
	case noticeWarn:
		return th.NoticeWarn
	case noticeError:
		return th.NoticeError
	default:
		return lipgloss.NewStyle()
	}
}

// stateStyle returns the lipgloss style for a task state label.
func stateStyle(s models.TaskState) lipgloss.Style {
	switch s {
	case models.StateDraft:
		return th.StateDraft
	case models.StateArchive:
		return th.StateArchive
	default:
		return lipgloss.NewStyle()
	}
}

// statusStyle returns the lipgloss style for a given status label.
func statusStyle(s string) lipgloss.Style {
	if st, ok := th.Status[s]; ok {
		return st
	}
	return lipgloss.NewStyle()
}
