// Package tui is hoppr's terminal interface.
package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/SarthakShrivastav-a/hoppr/internal/doctor"
	"github.com/SarthakShrivastav-a/hoppr/internal/history"
	"github.com/SarthakShrivastav-a/hoppr/internal/host"
	"github.com/SarthakShrivastav-a/hoppr/internal/keys"
	"github.com/SarthakShrivastav-a/hoppr/internal/probe"
	"github.com/SarthakShrivastav-a/hoppr/internal/sshconfig"
)

// probeState mirrors probe.State so styles.go does not have to import probe.
type probeState int

const (
	stateUnknown probeState = iota
	stateChecking
	stateUp
	stateDown
)

// view is which screen is on top.
type view int

const (
	viewList view = iota
	viewForm
	viewImport
	viewKeys
	viewDoctor
	viewConfirm
	viewHelp
)

// Model is the whole application state.
type Model struct {
	// Config, and the hosts read from it.
	configPath string
	tree       *sshconfig.Tree
	hosts      []*host.Host
	matches    []host.Match

	// ReadOnly refuses every mutation.
	//
	// Users explicitly ask for this, and the reason they give is worth
	// repeating: it guards their config against bugs in the manager itself.
	// A tool that rewrites ssh_config should be able to promise not to.
	readOnly bool

	hist *history.History

	// Screen state.
	view     view
	prev     view
	cursor   int
	offset   int
	width    int
	height   int
	quitting bool

	// Search and tag filtering.
	search    textinput.Model
	searching bool
	allTags   []string
	tagFilter []string
	tagCursor int
	tagMode   bool

	// Reachability, keyed by host alias.
	status  map[string]probeState
	latency map[string]time.Duration

	// The keys pane.
	keyList   []*keys.Key
	keyUsage  []keys.Usage
	keyCursor int
	keysBusy  bool

	// The doctor pane.
	report       *doctor.Report
	doctorCursor int

	// Forms.
	form    *form
	confirm *confirmState

	// Transient feedback.
	flash    string
	flashErr bool
}

// confirmState backs a yes/no prompt.
type confirmState struct {
	prompt string
	detail string
	action func() tea.Cmd
}

// New builds the model. It does not touch the network or the filesystem beyond
// the config it was handed; the expensive work happens in Init.
func New(configPath string, tree *sshconfig.Tree, readOnly bool) Model {
	si := textinput.New()
	si.Prompt = "" // the view draws its own, so it can style it
	si.Placeholder = "search hosts"
	si.CharLimit = 128

	hosts := host.Load(tree)

	m := Model{
		configPath: configPath,
		tree:       tree,
		hosts:      hosts,
		readOnly:   readOnly,
		hist:       history.Load(),
		search:     si,
		allTags:    host.AllTags(hosts),
		status:     map[string]probeState{},
		latency:    map[string]time.Duration{},
		// A sane default so the first frame is not rendered into a 0x0 box on
		// terminals that are slow to report their size.
		width:  80,
		height: 24,
	}
	m.refilter()
	return m
}

// Init kicks off the reachability sweep.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		probeCmd(m.hosts),
	)
}

// ---- Messages ----

type keysScannedMsg struct {
	keys  []*keys.Key
	usage []keys.Usage
}

type doctorRanMsg struct{ report *doctor.Report }

type reloadedMsg struct {
	tree  *sshconfig.Tree
	hosts []*host.Host
}

type flashMsg struct {
	text string
	err  bool
}

// connectFinishedMsg carries ssh's exit back into the program.
//
// A failed connection is not a hoppr error and must not be rendered as one. ssh
// has already printed the real reason to the terminal; the job here is to say
// that it failed and get out of the way, not to paint a box over the output the
// user needs to read.
type connectFinishedMsg struct{ err error }

// ---- Commands ----

// probeCmd sweeps every host's reachability, streaming each result back as it
// arrives so the list fills in progressively.
func probeCmd(hosts []*host.Host) tea.Cmd {
	if len(hosts) == 0 {
		return nil
	}

	// Map addresses back to aliases: two hosts can legitimately share an
	// address, and both of their rows should light up.
	byAddr := map[string][]string{}
	var addrs []string
	for _, h := range hosts {
		a := h.Addr()
		if _, seen := byAddr[a]; !seen {
			addrs = append(addrs, a)
		}
		byAddr[a] = append(byAddr[a], h.Alias)
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var results []probe.Result
		for r := range probe.CheckAll(ctx, addrs, probe.DefaultTimeout) {
			results = append(results, r)
		}
		return probeBatchMsg{results: results, byAddr: byAddr}
	}
}

// probeBatchMsg delivers the whole sweep at once.
//
// Streaming each result as a separate message would be nicer, but Bubble Tea
// commands return exactly one message. Rather than reach for a channel-backed
// subscription for what is a sub-second operation on a warm network, collect
// and deliver once.
type probeBatchMsg struct {
	results []probe.Result
	byAddr  map[string][]string
}

func scanKeysCmd(hosts []*host.Host) tea.Cmd {
	return func() tea.Msg {
		ks := keys.Scan(keys.DefaultRoots())
		return keysScannedMsg{keys: ks, usage: keys.Index(ks, hosts)}
	}
}

func doctorCmd(configPath string, hosts []*host.Host, ks []*keys.Key) tea.Cmd {
	return func() tea.Msg {
		if ks == nil {
			ks = keys.Scan(keys.DefaultRoots())
		}
		return doctorRanMsg{report: doctor.Run(configPath, hosts, ks)}
	}
}

func reloadCmd(configPath string) tea.Cmd {
	return func() tea.Msg {
		tree, err := sshconfig.LoadTree(configPath)
		if err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		return reloadedMsg{tree: tree, hosts: host.Load(tree)}
	}
}

func flash(text string) tea.Cmd {
	return func() tea.Msg { return flashMsg{text: text} }
}

func flashError(err error) tea.Cmd {
	return func() tea.Msg { return flashMsg{text: err.Error(), err: true} }
}

// ---- Helpers ----

// refilter recomputes the visible rows and keeps the cursor in range.
func (m *Model) refilter() {
	m.matches = host.Filter(m.hosts, m.search.Value(), m.tagFilter)

	// With no query, rank by what you actually use. Alphabetical is close to
	// random here: the two boxes you touch daily sit wherever the alphabet put
	// them, under six you provisioned once and forgot.
	//
	// Once a query is being typed, the fuzzy score is what the user is steering
	// by, and reordering underneath them would make the list fight back.
	if m.search.Value() == "" {
		host.SortByRecency(m.matches, m.hist.LastUsed)
	}

	if m.cursor >= len(m.matches) {
		m.cursor = max(0, len(m.matches)-1)
	}
	m.clampScroll()
}

// selected returns the host under the cursor.
func (m *Model) selected() *host.Host {
	if m.cursor < 0 || m.cursor >= len(m.matches) {
		return nil
	}
	return m.matches[m.cursor].Host
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
