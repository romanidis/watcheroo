// Package disk is where a watch looks at the files: it expands globs and
// walks directories on the local file system, under the rules of a
// domain.Watchlist, and stamps each file it finds. It is the one adapter
// for watch.Scanner. Polling is the design, not a stopgap (decided
// 2026-10-02): an event backend like fsnotify holds a file descriptor per
// watched file on macOS, and a watch here is a few files.
package disk

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/romanidis/watcheroo/internal/domain"
	"github.com/romanidis/watcheroo/internal/watch"
)

var _ watch.Scanner = Scanner{}

// Scanner looks at the files under the working directory, or wherever a
// glob points. It keeps nothing between looks: every look matches the
// patterns again, so a file created after the watch started is picked up as
// soon as a pattern matches it.
type Scanner struct{}

// Scan returns every file list matches now. A glob is expanded once, and a
// directory it matches walked. The regexes are walked together, one walk for
// each directory they start from (see domain.Pattern.WalkRoot), so that
// several regexes cost one walk, and an anchored one walks only its own
// directory.
func (Scanner) Scan(list domain.Watchlist) ([]domain.Match, error) {
	var matches []domain.Match
	patterns := list.Patterns()
	regexes := map[string][]int{} // the indexes of the regexes, by the directory their walk starts from
	for i, p := range patterns {
		if p.IsRegex() {
			root := p.WalkRoot()
			regexes[root] = append(regexes[root], i)
			continue
		}
		var opts []doublestar.GlobOption
		if !list.Hidden() {
			// As in a shell, a wildcard does not match a leading dot.
			opts = append(opts, doublestar.WithNoHidden())
		}
		paths, err := doublestar.FilepathGlob(p.Glob(), opts...)
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w", p.Glob(), err)
		}
		for _, path := range paths {
			if !p.Admits(path) {
				continue
			}
			found, err := walk(list, path, func(string) []int { return []int{i} })
			if err != nil {
				return nil, err
			}
			matches = append(matches, found...)
		}
	}
	for root, indexes := range regexes {
		found, err := walk(list, root, func(path string) []int {
			var matching []int
			for _, i := range indexes {
				if patterns[i].MatchesPath(path) {
					matching = append(matching, i)
				}
			}
			return matching
		})
		if err != nil {
			return nil, err
		}
		matches = append(matches, found...)
	}
	return matches, nil
}

// walk stamps root when it is a file, or every file under it when it is a
// directory, matched by the patterns whose indexes match returns for it.
// What list leaves out is skipped: all of root when an exclude names a
// directory above it, a directory and everything under it when list prunes
// it, and a file when list drops it. So is a file that disappears before it
// is stamped, as if it had not matched.
func walk(list domain.Watchlist, root string, match func(path string) []int) ([]domain.Match, error) {
	if list.ExcludesAbove(root) {
		return nil, nil
	}
	var matches []domain.Match
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if list.Prunes(root, path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || list.Drops(root, path) {
			return nil
		}
		indexes := match(path)
		if len(indexes) == 0 {
			return nil
		}
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		stamp := domain.NewStamp(info.ModTime(), info.Size())
		for _, i := range indexes {
			matches = append(matches, domain.Match{Path: path, Pattern: i, Stamp: stamp})
		}
		return nil
	})
	return matches, err
}
