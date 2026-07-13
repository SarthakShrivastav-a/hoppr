package host

import (
	"sort"
	"time"
)

// SortByRecency reorders matches so the hosts you actually use come first.
//
// It only applies when there is no search query. Once someone is typing, the
// fuzzy score is what they are steering by, and reordering underneath them
// would make the list feel like it is fighting back.
//
// With no query, though, ranking alphabetically is close to ranking randomly:
// the two boxes you touch every day sit wherever the alphabet put them, below
// six you provisioned once and forgot. Most-recently-used is what a picker is
// for.
//
// Hosts that have never been connected to keep their existing order, below the
// ones that have. A brand-new host does not deserve to outrank the box you were
// on ten minutes ago just because nothing is known about it.
func SortByRecency(matches []Match, lastUsed func(alias string) time.Time) {
	sort.SliceStable(matches, func(i, j int) bool {
		a := lastUsed(matches[i].Host.Alias)
		b := lastUsed(matches[j].Host.Alias)

		switch {
		case a.IsZero() && b.IsZero():
			// Neither has been used: leave them as they were, which is the
			// stable alphabetical order Filter produced.
			return false
		case a.IsZero():
			return false // unused sorts after used
		case b.IsZero():
			return true
		default:
			return a.After(b)
		}
	})
}
