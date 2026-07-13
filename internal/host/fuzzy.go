package host

import (
	"sort"
	"strings"
	"unicode"
)

// Scoring weights. The absolute numbers do not matter; the ratios do.
const (
	scoreMatch          = 16 // a character matched at all
	bonusConsecutive    = 12 // matched immediately after the previous match
	bonusWordStart      = 10 // matched at the start of a word (after -, _, ., /)
	bonusCamel          = 8  // matched at a lowercase-to-uppercase boundary
	bonusFirstChar      = 8  // matched the very first character of the text
	penaltyLeadingSkip  = 2  // charged per character skipped before the first match
	maxLeadingSkipPenal = 20 // but only up to a point
)

// Match is a host that matched a query, with the score and the matched rune
// offsets so the UI can highlight them.
type Match struct {
	Host      *Host
	Score     int
	Positions []int // rune offsets into the matched field
	Field     string
}

// fuzzyScore scores query against text as a subsequence match, fzf-style.
//
// Substring matching is what users file bugs about: typing "prd" should find
// "prod-api", and typing "papi" should find "prod-api" too. A subsequence match
// gets that. The bonuses are what stop it from feeling random — without them,
// "ap" scores the same against "prod-api" and "a-random-p", and the list jumps
// around as you type.
//
// Reports ok=false when query is not a subsequence of text at all.
func fuzzyScore(query, text string) (score int, positions []int, ok bool) {
	if query == "" {
		return 0, nil, true
	}

	q := []rune(strings.ToLower(query))
	t := []rune(text)
	tl := []rune(strings.ToLower(text))

	qi := 0
	prevMatch := -2

	for ti := 0; ti < len(tl) && qi < len(q); ti++ {
		if tl[ti] != q[qi] {
			continue
		}

		s := scoreMatch
		if ti == prevMatch+1 {
			s += bonusConsecutive
		}
		if ti == 0 {
			s += bonusFirstChar
		} else {
			switch {
			case isBoundary(t[ti-1]):
				s += bonusWordStart
			case unicode.IsLower(t[ti-1]) && unicode.IsUpper(t[ti]):
				s += bonusCamel
			}
		}
		if qi == 0 && ti > 0 {
			p := ti * penaltyLeadingSkip
			if p > maxLeadingSkipPenal {
				p = maxLeadingSkipPenal
			}
			s -= p
		}

		score += s
		positions = append(positions, ti)
		prevMatch = ti
		qi++
	}

	if qi < len(q) {
		return 0, nil, false
	}
	// All else equal, prefer the shorter host name: "api" should rank "api"
	// above "api-staging-canary-2".
	score -= len(t) / 4
	return score, positions, true
}

func isBoundary(r rune) bool {
	switch r {
	case '-', '_', '.', '/', ' ', '@', ':':
		return true
	}
	return false
}

// Filter returns the hosts matching query and every tag in tags, best first.
//
// Tags are AND-ed with each other and with the query: selecting the "prod" tag
// and typing "api" means prod AND api, which is what people expect from a
// filter bar.
func Filter(hosts []*Host, query string, tags []string) []Match {
	var out []Match

	for _, h := range hosts {
		if !hasAllTags(h, tags) {
			continue
		}

		m, ok := best(h, query)
		if !ok {
			continue
		}
		out = append(out, m)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		// Stable, predictable tiebreak so the list never shuffles on redraw.
		return out[i].Host.Alias < out[j].Host.Alias
	})
	return out
}

// best scores the query against every field worth searching and keeps the
// strongest hit.
//
// Searching the target and tags — not just the alias — is what lets someone who
// remembers "the box at 203.0.113.10" or "the one tagged staging" find it
// without remembering what they named it.
func best(h *Host, query string) (Match, bool) {
	if query == "" {
		return Match{Host: h, Field: "alias"}, true
	}

	type field struct {
		name  string
		text  string
		scale int // percent; non-alias fields are worth slightly less
	}
	fields := []field{
		{"alias", h.Alias, 100},
		{"target", h.Target(), 80},
		{"note", h.Meta.Note, 60},
	}
	for _, a := range h.Aliases[1:] {
		fields = append(fields, field{"alias", a, 90})
	}
	for _, t := range h.Tags() {
		fields = append(fields, field{"tag", t, 70})
	}

	found := false
	var bestM Match
	for _, f := range fields {
		if f.text == "" {
			continue
		}
		s, pos, ok := fuzzyScore(query, f.text)
		if !ok {
			continue
		}
		s = s * f.scale / 100
		if !found || s > bestM.Score {
			bestM = Match{Host: h, Score: s, Positions: pos, Field: f.name}
			found = true
		}
	}
	return bestM, found
}

func hasAllTags(h *Host, tags []string) bool {
	for _, t := range tags {
		if !h.Meta.HasTag(t) {
			return false
		}
	}
	return true
}

// sortStrings sorts case-insensitively, which is how a tag list should read.
func sortStrings(s []string) {
	sort.Slice(s, func(i, j int) bool {
		return strings.ToLower(s[i]) < strings.ToLower(s[j])
	})
}
