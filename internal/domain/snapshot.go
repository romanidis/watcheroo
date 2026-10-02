package domain

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"time"
)

// Stamp is what tells one look at a file from the next: when it was last
// modified, and its size. Two looks that stamp a file the same see the same
// file, as far as a watch can tell without reading it.
type Stamp struct {
	modTime int64
	size    int64
}

// NewStamp returns the Stamp of a file modified at modTime, size bytes long.
func NewStamp(modTime time.Time, size int64) Stamp {
	return Stamp{modTime: modTime.UnixNano(), size: size}
}

// Match is a file one pattern of a watchlist matches, as a look finds it.
// It is a record of what was found, with nothing to guard: Pattern is the
// index of the pattern in the watchlist's Patterns, and NewSnapshot makes
// sense of a file several patterns match.
type Match struct {
	Path    string
	Pattern int
	Stamp   Stamp
}

// Snapshot is the files a watchlist matches at one look, each stamped, and
// which of its patterns match none.
type Snapshot struct {
	patterns []Pattern
	files    map[string]seen
	matched  []bool // matched[i] is whether patterns[i] matches a file
}

// seen is what a Snapshot keeps of a file: its stamp, and the index of the
// first pattern that matches it, which orders it among the others.
type seen struct {
	stamp   Stamp
	pattern int
}

// NewSnapshot returns the Snapshot of a look at list that found matches. A
// file two patterns match takes the place of the first.
func NewSnapshot(list Watchlist, matches []Match) Snapshot {
	s := Snapshot{
		patterns: list.patterns,
		files:    make(map[string]seen, len(matches)),
		matched:  make([]bool, len(list.patterns)),
	}
	for _, m := range matches {
		s.matched[m.Pattern] = true
		if f, ok := s.files[m.Path]; ok && f.pattern <= m.Pattern {
			continue
		}
		s.files[m.Path] = seen{stamp: m.Stamp, pattern: m.Pattern}
	}
	return s
}

// Files returns the files, in the order of the patterns that match them, and
// in path order among the files one pattern matches. That is the order {}
// lists them in, so awk -f {} gets the script a pattern named first.
func (s Snapshot) Files() []string {
	return slices.SortedFunc(maps.Keys(s.files), func(a, b string) int {
		return cmp.Or(cmp.Compare(s.files[a].pattern, s.files[b].pattern), strings.Compare(a, b))
	})
}

// Unmatched returns the patterns that match no file, which may be typos.
func (s Snapshot) Unmatched() []Pattern {
	var patterns []Pattern
	for i, ok := range s.matched {
		if !ok {
			patterns = append(patterns, s.patterns[i])
		}
	}
	return patterns
}

// Same reports whether s and other found the same files, stamped the same,
// in the same places.
func (s Snapshot) Same(other Snapshot) bool {
	return maps.Equal(s.files, other.files)
}

// ChangeSince returns the Change from prev to s.
func (s Snapshot) ChangeSince(prev Snapshot) Change {
	c := Change{Files: s.Files(), Unmatched: s.Unmatched()}
	for path, f := range s.files {
		if p, ok := prev.files[path]; !ok {
			c.Added = append(c.Added, path)
		} else if p.stamp != f.stamp {
			c.Modified = append(c.Modified, path)
		}
	}
	for path := range prev.files {
		if _, ok := s.files[path]; !ok {
			c.Removed = append(c.Removed, path)
		}
	}
	slices.Sort(c.Added)
	slices.Sort(c.Removed)
	slices.Sort(c.Modified)
	return c
}

// Change is what a run is made for: the files watched, and how they differ
// from the files of the run before. It is worked out by
// Snapshot.ChangeSince and only read after, so its fields are plain.
type Change struct {
	Files []string // every file watched, in the order Snapshot.Files gives

	// Added, Removed and Modified are in path order. All three are empty for
	// the first run, and for one asked for when nothing changed.
	Added    []string
	Removed  []string
	Modified []string

	Unmatched []Pattern // the patterns that match no file
}
