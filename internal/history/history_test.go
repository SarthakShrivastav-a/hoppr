package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempState(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func TestRecordAndReload(t *testing.T) {
	home := tempState(t)

	h := Load()
	h.Record("prod")
	h.Record("prod")
	h.Record("dev")

	// A fresh Load must see what the previous one wrote.
	h2 := Load()

	e, ok := h2.Get("prod")
	if !ok {
		t.Fatal("prod was not persisted")
	}
	if e.Count != 2 {
		t.Errorf("Count = %d, want 2", e.Count)
	}
	if e.Last.IsZero() {
		t.Error("Last was not recorded")
	}

	if _, ok := h2.Get("dev"); !ok {
		t.Error("dev was not persisted")
	}
	if _, ok := h2.Get("never-touched"); ok {
		t.Error("a host that was never connected to should not have a record")
	}

	if _, err := os.Stat(filepath.Join(home, ".hoppr", "history.json")); err != nil {
		t.Errorf("history file was not written: %v", err)
	}
}

func TestCorruptHistoryIsNotFatal(t *testing.T) {
	home := tempState(t)

	dir := filepath.Join(home, ".hoppr")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "history.json"), []byte("{{{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	// History is a convenience cache. Refusing to start because it is unreadable
	// would be a self-inflicted outage.
	h := Load()
	if h == nil {
		t.Fatal("Load returned nil on a corrupt file")
	}
	if _, ok := h.Get("anything"); ok {
		t.Error("a corrupt file should yield empty history, not garbage")
	}

	// And it must recover: recording should overwrite the bad file.
	h.Record("prod")
	if _, ok := Load().Get("prod"); !ok {
		t.Error("history did not recover after a corrupt file")
	}
}

func TestLastUsedIsZeroForUnknownHost(t *testing.T) {
	tempState(t)

	if !Load().LastUsed("nope").IsZero() {
		t.Error("an unknown host should have a zero last-used time")
	}
}

func TestAgo(t *testing.T) {
	now := time.Now()
	cases := []struct {
		in   time.Time
		want string
	}{
		{time.Time{}, "never"},
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-2 * 24 * time.Hour), "2d ago"},
	}
	for _, tc := range cases {
		if got := Ago(tc.in); got != tc.want {
			t.Errorf("Ago(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}

	// Anything older than a month gets a date rather than an ever-growing
	// "417d ago", which is not a useful thing to read in a list column.
	old := now.Add(-90 * 24 * time.Hour)
	if got := Ago(old); got != old.Format("2006-01-02") {
		t.Errorf("Ago(90 days ago) = %q, want a date", got)
	}
}
