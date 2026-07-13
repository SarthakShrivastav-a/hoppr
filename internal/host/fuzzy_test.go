package host

import "testing"

func TestFuzzyIsSubsequenceNotSubstring(t *testing.T) {
	// The whole point: "prd" is not a substring of "prod-api", but it is the
	// kind of thing people actually type.
	if _, _, ok := fuzzyScore("prd", "prod-api"); !ok {
		t.Error(`"prd" should match "prod-api"`)
	}
	if _, _, ok := fuzzyScore("papi", "prod-api"); !ok {
		t.Error(`"papi" should match "prod-api"`)
	}
	if _, _, ok := fuzzyScore("xyz", "prod-api"); ok {
		t.Error(`"xyz" should not match "prod-api"`)
	}
}

func TestFuzzyIsCaseInsensitive(t *testing.T) {
	if _, _, ok := fuzzyScore("PROD", "prod-api"); !ok {
		t.Error("matching should ignore case")
	}
}

func TestFuzzyPrefersConsecutiveRuns(t *testing.T) {
	// "api" appears as a solid run in "api-1" and scattered in "a-p-i".
	run, _, _ := fuzzyScore("api", "api-1")
	scattered, _, _ := fuzzyScore("api", "a-p-i")
	if run <= scattered {
		t.Errorf("a consecutive run (%d) should outscore a scattered match (%d)", run, scattered)
	}
}

func TestFuzzyPrefersWordStarts(t *testing.T) {
	// In "prod-api" the 'a' starts a word; in "prodxapi" it does not.
	boundary, _, _ := fuzzyScore("pa", "prod-api")
	middle, _, _ := fuzzyScore("pa", "prodxapi")
	if boundary <= middle {
		t.Errorf("a word-start match (%d) should outscore a mid-word one (%d)", boundary, middle)
	}
}

func TestFuzzyPrefersShorterNames(t *testing.T) {
	short, _, _ := fuzzyScore("api", "api")
	long, _, _ := fuzzyScore("api", "api-staging-canary-2")
	if short <= long {
		t.Errorf("an exact short name (%d) should outscore a long one (%d)", short, long)
	}
}

func TestFuzzyReportsMatchPositions(t *testing.T) {
	_, pos, ok := fuzzyScore("pa", "prod-api")
	if !ok {
		t.Fatal("expected a match")
	}
	// 'p' at 0, then 'a' at 5 ("prod-|a|pi") — the highlighter needs these.
	if len(pos) != 2 || pos[0] != 0 || pos[1] != 5 {
		t.Fatalf("want positions [0 5], got %v", pos)
	}
}

func TestFilterRanksBestMatchFirst(t *testing.T) {
	hosts := load(t)
	got := Filter(hosts, "prod", nil)

	if len(got) == 0 {
		t.Fatal("no matches")
	}
	if got[0].Host.Alias != "prod" {
		t.Errorf("want prod ranked first, got %q", got[0].Host.Alias)
	}
}

func TestFilterSearchesTargetAndTags(t *testing.T) {
	hosts := load(t)

	// By IP, which lives in HostName, not in the alias.
	if got := Filter(hosts, "203.0.113", nil); len(got) != 1 || got[0].Host.Alias != "prod" {
		t.Errorf("searching by IP should find prod, got %v", aliases(got))
	}
	// By tag.
	if got := Filter(hosts, "aws", nil); len(got) != 1 || got[0].Host.Alias != "prod" {
		t.Errorf("searching by tag should find prod, got %v", aliases(got))
	}
	// By note.
	if got := Filter(hosts, "API box", nil); len(got) != 1 || got[0].Host.Alias != "prod" {
		t.Errorf("searching by note should find prod, got %v", aliases(got))
	}
}

func TestFilterTagsAreAndedWithQuery(t *testing.T) {
	hosts := load(t)

	if got := Filter(hosts, "", []string{"aws"}); len(got) != 1 || got[0].Host.Alias != "prod" {
		t.Errorf("tag filter alone should yield prod, got %v", aliases(got))
	}
	// dev is not tagged aws, so this must be empty even though "dev" matches.
	if got := Filter(hosts, "dev", []string{"aws"}); len(got) != 0 {
		t.Errorf("tag and query should AND, got %v", aliases(got))
	}
}

func TestEmptyQueryReturnsEverything(t *testing.T) {
	hosts := load(t)
	if got := Filter(hosts, "", nil); len(got) != len(hosts) {
		t.Errorf("an empty query should return all %d hosts, got %d", len(hosts), len(got))
	}
}

func aliases(ms []Match) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.Host.Alias)
	}
	return out
}
