package tui

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/SarthakShrivastav-a/hoppr/internal/connect"
	"github.com/SarthakShrivastav-a/hoppr/internal/host"
	"github.com/SarthakShrivastav-a/hoppr/internal/keys"
	"github.com/SarthakShrivastav-a/hoppr/internal/probe"
)

var errReadOnly = errors.New("read-only mode: config not modified")

// Update is the root message handler.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
		return m, nil

	case probeBatchMsg:
		for _, r := range msg.results {
			for _, alias := range msg.byAddr[r.Addr] {
				m.status[alias] = toState(r.State)
				m.latency[alias] = r.Latency
			}
		}
		return m, nil

	case keysScannedMsg:
		m.keyList, m.keyUsage, m.keysBusy = msg.keys, msg.usage, false
		if m.keyCursor >= len(m.keyUsage) {
			m.keyCursor = max(0, len(m.keyUsage)-1)
		}
		return m, nil

	case doctorRanMsg:
		m.report = msg.report
		m.doctorCursor = 0
		return m, nil

	case reloadedMsg:
		m.tree, m.hosts = msg.tree, msg.hosts
		m.allTags = host.AllTags(m.hosts)
		m.refilter()
		return m, nil

	case flashMsg:
		m.flash, m.flashErr = msg.text, msg.err
		return m, nil

	case connectFinishedMsg:
		if msg.err != nil {
			// ssh already printed the real reason. Say it failed, and do not
			// paint over the output the user needs to read.
			var ee *exec.ExitError
			if errors.As(msg.err, &ee) {
				m.flash = fmt.Sprintf("ssh exited with status %d — see the output above", ee.ExitCode())
			} else {
				m.flash = msg.err.Error()
			}
			m.flashErr = true
		}
		// The host list may be stale if the session changed anything, and a
		// re-probe after a session is cheap and usually informative.
		return m, probeCmd(m.hosts)

	case tea.KeyMsg:
		// Any keystroke dismisses a stale flash message.
		m.flash = ""
		return m.handleKey(msg)
	}

	return m, nil
}

// handleKey dispatches to the active view.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+C always quits, from every view, no matter what has focus. A TUI you
	// cannot escape from is a bug, and a text field that swallows the interrupt
	// is the usual way that happens.
	if msg.Type == tea.KeyCtrlC {
		m.quitting = true
		return m, tea.Quit
	}

	switch m.view {
	case viewForm:
		return m.updateForm(msg)
	case viewImport:
		return m.updateImport(msg)
	case viewKeys:
		return m.updateKeys(msg)
	case viewDoctor:
		return m.updateDoctor(msg)
	case viewConfirm:
		return m.updateConfirm(msg)
	case viewHelp:
		if key.Matches(msg, defaultKeys.Back, defaultKeys.Help, defaultKeys.Quit) {
			m.view = m.prev
		}
		return m, nil
	default:
		return m.updateList(msg)
	}
}

// updateList handles the main host list.
func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := defaultKeys

	// While the search field has focus it must receive almost every keystroke,
	// or typing a host name called "dev" would trigger delete, edit, and
	// nothing-useful instead of filtering.
	if m.searching {
		switch msg.Type {
		case tea.KeyEsc:
			m.searching = false
			m.search.SetValue("")
			m.search.Blur()
			m.refilter()
			return m, nil
		case tea.KeyEnter:
			m.searching = false
			m.search.Blur()
			return m, nil
		case tea.KeyUp, tea.KeyDown:
			// Let the user move through results without leaving the field.
			return m.moveCursor(msg)
		}

		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		m.refilter()
		return m, cmd
	}

	if m.tagMode {
		return m.updateTagFilter(msg)
	}

	switch {
	case key.Matches(msg, k.Quit):
		m.quitting = true
		return m, tea.Quit

	case key.Matches(msg, k.Help):
		m.prev, m.view = m.view, viewHelp
		return m, nil

	case key.Matches(msg, k.Search):
		m.searching = true
		m.search.Focus()
		return m, textinputBlink()

	case key.Matches(msg, k.Clear):
		if m.search.Value() != "" || len(m.tagFilter) > 0 {
			m.search.SetValue("")
			m.tagFilter = nil
			m.refilter()
		}
		return m, nil

	case key.Matches(msg, k.Up, k.Down, k.Top, k.Bottom, k.PageUp, k.PageDn):
		return m.moveCursor(msg)

	case key.Matches(msg, k.Connect):
		h := m.selected()
		if h == nil {
			return m, nil
		}
		return m.connect(h)

	case key.Matches(msg, k.Probe):
		for a := range m.status {
			m.status[a] = stateChecking
		}
		return m, probeCmd(m.hosts)

	case key.Matches(msg, k.Tag):
		if len(m.allTags) == 0 {
			return m, flash("no tags yet — add one when you edit a host")
		}
		m.tagMode = true
		return m, nil

	case key.Matches(msg, k.Add):
		if m.readOnly {
			return m, flashError(errReadOnly)
		}
		m.form = newAddForm()
		m.view = viewForm
		return m, textinputBlink()

	case key.Matches(msg, k.Edit):
		h := m.selected()
		if h == nil {
			return m, nil
		}
		if m.readOnly {
			return m, flashError(errReadOnly)
		}
		m.form = newEditForm(h)
		m.view = viewForm
		return m, textinputBlink()

	case key.Matches(msg, k.Import):
		if m.readOnly {
			return m, flashError(errReadOnly)
		}
		m.form = newImportForm()
		m.view = viewImport
		return m, textinputBlink()

	case key.Matches(msg, k.Delete):
		h := m.selected()
		if h == nil {
			return m, nil
		}
		if m.readOnly {
			return m, flashError(errReadOnly)
		}
		return m.askDelete(h), nil

	case key.Matches(msg, k.Keys):
		m.view = viewKeys
		if m.keyList == nil {
			m.keysBusy = true
			return m, scanKeysCmd(m.hosts)
		}
		return m, nil

	case key.Matches(msg, k.Doctor):
		m.view = viewDoctor
		return m, doctorCmd(m.configPath, m.hosts, m.keyList)
	}

	return m, nil
}

