package domain_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/romanidis/watcheroo/internal/domain"
)

// stamp is a file's stamp at second s.
func stamp(s int) domain.Stamp {
	return domain.NewStamp(time.Unix(int64(s), 0), 1)
}

// watchlist returns the Watchlist of globs.
func watchlist(globs ...string) domain.Watchlist {
	var patterns []domain.Pattern
	for _, glob := range globs {
		patterns = append(patterns, must(domain.NewGlob(glob)))
	}
	return must(domain.NewWatchlist(patterns, nil, false))
}

// globs spells patterns.
func globs(patterns []domain.Pattern) []string {
	var out []string
	for _, p := range patterns {
		out = append(out, p.String())
	}
	return out
}

func TestSnapshotFiles(t *testing.T) {
	list := watchlist("b.csv", "*.csv", "report.awk", "reprot.awk")
	s := domain.NewSnapshot(list, []domain.Match{
		{Path: "c.csv", Pattern: 1, Stamp: stamp(1)},
		{Path: "b.csv", Pattern: 1, Stamp: stamp(1)},
		{Path: "a.csv", Pattern: 1, Stamp: stamp(1)},
		{Path: "b.csv", Pattern: 0, Stamp: stamp(1)},
		{Path: "report.awk", Pattern: 2, Stamp: stamp(1)},
	})
	if got, want := s.Files(), []string{"b.csv", "a.csv", "c.csv", "report.awk"}; !slices.Equal(got, want) {
		t.Errorf("listed %v, want the files in the order of their patterns, and in path order among one pattern's: %v", got, want)
	}
	if got, want := globs(s.Unmatched()), []string{"reprot.awk"}; !slices.Equal(got, want) {
		t.Errorf("unmatched %v, want %v", got, want)
	}
}

func TestSnapshotChangeSince(t *testing.T) {
	list := watchlist("*.txt", "later.csv")
	prev := domain.NewSnapshot(list, []domain.Match{
		{Path: "a.txt", Pattern: 0, Stamp: stamp(1)},
		{Path: "b.txt", Pattern: 0, Stamp: stamp(1)},
	})
	cur := domain.NewSnapshot(list, []domain.Match{
		{Path: "a.txt", Pattern: 0, Stamp: stamp(2)},
		{Path: "c.txt", Pattern: 0, Stamp: stamp(2)},
	})
	got := cur.ChangeSince(prev)
	want := domain.Change{
		Files:     []string{"a.txt", "c.txt"},
		Added:     []string{"c.txt"},
		Removed:   []string{"b.txt"},
		Modified:  []string{"a.txt"},
		Unmatched: []domain.Pattern{must(domain.NewGlob("later.csv"))},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("changed as %+v, want %+v", got, want)
	}
	if cur.Same(prev) || !cur.Same(cur) {
		t.Error("want a snapshot the same as itself only")
	}
}
