package host

import (
	"testing"
	"time"
)

func TestSortByRecencyPutsRecentHostsFirst(t *testing.T) {
	hosts := load(t) // prod, dev
	matches := Filter(hosts, "", nil)

	now := time.Now()
	last := map[string]time.Time{
		"dev":  now.Add(-1 * time.Minute),
		"prod": now.Add(-2 * time.Hour),
	}

	SortByRecency(matches, func(a string) time.Time { return last[a] })

	if matches[0].Host.Alias != "dev" {
		t.Fatalf("the most recently used host should come first, got %q", matches[0].Host.Alias)
	}
}

func TestNeverUsedHostsSortAfterUsedOnes(t *testing.T) {
	hosts := load(t)
	matches := Filter(hosts, "", nil)

	// prod was used; dev never was.
	last := map[string]time.Time{"prod": time.Now().Add(-time.Hour)}

	SortByRecency(matches, func(a string) time.Time { return last[a] })

	if matches[0].Host.Alias != "prod" {
		t.Fatalf("a used host should outrank a never-used one, got %q first", matches[0].Host.Alias)
	}
	if matches[1].Host.Alias != "dev" {
		t.Fatalf("want the unused host second, got %q", matches[1].Host.Alias)
	}
}

func TestSortByRecencyIsStableForUnusedHosts(t *testing.T) {
	hosts := load(t)
	matches := Filter(hosts, "", nil)

	before := []string{matches[0].Host.Alias, matches[1].Host.Alias}

	// Nothing has ever been used, so the existing (alphabetical) order must
	// survive rather than being shuffled.
	SortByRecency(matches, func(string) time.Time { return time.Time{} })

	after := []string{matches[0].Host.Alias, matches[1].Host.Alias}
	if before[0] != after[0] || before[1] != after[1] {
		t.Fatalf("order changed for hosts with no history: %v -> %v", before, after)
	}
}