// connect hands the terminal over to ssh.
//
// tea.ExecProcess suspends the program, restores the terminal to its normal
// state, runs ssh attached to the real stdio, and resumes afterwards. Doing it
// any other way — piping ssh's output through the TUI, or leaving the alt-screen
// up — produces the bugs this category is full of: garbled output, a manager
// left running behind the session, and ssh's error message flashing past too
// fast to read.
func (m Model) connect(h *host.Host) (tea.Model, tea.Cmd) {
	cmd, err := connect.Command(h, connect.Options{ConfigPath: m.configPath})
	if err != nil {
		// An alias ssh would read as a flag. host.Load should already have kept
		// it out of the list, so reaching here means something is wrong — say so
		// rather than running it.
		return m, flashError(err)
	}

	m.hist.Record(h.Alias)

	return m, tea.Sequence(
		// Set the terminal title to the host, so a wall of tabs is navigable.
		tea.SetWindowTitle("ssh "+h.Alias),
		tea.ExecProcess(cmd, func(err error) tea.Msg {
			return connectFinishedMsg{err: err}
		}),
		tea.SetWindowTitle("hoppr"),
	)
}

// moveCursor handles every motion key for the list.
func (m Model) moveCursor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := defaultKeys
	page := max(1, m.listHeight()-1)

	switch {
	case key.Matches(msg, k.Up):
		m.cursor--
	case key.Matches(msg, k.Down):
		m.cursor++
	case key.Matches(msg, k.Top):
		m.cursor = 0
	case key.Matches(msg, k.Bottom):
		m.cursor = len(m.matches) - 1
	case key.Matches(msg, k.PageUp):
		m.cursor -= page
	case key.Matches(msg, k.PageDn):
		m.cursor += page
	}

	m.cursor = clamp(m.cursor, 0, max(0, len(m.matches)-1))
	m.clampScroll()
	return m, nil
}

// updateTagFilter drives the tag bar.
func (m Model) updateTagFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := defaultKeys

	switch {
	case key.Matches(msg, k.Back), key.Matches(msg, k.Tag):
		m.tagMode = false
	case key.Matches(msg, k.Up):
		m.tagCursor = clamp(m.tagCursor-1, 0, len(m.allTags)-1)
	case key.Matches(msg, k.Down):
		m.tagCursor = clamp(m.tagCursor+1, 0, len(m.allTags)-1)
	case key.Matches(msg, k.Confirm):
		m.toggleTag(m.allTags[m.tagCursor])
		m.refilter()
	case key.Matches(msg, k.Clear):
		m.tagFilter = nil
		m.refilter()
	}
	return m, nil
}

func (m *Model) toggleTag(tag string) {
	for i, t := range m.tagFilter {
		if t == tag {
			m.tagFilter = append(m.tagFilter[:i], m.tagFilter[i+1:]...)
			return
		}
	}
	m.tagFilter = append(m.tagFilter, tag)
}

// askDelete stages a confirmation rather than deleting straight away.
func (m Model) askDelete(h *host.Host) Model {
	alias := h.Alias
	file := h.ConfigFile()

	m.confirm = &confirmState{
		prompt: fmt.Sprintf("Delete host %q?", alias),
		detail: fmt.Sprintf("Removes the stanza from %s. The key file, if any, is left alone.", file.Path),
		action: func() tea.Cmd {
			file.RemoveHost(alias)
			if err := file.Save(); err != nil {
				return flashError(err)
			}
			return tea.Batch(
				reloadCmd(m.configPath),
				flash("deleted "+alias),
			)
		},
	}
	m.view = viewConfirm
	return m
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		c := m.confirm
		m.confirm = nil
		m.view = viewList
		if c == nil {
			return m, nil
		}
		return m, c.action()
	case "n", "N", "esc", "q":
		m.confirm = nil
		m.view = viewList
	}
	return m, nil
}

