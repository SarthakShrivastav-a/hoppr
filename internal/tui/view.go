package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/SarthakShrivastav-a/hoppr/internal/history"
	"github.com/SarthakShrivastav-a/hoppr/internal/host"
)

// Layout thresholds.
//
// Below compactWidth the detail column is dropped rather than squeezed; below
// tinyWidth even the target is dropped and only the alias remains.
//
// What never happens is a refusal to render. "Terminal too small, please
// resize" is a real behaviour in this category and it is filed as a bug every
// time: it breaks tmux splits, i3 tiles, and the VS Code panel, all of which are
// where people actually live.
const (
	compactWidth = 90
	tinyWidth    = 46
	minListRows  = 3
)

// View renders the whole frame.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	switch m.view {
	case viewForm:
		return m.viewForm()
	case viewImport:
		return m.viewImport()
	case viewKeys:
		return m.viewKeys()
	case viewDoctor:
		return m.viewDoctor()
	case viewConfirm:
		return m.viewConfirm()
	case viewHelp:
		return m.viewHelp()
	default:
		return m.viewList()
	}
}

// ---- Host list ----

func (m Model) viewList() string {
	var b strings.Builder

	b.WriteString(m.header())
	b.WriteByte('\n')

	if m.tagMode {
		b.WriteString(m.tagBar())
		b.WriteByte('\n')
	}

	rows := m.listHeight()

	switch {
	case len(m.hosts) == 0:
		b.WriteString(m.emptyState())
	case len(m.matches) == 0:
		b.WriteString(subtitleStyle.Render("\n  no host matches that filter\n"))
	default:
		end := min(m.offset+rows, len(m.matches))
		for i := m.offset; i < end; i++ {
			b.WriteString(m.row(m.matches[i], i == m.cursor))
			b.WriteByte('\n')
		}
	}

	b.WriteString(m.footer())
	return b.String()
}

// row renders one host.
func (m Model) row(match host.Match, selected bool) string {
	h := match.Host

	cursor := "  "
	if selected {
		cursor = promptStyle.Render("▸ ")
	}

	dot := statusDot(m.status[h.Alias])

	// Highlight the characters the fuzzy match actually hit, but only when the
	// match was against the alias — highlighting positions from a tag match
	// against the alias text would light up the wrong letters.
	alias := h.Alias
	if match.Field == "alias" && len(match.Positions) > 0 {
		alias = highlight(alias, match.Positions)
	} else {
		alias = aliasStyle.Render(alias)
	}

	line := cursor + dot + " " + pad(alias, m.aliasWidth())

	if m.width >= tinyWidth {
		line += " " + targetStyle.Render(truncate(h.Target(), m.targetWidth()))
	}
	if m.width >= compactWidth {
		line += "  " + m.rowMeta(h)
	}

	style := rowStyle
	if selected {
		style = selectedRowStyle
	}
	return style.Width(m.width).Render(line)
}

// rowMeta is the right-hand column: tags, then last-used.
func (m Model) rowMeta(h *host.Host) string {
	var parts []string

	if tags := h.Tags(); len(tags) > 0 {
		parts = append(parts, tagStyle.Render(strings.Join(tags, " ")))
	}
	if last := m.hist.LastUsed(h.Alias); !last.IsZero() {
		parts = append(parts, subtitleStyle.Render(history.Ago(last)))
	}
	if lat, ok := m.latency[h.Alias]; ok && m.status[h.Alias] == stateUp {
		parts = append(parts, subtitleStyle.Render(fmt.Sprintf("%dms", lat.Milliseconds())))
	}
	return strings.Join(parts, "  ")
}

// emptyState is the first thing a new user sees, so it says what to do rather
// than just reporting that there is nothing here.
func (m Model) emptyState() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  " + subtitleStyle.Render("No hosts in "+m.configPath) + "\n\n")
	b.WriteString("  " + keyStyle.Render("i") + subtitleStyle.Render("  paste an ssh command you already use") + "\n")
	b.WriteString("  " + keyStyle.Render("a") + subtitleStyle.Render("  add a host by hand") + "\n")
	b.WriteString("  " + keyStyle.Render("K") + subtitleStyle.Render("  find private keys lying around your disk") + "\n")
	return b.String()
}

