package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/SarthakShrivastav-a/hoppr/internal/doctor"
)

// doctorFinding lets update.go talk about findings without importing doctor.
type doctorFinding doctor.Finding

func (f doctorFinding) Fixable() bool { return doctor.Finding(f).Fixable() }

// ---- Add / edit form ----

func (m Model) viewForm() string {
	var b strings.Builder

	title := "Add host"
	if m.form.editing != nil {
		title = "Edit " + m.form.editing.Alias
	}
	b.WriteString(titleStyle.Render(title) + "\n\n")

	for i, in := range m.form.inputs {
		label := labelStyle.Render(fieldLabels[i])
		if i == m.form.focus {
			label = labelStyle.Foreground(colAccent).Bold(true).Render(fieldLabels[i])
		}
		b.WriteString("  " + label + in.View() + "\n")
	}

	if m.form.err != "" {
		b.WriteString("\n  " + errStyle.Render(m.form.err) + "\n")
	}

	b.WriteString("\n  " + hint("enter", "save") + hint("tab", "next field") + hint("esc", "cancel"))
	return b.String()
}

// ---- Import ----

func (m Model) viewImport() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Import an ssh command") + "\n\n")
	b.WriteString("  " + subtitleStyle.Render("Paste the command you have been copying around. The") + "\n")
	b.WriteString("  " + subtitleStyle.Render("leading cd, if there is one, is fine — it gets ignored.") + "\n\n")

	b.WriteString("  " + promptStyle.Render("$ ") + m.form.inputs[0].View() + "\n")

	if m.form.err != "" {
		b.WriteString("\n  " + errStyle.Render(m.form.err) + "\n")
	}

	b.WriteString("\n  " + hint("enter", "parse") + hint("esc", "cancel"))
	return b.String()
}

// ---- Keys pane ----

func (m Model) viewKeys() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Keys") + subtitleStyle.Render(fmt.Sprintf(" %d found", len(m.keyUsage))) + "\n\n")

	if m.keysBusy {
		b.WriteString("  " + subtitleStyle.Render("scanning…") + "\n")
		return b.String()
	}
	if len(m.keyUsage) == 0 {
		b.WriteString("  " + subtitleStyle.Render("No private keys found in ~/.ssh, Downloads, Desktop, or Documents.") + "\n")
		b.WriteString("\n  " + hint("esc", "back"))
		return b.String()
	}

	rows := max(minListRows, m.height-8)
	start := 0
	if m.keyCursor >= rows {
		start = m.keyCursor - rows + 1
	}
	end := min(start+rows, len(m.keyUsage))

	for i := start; i < end; i++ {
		u := m.keyUsage[i]
		k := u.Key

		cursor := "  "
		if i == m.keyCursor {
			cursor = promptStyle.Render("▸ ")
		}

		// The permission verdict is the most important thing on the row, so it
		// gets the marker position.
		perm := okStyle.Render("✓")
		if !k.Perm.OK {
			perm = errStyle.Render("!")
		}

		name := pad(aliasStyle.Render(k.Name()), 24)
		kind := pad(subtitleStyle.Render(k.Describe()), 18)

		// What uses this key is the whole point of the pane.
		var used string
		switch {
		case u.Orphaned():
			used = warnStyle.Render("unused")
		default:
			var aliases []string
			for _, h := range u.Hosts {
				aliases = append(aliases, h.Alias)
			}
			used = tagStyle.Render(strings.Join(aliases, " "))
		}

		line := cursor + perm + " " + name + kind + used
		if k.Stray() {
			line += " " + warnStyle.Render("· stray")
		}

		style := rowStyle
		if i == m.keyCursor {
			style = selectedRowStyle
		}
		b.WriteString(style.Width(m.width).Render(line) + "\n")
	}

	// Detail for the selected key.
	if m.keyCursor >= 0 && m.keyCursor < len(m.keyUsage) {
		k := m.keyUsage[m.keyCursor].Key

		b.WriteString("\n")
		b.WriteString("  " + labelStyle.Render("path") + valueStyle.Render(k.Path) + "\n")
		b.WriteString("  " + labelStyle.Render("fingerprint") + valueStyle.Render(k.ShortFingerprint()) + "\n")

		permStyle := okStyle
		if !k.Perm.OK {
			permStyle = errStyle
		}
		b.WriteString("  " + labelStyle.Render("permissions") + permStyle.Render(k.Perm.Detail) + "\n")
	}

	if m.flash != "" {
		style := okStyle
		if m.flashErr {
			style = errStyle
		}
		b.WriteString("\n  " + style.Render(m.flash) + "\n")
	}

	b.WriteString("\n  " + hint("f", "fix permissions") + hint("A", "adopt") + hint("r", "rescan") + hint("esc", "back"))
	return b.String()
}

