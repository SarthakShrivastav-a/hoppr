package tui

import "github.com/charmbracelet/lipgloss"

// The palette is defined with lipgloss.AdaptiveColor so the UI is legible on a
// light terminal as well as a dark one. Hardcoding a dark palette is the single
// most common way a TUI ends up unreadable for half its users, and the author
// never sees it because their own terminal is dark.
var (
	colFg      = lipgloss.AdaptiveColor{Light: "236", Dark: "252"}
	colMuted   = lipgloss.AdaptiveColor{Light: "245", Dark: "243"}
	colFaint   = lipgloss.AdaptiveColor{Light: "250", Dark: "238"}
	colAccent  = lipgloss.AdaptiveColor{Light: "27", Dark: "111"}
	colUp      = lipgloss.AdaptiveColor{Light: "28", Dark: "78"}
	colDown    = lipgloss.AdaptiveColor{Light: "160", Dark: "203"}
	colWarn    = lipgloss.AdaptiveColor{Light: "130", Dark: "215"}
	colMatch   = lipgloss.AdaptiveColor{Light: "202", Dark: "215"}
	colSelBg   = lipgloss.AdaptiveColor{Light: "254", Dark: "237"}
	colBorder  = lipgloss.AdaptiveColor{Light: "252", Dark: "238"}
	colInverse = lipgloss.AdaptiveColor{Light: "255", Dark: "232"}
)

var (
	// Chrome.
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colInverse).
			Background(colAccent).
			Padding(0, 1)

	subtitleStyle = lipgloss.NewStyle().Foreground(colMuted)

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colBorder)

	// List rows.
	rowStyle = lipgloss.NewStyle().Foreground(colFg)

	selectedRowStyle = lipgloss.NewStyle().
				Foreground(colFg).
				Background(colSelBg).
				Bold(true)

	aliasStyle     = lipgloss.NewStyle().Foreground(colFg).Bold(true)
	targetStyle    = lipgloss.NewStyle().Foreground(colMuted)
	matchStyle     = lipgloss.NewStyle().Foreground(colMatch).Bold(true)
	tagStyle       = lipgloss.NewStyle().Foreground(colAccent)
	activeTagStyle = lipgloss.NewStyle().
			Foreground(colInverse).
			Background(colAccent).
			Padding(0, 1)
	inactiveTagStyle = lipgloss.NewStyle().
				Foreground(colMuted).
				Padding(0, 1)

	// Status.
	upStyle      = lipgloss.NewStyle().Foreground(colUp)
	downStyle    = lipgloss.NewStyle().Foreground(colDown)
	warnStyle    = lipgloss.NewStyle().Foreground(colWarn)
	unknownStyle = lipgloss.NewStyle().Foreground(colFaint)

	// Detail pane.
	labelStyle = lipgloss.NewStyle().Foreground(colMuted).Width(14)
	valueStyle = lipgloss.NewStyle().Foreground(colFg)

	// Footer and messages.
	helpStyle   = lipgloss.NewStyle().Foreground(colMuted)
	keyStyle    = lipgloss.NewStyle().Foreground(colFg).Bold(true)
	errStyle    = lipgloss.NewStyle().Foreground(colDown).Bold(true)
	okStyle     = lipgloss.NewStyle().Foreground(colUp)
	promptStyle = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
)

// statusDot renders a reachability indicator.
//
// The glyph carries the meaning as well as the color, so the display still works
// for a colorblind user and in a terminal with a mangled palette. "Unknown" is
// visually distinct from "down" on purpose: a manager that renders "not checked
// yet" identically to "unreachable" teaches you to ignore the column entirely.
func statusDot(s probeState) string {
	switch s {
	case stateUp:
		return upStyle.Render("●")
	case stateDown:
		return downStyle.Render("○")
	case stateChecking:
		return unknownStyle.Render("◌")
	default:
		return unknownStyle.Render("·")
	}
}
