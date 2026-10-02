package domain

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ErrNothingToWatch is a watchlist with no pattern in it.
var ErrNothingToWatch = errors.New("nothing to watch: give a glob or a regex")

// Watchlist is what a watch looks at: its patterns, in the order they were
// given, which orders the files they match, and what it leaves out of what
// they match.
//
// Left out are what an exclude names; hidden files and directories, whose
// names start with a dot, unless a glob names them with a dot of its own or
// the watchlist takes hidden files too, since that is where editors keep
// their swap and lock files, and where git keeps its objects; and the
// backups and temporary files editors leave next to the files they save,
// unless a glob names them so; see editorJunk.
type Watchlist struct {
	patterns []Pattern
	excludes []Exclude
	hidden   bool
}

// NewWatchlist returns the Watchlist for patterns, leaving out what excludes
// name, and taking hidden files too when hidden is set.
func NewWatchlist(patterns []Pattern, excludes []Exclude, hidden bool) (Watchlist, error) {
	if len(patterns) == 0 {
		return Watchlist{}, ErrNothingToWatch
	}
	return Watchlist{patterns: slices.Clone(patterns), excludes: slices.Clone(excludes), hidden: hidden}, nil
}

// Patterns returns the patterns, in the order they were given.
func (w Watchlist) Patterns() []Pattern {
	return slices.Clone(w.patterns)
}

// Hidden reports whether hidden files are watched too, which lets a wildcard
// match a leading dot.
func (w Watchlist) Hidden() bool {
	return w.hidden
}

// Prunes reports whether the walk from root leaves out path, and everything
// under it when it is a directory: what an exclude names, and, below root, a
// hidden file or directory, unless the watchlist takes them.
func (w Watchlist) Prunes(root, path string) bool {
	return w.excludesPath(path) || path != root && isHidden(path) && !w.hidden
}

// Drops reports whether the walk from root leaves out path, a file it found:
// editor junk below root. Root itself was named on purpose.
func (w Watchlist) Drops(root, path string) bool {
	return path != root && isEditorJunk(path)
}

// ExcludesAbove reports whether an exclude names a directory above path,
// which leaves path out with it.
func (w Watchlist) ExcludesAbove(path string) bool {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if w.excludesPath(dir) {
			return true
		}
		if filepath.Dir(dir) == dir {
			return false
		}
	}
}

// excludesPath reports whether an exclude names path itself, leaving its
// directories to the caller.
func (w Watchlist) excludesPath(path string) bool {
	path = filepath.Clean(path)
	name := filepath.Base(path)
	if name == "." || name == ".." || name == string(filepath.Separator) {
		return false
	}
	return slices.ContainsFunc(w.excludes, func(e Exclude) bool {
		return e.names(path, name)
	})
}

// Exclude names files and directories a watch leaves out, and everything
// under such a directory. A glob with a separator in it is matched against
// the whole path, and one without against each name along it, so
// node_modules names a directory of that name wherever it is. A regex is
// matched against the slash-separated path, as a watched one is.
type Exclude struct {
	glob  string
	regex *regexp.Regexp
}

// NewExcludeGlob returns the Exclude for glob.
func NewExcludeGlob(glob string) (Exclude, error) {
	if !doublestar.ValidatePathPattern(glob) {
		return Exclude{}, fmt.Errorf("exclude %q: %w", glob, ErrBadGlob)
	}
	return Exclude{glob: filepath.Clean(filepath.FromSlash(glob))}, nil
}

// NewExcludeRegex returns the Exclude for the regular expression expr.
func NewExcludeRegex(expr string) (Exclude, error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return Exclude{}, fmt.Errorf("exclude regex: %w", err)
	}
	return Exclude{regex: re}, nil
}

// names reports whether e names path, whose last element is name.
func (e Exclude) names(path, name string) bool {
	if e.regex != nil {
		return e.regex.MatchString(filepath.ToSlash(path))
	}
	against := name
	if strings.ContainsRune(e.glob, filepath.Separator) {
		against = path
	}
	ok, _ := doublestar.PathMatch(e.glob, against)
	return ok
}

// editorJunk are the names of the files editors leave next to the ones they
// save: the backups and autosaves of emacs, and the temporary files of
// JetBrains IDEs. vim's 4913 is not among them: a real file of that name
// would go missing, and vim deletes its own within milliseconds.
var editorJunk = []string{"*~", "#*#", "*___jb_tmp___", "*___jb_old___"}

// isEditorJunk reports whether the last element of path is a name in
// editorJunk. For a glob, that means the glob names such files on purpose,
// as '*~' does.
func isEditorJunk(path string) bool {
	name := filepath.Base(path)
	return slices.ContainsFunc(editorJunk, func(pattern string) bool {
		ok, _ := filepath.Match(pattern, name)
		return ok
	})
}

// isHidden reports whether the last element of path names a hidden file or
// directory.
func isHidden(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(base, ".") && base != "." && base != ".."
}