// updateKeys drives the key manager.
func (m Model) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := defaultKeys

	switch {
	case key.Matches(msg, k.Back), key.Matches(msg, k.Quit), key.Matches(msg, k.Keys):
		m.view = viewList
		return m, nil

	case key.Matches(msg, k.Up):
		m.keyCursor = clamp(m.keyCursor-1, 0, max(0, len(m.keyUsage)-1))
		return m, nil
	case key.Matches(msg, k.Down):
		m.keyCursor = clamp(m.keyCursor+1, 0, max(0, len(m.keyUsage)-1))
		return m, nil

	case key.Matches(msg, k.Probe):
		m.keysBusy = true
		return m, scanKeysCmd(m.hosts)

	case key.Matches(msg, k.Fix):
		u := m.selectedKey()
		if u == nil || u.Key.Perm.OK {
			return m, nil
		}
		path := u.Key.Path
		name := u.Key.Name()
		return m, tea.Sequence(
			func() tea.Msg {
				if err := keys.Fix(path); err != nil {
					return flashMsg{text: err.Error(), err: true}
				}
				return flashMsg{text: "locked " + name + " to your account"}
			},
			scanKeysCmd(m.hosts),
		)

	case key.Matches(msg, k.Adopt):
		u := m.selectedKey()
		if u == nil {
			return m, nil
		}
		if m.readOnly {
			return m, flashError(errReadOnly)
		}
		if !u.Key.Stray() {
			return m, flash(u.Key.Name() + " already lives in ~/.ssh")
		}
		return m.askAdopt(u.Key), nil
	}

	return m, nil
}

func (m *Model) selectedKey() *keys.Usage {
	if m.keyCursor < 0 || m.keyCursor >= len(m.keyUsage) {
		return nil
	}
	return &m.keyUsage[m.keyCursor]
}

// askAdopt confirms before moving a key, because moving a key the user does not
// expect us to touch is exactly the kind of surprise that loses trust.
func (m Model) askAdopt(k *keys.Key) Model {
	hosts := m.hosts
	configPath := m.configPath
	tree := m.tree

	m.confirm = &confirmState{
		prompt: fmt.Sprintf("Adopt %s?", k.Name()),
		detail: "Moves it to ~/.ssh/keys, locks its permissions, and repoints every host that uses it.",
		action: func() tea.Cmd {
			res, err := keys.Adopt(k, hosts)
			if err != nil {
				return flashError(err)
			}
			if err := tree.Save(); err != nil {
				return flashError(err)
			}

			msg := "adopted " + k.Name()
			if n := len(res.Rewritten); n > 0 {
				msg = fmt.Sprintf("%s and repointed %d host", msg, n)
				if n > 1 {
					msg += "s"
				}
			}
			return tea.Batch(
				reloadCmd(configPath),
				scanKeysCmd(hosts),
				flash(msg),
			)
		},
	}
	m.view = viewConfirm
	return m
}

// updateDoctor drives the diagnostics pane.
func (m Model) updateDoctor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := defaultKeys
	problems := m.doctorProblems()

	switch {
	case key.Matches(msg, k.Back), key.Matches(msg, k.Quit), key.Matches(msg, k.Doctor):
		m.view = viewList
		return m, nil

	case key.Matches(msg, k.Up):
		m.doctorCursor = clamp(m.doctorCursor-1, 0, max(0, len(problems)-1))
		return m, nil
	case key.Matches(msg, k.Down):
		m.doctorCursor = clamp(m.doctorCursor+1, 0, max(0, len(problems)-1))
		return m, nil

	case key.Matches(msg, k.Probe):
		return m, doctorCmd(m.configPath, m.hosts, m.keyList)

	case key.Matches(msg, k.Fix):
		if m.doctorCursor >= len(problems) {
			return m, nil
		}
		f := problems[m.doctorCursor]
		if !f.Fixable() {
			return m, flash("nothing to fix automatically here")
		}
		if m.readOnly {
			return m, flashError(errReadOnly)
		}

		return m, tea.Sequence(
			func() tea.Msg {
				if err := f.Fix(); err != nil {
					return flashMsg{text: err.Error(), err: true}
				}
				return flashMsg{text: "fixed: " + f.Title}
			},
			reloadCmd(m.configPath),
			doctorCmd(m.configPath, m.hosts, nil),
		)
	}

	return m, nil
}

func (m *Model) doctorProblems() []doctorFinding {
	if m.report == nil {
		return nil
	}
	ps := m.report.Problems()
	out := make([]doctorFinding, len(ps))
	for i, p := range ps {
		out[i] = doctorFinding(p)
	}
	return out
}

func toState(s probe.State) probeState {
	switch s {
	case probe.Up:
		return stateUp
	case probe.Down:
		return stateDown
	case probe.Checking:
		return stateChecking
	default:
		return stateUnknown
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