func (m Model) header() string {
	title := titleStyle.Render("hoppr")

	var right string
	if m.readOnly {
		right = warnStyle.Render(" read-only ")
	}

	// The search field replaces the subtitle while it has focus.
	var mid string
	if m.searching || m.search.Value() != "" {
		mid = " " + promptStyle.Render("/") + m.search.View()
	} else {
		n := len(m.matches)
		label := fmt.Sprintf(" %d host", n)
		if n != 1 {
			label += "s"
		}
		if len(m.tagFilter) > 0 {
			label += subtitleStyle.Render(" · " + strings.Join(m.tagFilter, "+"))
		}
		mid = subtitleStyle.Render(label)
	}

	line := title + mid
	gap := m.width - lipgloss.Width(line) - lipgloss.Width(right)
	if gap > 0 {
		line += strings.Repeat(" ", gap)
	}
	return line + right
}

func (m Model) footer() string {
	if m.flash != "" {
		style := okStyle
		if m.flashErr {
			style = errStyle
		}
		return "\n" + style.Render("  "+m.flash)
	}

	hints := [][2]string{
		{"enter", "connect"},
		{"/", "search"},
		{"i", "import"},
		{"a", "add"},
		{"e", "edit"},
		{"K", "keys"},
		{"D", "doctor"},
		{"?", "help"},
	}
	if m.width < compactWidth {
		hints = hints[:4]
	}

	var parts []string
	for _, h := range hints {
		parts = append(parts, keyStyle.Render(h[0])+" "+helpStyle.Render(h[1]))
	}
	return "\n  " + strings.Join(parts, helpStyle.Render(" · "))
}

// tagBar is the tag filter strip.
func (m Model) tagBar() string {
	var parts []string
	for i, t := range m.allTags {
		style := inactiveTagStyle
		if contains(m.tagFilter, t) {
			style = activeTagStyle
		}

		label := t
		if i == m.tagCursor {
			label = "▸" + t
		}
		parts = append(parts, style.Render(label))
	}
	return "  " + strings.Join(parts, " ")
}

// ---- Geometry ----

// chromeHeight is everything that is not a list row: header, footer, and the
// tag bar when it is up.
func (m Model) chromeHeight() int {
	h := 4 // header + blank + footer + its leading newline
	if m.tagMode {
		h++
	}
	return h
}

// listHeight is how many host rows fit. It never goes below minListRows, so a
// very short terminal scrolls rather than showing nothing at all.
func (m Model) listHeight() int {
	return max(minListRows, m.height-m.chromeHeight())
}

func (m Model) aliasWidth() int {
	switch {
	case m.width < tinyWidth:
		return max(8, m.width-6)
	case m.width < compactWidth:
		return 20
	default:
		return 24
	}
}

func (m Model) targetWidth() int {
	if m.width < compactWidth {
		return max(10, m.width-m.aliasWidth()-8)
	}
	return 34
}

// clampScroll keeps the cursor inside the visible window.
func (m *Model) clampScroll() {
	rows := m.listHeight()

	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
	if maxOffset := max(0, len(m.matches)-rows); m.offset > maxOffset {
		m.offset = maxOffset
	}
}

// ---- Text helpers ----

// highlight bolds the runes at the given offsets.
func highlight(s string, positions []int) string {
	at := map[int]bool{}
	for _, p := range positions {
		at[p] = true
	}

	var b strings.Builder
	for i, r := range []rune(s) {
		if at[i] {
			b.WriteString(matchStyle.Render(string(r)))
		} else {
			b.WriteString(aliasStyle.Render(string(r)))
		}
	}
	return b.String()
}

// truncate shortens s to w display cells, with an ellipsis.
func truncate(s string, w int) string {
	if w <= 1 || lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	if len(r) <= w-1 {
		return s
	}
	return string(r[:w-1]) + "…"
}

// pad right-pads s to w display cells, measuring the rendered width so ANSI
// styling does not throw the column alignment off.
func pad(s string, w int) string {
	gap := w - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
