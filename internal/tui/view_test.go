package tui

import (
	"strings"
	"testing"

	"github.com/SarthakShrivastav-a/hoppr/internal/host"
	"github.com/SarthakShrivastav-a/hoppr/internal/sshconfig"
)

func newTestModel(t *testing.T, src string) Model {
	t.Helper()
	f := sshconfig.ParseBytes([]byte(src), "config")
	tree := &sshconfig.Tree{Root: f, Files: []*sshconfig.File{f}}
	return New("config", tree, false)
}

const testConfig = `Host prod
    # hoppr: tags=aws
    HostName 203.0.113.10
    User ec2-user

Host dev
    HostName dev.example.com

Host staging
    HostName staging.example.com
`

func TestSmallTerminalStillRenders(t *testing.T) {
	// "Terminal too small, please resize" is real behaviour in this category and
	// it is filed as a bug every time, because it breaks tmux splits, tiling
	// window managers, and the editor terminal panel. Degrade; never refuse.
	m := newTestModel(t, testConfig)

	for _, size := range [][2]int{{20, 5}, {40, 8}, {1, 1}, {200, 60}} {
		m.width, m.height = size[0], size[1]
		m.clampScroll()

		out := m.View()
		if strings.TrimSpace(out) == "" {
			t.Errorf("%dx%d rendered nothing at all", size[0], size[1])
		}
		if strings.Contains(strings.ToLower(out), "too small") {
			t.Errorf("%dx%d refused to render", size[0], size[1])
		}
	}
}

func TestListHeightNeverCollapses(t *testing.T) {
	m := newTestModel(t, testConfig)

	// Even in a terminal shorter than the chrome, there must be room for rows,
	// or the list scrolls into nothing.
	for _, h := range []int{0, 1, 3, 5, 24, 100} {
		m.height = h
		if got := m.listHeight(); got < minListRows {
			t.Errorf("height %d gave a list height of %d, below the floor of %d", h, got, minListRows)
		}
	}
}

func TestClampScrollKeepsTheCursorVisible(t *testing.T) {
	m := newTestModel(t, testConfig)
	m.width, m.height = 80, 8

	m.cursor = len(m.matches) - 1
	m.clampScroll()

	if m.cursor < m.offset || m.cursor >= m.offset+m.listHeight() {
		t.Fatalf("cursor %d is outside the window [%d, %d)",
			m.cursor, m.offset, m.offset+m.listHeight())
	}
	if m.offset < 0 {
		t.Errorf("offset went negative: %d", m.offset)
	}
}

func TestEmptyConfigShowsAnActionableEmptyState(t *testing.T) {
	m := newTestModel(t, "")
	m.width, m.height = 80, 24

	out := m.View()
	// The first thing a new user sees should say what to do, not just report
	// that there is nothing here.
	for _, want := range []string{"paste an ssh command", "add a host"} {
		if !strings.Contains(out, want) {
			t.Errorf("the empty state should mention %q:\n%s", want, out)
		}
	}
}

func TestSearchFiltersTheList(t *testing.T) {
	m := newTestModel(t, testConfig)

	if len(m.matches) != 3 {
		t.Fatalf("want 3 hosts to start, got %d", len(m.matches))
	}

	m.search.SetValue("prd")
	m.refilter()

	// Fuzzy, not substring: "prd" is not a substring of "prod".
	if len(m.matches) != 1 || m.matches[0].Host.Alias != "prod" {
		t.Fatalf("fuzzy search for prd should find prod, got %d matches", len(m.matches))
	}
}

func TestRefilterKeepsTheCursorInRange(t *testing.T) {
	m := newTestModel(t, testConfig)

	m.cursor = 2
	m.search.SetValue("prod") // narrows to one match
	m.refilter()

	if m.cursor >= len(m.matches) {
		t.Fatalf("cursor %d is past the end of a %d-row list", m.cursor, len(m.matches))
	}
	if m.selected() == nil {
		t.Fatal("nothing is selected after narrowing the list")
	}
}

func TestTagFilterAndsWithSearch(t *testing.T) {
	m := newTestModel(t, testConfig)

	m.tagFilter = []string{"aws"}
	m.refilter()
	if len(m.matches) != 1 || m.matches[0].Host.Alias != "prod" {
		t.Fatalf("the aws tag should select only prod, got %d", len(m.matches))
	}

	// dev is not tagged aws, so this must be empty even though "dev" matches.
	m.search.SetValue("dev")
	m.refilter()
	if len(m.matches) != 0 {
		t.Fatalf("tag and query should AND, got %d matches", len(m.matches))
	}
}

func TestSelectedIsNilWhenNothingMatches(t *testing.T) {
	m := newTestModel(t, testConfig)

	m.search.SetValue("zzzzz")
	m.refilter()

	// Every keybinding that acts on a host goes through this. Returning a host
	// here would connect to, edit, or delete the wrong one.
	if m.selected() != nil {
		t.Fatal("selected() returned a host from an empty list")
	}
}

func TestPatternStanzasNeverAppearInTheList(t *testing.T) {
	m := newTestModel(t, "Host *\n  ServerAliveInterval 60\n\nHost prod\n  HostName 1.1.1.1\n")

	for _, match := range m.matches {
		if host.IsPattern(match.Host.Alias) {
			t.Errorf("pattern %q is selectable in the UI", match.Host.Alias)
		}
	}
	if len(m.matches) != 1 {
		t.Fatalf("want only prod, got %d rows", len(m.matches))
	}
}

func TestTruncateAndPad(t *testing.T) {
	if got := truncate("abcdefgh", 5); got != "abcd…" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("truncate should leave short strings alone, got %q", got)
	}
	if got := pad("ab", 5); got != "ab   " {
		t.Errorf("pad = %q", got)
	}
	if got := pad("abcdef", 3); got != "abcdef" {
		t.Errorf("pad should not truncate, got %q", got)
	}
}

func TestWrapBreaksOnWords(t *testing.T) {
	lines := wrap("the quick brown fox jumps", 10)
	for _, l := range lines {
		if len(l) > 10 {
			t.Errorf("line %q exceeds the width", l)
		}
	}
	if strings.Join(lines, " ") != "the quick brown fox jumps" {
		t.Errorf("wrap lost or reordered words: %v", lines)
	}
}
