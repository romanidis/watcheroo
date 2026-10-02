package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/romanidis/watcheroo/internal/domain"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestPatternErrors(t *testing.T) {
	if _, err := domain.NewGlob("["); !errors.Is(err, domain.ErrBadGlob) || !strings.Contains(err.Error(), `glob "["`) {
		t.Errorf(`NewGlob("[") returned %v, want ErrBadGlob naming the glob`, err)
	}
	if _, err := domain.NewExcludeGlob("["); !errors.Is(err, domain.ErrBadGlob) || !strings.Contains(err.Error(), `exclude "["`) {
		t.Errorf(`NewExcludeGlob("[") returned %v, want ErrBadGlob naming the exclude`, err)
	}
	if _, err := domain.NewRegex("("); err == nil || !strings.HasPrefix(err.Error(), "regex: error parsing regexp") {
		t.Errorf(`NewRegex("(") returned %v, want regexp's error`, err)
	}
	if _, err := domain.NewExcludeRegex("("); err == nil || !strings.HasPrefix(err.Error(), "exclude regex: error parsing regexp") {
		t.Errorf(`NewExcludeRegex("(") returned %v, want regexp's error`, err)
	}
	if _, err := domain.NewGlob("later.txt"); err != nil {
		t.Errorf("NewGlob for a file that does not exist yet returned %v, want none", err)
	}
	if _, err := domain.NewWatchlist(nil, nil, false); !errors.Is(err, domain.ErrNothingToWatch) {
		t.Errorf("NewWatchlist with no pattern returned %v, want ErrNothingToWatch", err)
	}
}

func TestPatternAdmits(t *testing.T) {
	tests := []struct {
		glob string
		path string
		want bool
	}{
		{glob: "*", path: "report.awk", want: true},
		{glob: "*", path: "report.awk~", want: false},
		{glob: "data/*", path: "data/#jan.csv#", want: false},
		{glob: "*", path: "x___jb_tmp___", want: false},
		{glob: "*~", path: "report.awk~", want: true},
		{glob: "data/#*#", path: "data/#jan.csv#", want: true},
	}
	for _, tt := range tests {
		if got := must(domain.NewGlob(tt.glob)).Admits(tt.path); got != tt.want {
			t.Errorf("%s admits %s: %v, want %v", tt.glob, tt.path, got, tt.want)
		}
	}
}

func TestPatternWalkRoot(t *testing.T) {
	tests := []struct {
		expr string
		want string
	}{
		{expr: `\.csv$`, want: "."},
		{expr: `data/.*\.csv$`, want: "."},
		{expr: `^data`, want: "."},
		{expr: `^data/`, want: "data"},
		{expr: `^data/.*\.csv$`, want: "data"},
		{expr: `^data/2024/jan`, want: "data/2024"},
		{expr: `^data/a|^data/b`, want: "."},
		{expr: `^data/|^logs/`, want: "."},
		{expr: `^a|ab/c`, want: "."},
		{expr: `(?i)^data/`, want: "."},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			if got := must(domain.NewRegex(tt.expr)).WalkRoot(); got != tt.want {
				t.Errorf("WalkRoot of %q = %q, want %q", tt.expr, got, tt.want)
			}
		})
	}
}

func TestWatchlistLeavesOut(t *testing.T) {
	list := must(domain.NewWatchlist(
		[]domain.Pattern{must(domain.NewGlob("data"))},
		[]domain.Exclude{
			must(domain.NewExcludeGlob("node_modules")),
			must(domain.NewExcludeGlob("data/*.md")),
			must(domain.NewExcludeRegex(`^data/old/`)),
		},
		false,
	))
	prunes := []struct {
		path string
		want bool
	}{
		{path: "data", want: false},
		{path: "data/x/node_modules", want: true},
		{path: "data/notes.md", want: true},
		{path: "notes.md", want: false},
		{path: "data/old/dec.csv", want: true},
		{path: "data/.cache", want: true},
	}
	for _, tt := range prunes {
		if got := list.Prunes("data", tt.path); got != tt.want {
			t.Errorf("the walk from data prunes %s: %v, want %v", tt.path, got, tt.want)
		}
	}
	if list.Prunes(".env", ".env") {
		t.Error("pruned the root of a walk for being hidden, though a glob named it")
	}
	if !list.Drops("data", "data/a.csv~") || list.Drops("a.csv~", "a.csv~") {
		t.Error("want editor junk dropped below the root of a walk, and not as the root")
	}
	if !list.ExcludesAbove("data/x/node_modules/pkg/a.js") || list.ExcludesAbove("data/x/a.js") {
		t.Error("want a file under an excluded directory left out with it, and no other")
	}
	hidden := must(domain.NewWatchlist([]domain.Pattern{must(domain.NewGlob("*"))}, nil, true))
	if hidden.Prunes(".", ".git") {
		t.Error("pruned a hidden directory from a watchlist that takes hidden files")
	}
}