// ---- Doctor pane ----

func (m Model) viewDoctor() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Doctor") + "\n\n")

	if m.report == nil {
		b.WriteString("  " + subtitleStyle.Render("checking…") + "\n")
		return b.String()
	}

	problems := m.report.Problems()
	if len(problems) == 0 {
		b.WriteString("  " + okStyle.Render("✓ nothing wrong.") + "\n")
		b.WriteString("  " + subtitleStyle.Render("Keys are locked down, every IdentityFile exists, and the config parses.") + "\n")
		b.WriteString("\n  " + hint("esc", "back"))
		return b.String()
	}

	for i, f := range problems {
		cursor := "  "
		if i == m.doctorCursor {
			cursor = promptStyle.Render("▸ ")
		}

		badge := warnStyle.Render("warn")
		if f.Severity == doctor.Fail {
			badge = errStyle.Render("fail")
		}

		b.WriteString(cursor + badge + " " + aliasStyle.Render(f.Title) + "\n")

		if i == m.doctorCursor {
			// Wrap the explanation to the terminal, so a long detail does not
			// spill off the right edge.
			for _, line := range wrap(f.Detail, max(20, m.width-6)) {
				b.WriteString("      " + subtitleStyle.Render(line) + "\n")
			}
			if f.Fixable() {
				b.WriteString("      " + okStyle.Render("f") + subtitleStyle.Render(" → "+f.FixLabel) + "\n")
			}
		}
	}

	if m.flash != "" {
		style := okStyle
		if m.flashErr {
			style = errStyle
		}
		b.WriteString("\n  " + style.Render(m.flash) + "\n")
	}

	b.WriteString("\n  " + hint("f", "fix this") + hint("r", "recheck") + hint("esc", "back"))
	return b.String()
}

// ---- Confirm ----

func (m Model) viewConfirm() string {
	if m.confirm == nil {
		return ""
	}

	body := aliasStyle.Render(m.confirm.prompt) + "\n\n" +
		subtitleStyle.Render(m.confirm.detail) + "\n\n" +
		hint("y", "yes") + hint("n", "no")

	box := borderStyle.Padding(1, 2).Render(body)

	// Center the dialog, but only if there is room. On a short terminal, Place
	// would clip the box; drawing it plain at the top is worse-looking and
	// strictly more useful.
	if m.width < 40 || m.height < 10 {
		return "\n" + box
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// ---- Help ----

func (m Model) viewHelp() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("hoppr — keys") + "\n\n")

	sections := []struct {
		name  string
		items [][2]string
	}{
		{"Move", [][2]string{
			{"j / ↓", "down"},
			{"k / ↑", "up"},
			{"g / G", "top / bottom"},
			{"ctrl+d / ctrl+u", "page down / up"},
		}},
		{"Hosts", [][2]string{
			{"enter", "connect"},
			{"/", "fuzzy search alias, target, tag, note"},
			{"t", "filter by tag"},
			{"i", "import a pasted ssh command"},
			{"a / e / d", "add / edit / delete"},
			{"r", "re-check reachability"},
		}},
		{"Keys and health", [][2]string{
			{"K", "key inventory — fingerprints, usage, permissions"},
			{"D", "doctor — diagnose what will break"},
			{"f", "apply the fix under the cursor"},
			{"A", "adopt a stray key into ~/.ssh/keys"},
		}},
		{"General", [][2]string{
			{"?", "this help"},
			{"esc", "back / clear filter"},
			{"q / ctrl+c", "quit"},
		}},
	}

	for _, s := range sections {
		b.WriteString("  " + tagStyle.Render(s.name) + "\n")
		for _, it := range s.items {
			b.WriteString("    " + pad(keyStyle.Render(it[0]), 20) + helpStyle.Render(it[1]) + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("  " + subtitleStyle.Render("config: "+m.configPath) + "\n")
	return b.String()
}

// ---- Shared bits ----

func hint(k, label string) string {
	return keyStyle.Render(k) + " " + helpStyle.Render(label) + "  "
}

// wrap breaks s into lines of at most w cells, on word boundaries.
func wrap(s string, w int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}

	var (
		lines []string
		cur   string
	)
	for _, word := range words {
		switch {
		case cur == "":
			cur = word
		case len(cur)+1+len(word) <= w:
			cur += " " + word
		default:
			lines = append(lines, cur)
			cur = word
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}
