package watch

import "github.com/romanidis/watcheroo/internal/domain"

// PatternInput is one pattern to watch, as given: a glob, or a regular
// expression when Regex is set.
type PatternInput struct {
	Expr  string
	Regex bool
}

// WatchlistInput is what to watch, as given: the patterns in the order they
// were given, which is the order {} lists their files in.
type WatchlistInput struct {
	Patterns     []PatternInput
	Exclude      []string // globs
	ExcludeRegex []string
	Hidden       bool
}

// CommandInput is what to run, as given. With Shell, Argv is one script for
// sh -c.
type CommandInput struct {
	Argv  []string
	Shell bool
}

// newWatchlist parses in into the domain's Watchlist.
func newWatchlist(in WatchlistInput) (domain.Watchlist, error) {
	var patterns []domain.Pattern
	for _, p := range in.Patterns {
		pattern, err := newPattern(p)
		if err != nil {
			return domain.Watchlist{}, err
		}
		patterns = append(patterns, pattern)
	}
	var excludes []domain.Exclude
	for _, expr := range in.ExcludeRegex {
		e, err := domain.NewExcludeRegex(expr)
		if err != nil {
			return domain.Watchlist{}, err
		}
		excludes = append(excludes, e)
	}
	for _, glob := range in.Exclude {
		e, err := domain.NewExcludeGlob(glob)
		if err != nil {
			return domain.Watchlist{}, err
		}
		excludes = append(excludes, e)
	}
	return domain.NewWatchlist(patterns, excludes, in.Hidden)
}

// newPattern parses p into the domain's Pattern.
func newPattern(p PatternInput) (domain.Pattern, error) {
	if p.Regex {
		return domain.NewRegex(p.Expr)
	}
	return domain.NewGlob(p.Expr)
}

// look returns the files list names now.
func look(scanner Scanner, list domain.Watchlist) (domain.Snapshot, error) {
	matches, err := scanner.Scan(list)
	if err != nil {
		return domain.Snapshot{}, err
	}
	return domain.NewSnapshot(list, matches), nil
}
