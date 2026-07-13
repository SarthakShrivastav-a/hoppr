// Package history records which hosts were connected to and when.
//
// This is the only state hoppr keeps outside ssh_config, and it is deliberately
// disposable: delete the file and hoppr still works, it just forgets what you
// used last. Nothing here is authoritative, so nothing here can corrupt a
// config or lock anyone out.
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/SarthakShrivastav-a/hoppr/internal/paths"
)

// maxEntries caps the file. History exists to sort the list by recency, and a
// host you have not touched in five hundred connections is not going to win
// that sort.
const maxEntries = 500

// Entry is one host's connection record.
type Entry struct {
	Alias string    `json:"alias"`
	Last  time.Time `json:"last"`
	Count int       `json:"count"`
}

// History is the on-disk record, keyed by host alias.
type History struct {
	mu      sync.Mutex
	path    string
	entries map[string]*Entry
}

// Load reads the history file. A missing or corrupt file yields empty history
// rather than an error: this is a convenience cache, and refusing to start
// because it is unreadable would be a self-inflicted outage.
func Load() *History {
	h := &History{entries: map[string]*Entry{}}

	dir, err := paths.StateDir()
	if err != nil {
		return h
	}
	h.path = filepath.Join(dir, "history.json")

	raw, err := os.ReadFile(h.path)
	if err != nil {
		return h
	}

	var list []*Entry
	if err := json.Unmarshal(raw, &list); err != nil {
		return h
	}
	for _, e := range list {
		if e.Alias != "" {
			h.entries[e.Alias] = e
		}
	}
	return h
}

// Record notes a connection to alias and persists it.
//
// A write failure is swallowed on purpose. The user asked to connect to a
// server; failing that because a cache file could not be written would be
// absurd.
func (h *History) Record(alias string) {
	h.mu.Lock()
	e, ok := h.entries[alias]
	if !ok {
		e = &Entry{Alias: alias}
		h.entries[alias] = e
	}
	e.Last = time.Now()
	e.Count++
	h.mu.Unlock()

	_ = h.Save()
}

// Get returns the record for alias, if there is one.
func (h *History) Get(alias string) (Entry, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	e, ok := h.entries[alias]
	if !ok {
		return Entry{}, false
	}
	return *e, true
}

// LastUsed returns when alias was last connected to, or the zero time.
func (h *History) LastUsed(alias string) time.Time {
	e, ok := h.Get(alias)
	if !ok {
		return time.Time{}
	}
	return e.Last
}

// Save writes the history file, keeping only the most recent entries.
func (h *History) Save() error {
	if h.path == "" {
		return nil
	}

	h.mu.Lock()
	list := make([]*Entry, 0, len(h.entries))
	for _, e := range h.entries {
		list = append(list, e)
	}
	h.mu.Unlock()

	sort.Slice(list, func(i, j int) bool { return list[i].Last.After(list[j].Last) })
	if len(list) > maxEntries {
		list = list[:maxEntries]
	}

	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(h.path, raw, 0o600)
}

// Ago renders a timestamp the way a list column wants it: short, and never
// wider than a few characters.
func Ago(t time.Time) string {
	if t.IsZero() {
		return "never"
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return itoa(int(d.Minutes())) + "m ago"
	case d < 24*time.Hour:
		return itoa(int(d.Hours())) + "h ago"
	case d < 30*24*time.Hour:
		return itoa(int(d.Hours()/24)) + "d ago"
	default:
		return t.Format("2006-01-02")
	}
}

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
