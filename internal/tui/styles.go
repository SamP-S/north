package tui

import "github.com/charmbracelet/lipgloss"

var (
	// status colours
	colorReady      = lipgloss.Color("12")  // bright blue
	colorInProgress = lipgloss.Color("11")  // bright yellow
	colorDone       = lipgloss.Color("10")  // bright green
	colorFailed     = lipgloss.Color("9")   // bright red
	colorBlocked    = lipgloss.Color("214") // orange
	colorDim        = lipgloss.Color("240") // dark grey

	// border colours
	colorActiveBorder   = lipgloss.Color("63")  // purple-blue
	colorInactiveBorder = lipgloss.Color("238") // dark grey

	// column card styles
	styleColumnActive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorActiveBorder)
	styleColumnInactive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorInactiveBorder)

	// task card inside a column
	styleCardSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorActiveBorder)
	styleCardNormal = lipgloss.NewStyle()
	// cards in non-focused board columns
	styleCardDim = lipgloss.NewStyle().Foreground(colorDim)

	// task ID — always dimmed
	styleID = lipgloss.NewStyle().Foreground(colorDim)

	// pane borders for the list view
	stylePaneActive = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorActiveBorder)
	stylePaneInactive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorInactiveBorder)

	// modal overlay (status picker, confirm)
	styleModal = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorActiveBorder).
			Padding(0, 1)

	// header / footer bars
	styleHeader = lipgloss.NewStyle().Bold(true)
	styleFooter = lipgloss.NewStyle().Foreground(colorDim)

	// status label colours
	styleStatusReady      = lipgloss.NewStyle().Foreground(colorReady)
	styleStatusInProgress = lipgloss.NewStyle().Foreground(colorInProgress)
	styleStatusDone       = lipgloss.NewStyle().Foreground(colorDone)
	styleStatusFailed     = lipgloss.NewStyle().Foreground(colorFailed)
	styleStatusBlocked    = lipgloss.NewStyle().Foreground(colorBlocked)

	// status-bar notice levels
	styleNoticeSuccess = lipgloss.NewStyle().Foreground(colorDone)
	styleNoticeWarn    = lipgloss.NewStyle().Foreground(colorInProgress)
	styleNoticeError   = lipgloss.NewStyle().Foreground(colorFailed).Bold(true)

	// help overlay
	styleHelp = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorDim).
			Padding(1, 2)
	styleHelpKey  = lipgloss.NewStyle().Foreground(colorActiveBorder).Bold(true)
	styleHelpDesc = lipgloss.NewStyle().Foreground(colorDim)
)

// noticeStyle returns the status-bar style for a notice level.
func noticeStyle(level noticeLevel) lipgloss.Style {
	switch level {
	case noticeSuccess:
		return styleNoticeSuccess
	case noticeWarn:
		return styleNoticeWarn
	case noticeError:
		return styleNoticeError
	default:
		return lipgloss.NewStyle()
	}
}

// statusStyle returns the lipgloss style for a given status label.
func statusStyle(s string) lipgloss.Style {
	switch s {
	case "ready":
		return styleStatusReady
	case "in_progress":
		return styleStatusInProgress
	case "done":
		return styleStatusDone
	case "failed":
		return styleStatusFailed
	case "blocked":
		return styleStatusBlocked
	default:
		return lipgloss.NewStyle()
	}
}
