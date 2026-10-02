package domain

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"regexp/syntax"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ErrBadGlob is a glob that does not parse. The error that wraps it names
// the glob.
var ErrBadGlob = errors.New("syntax error in pattern")

// Pattern names files to watch: a glob, in which ** matches any number of
// directories and {a,b} either a or b, or a regular expression matched
// against the slash-separated path of every file under the directory its
// walk starts from; see WalkRoot. A directory a glob matches stands for
// every file under it.
type Pattern struct {
	glob  string
	regex *regexp.Regexp
}

// NewGlob returns the Pattern for glob. Only its syntax is checked: a glob
// that matches nothing yet is no error, since the file it names may come
// later.
func NewGlob(glob string) (Pattern, error) {
	if !doublestar.ValidatePathPattern(glob) {
		return Pattern{}, fmt.Errorf("glob %q: %w", glob, ErrBadGlob)
	}
	return Pattern{glob: glob}, nil
}

// NewRegex returns the Pattern for the regular expression expr. For one that
// does not compile, it returns regexp's error, which says what is wrong.
func NewRegex(expr string) (Pattern, error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return Pattern{}, fmt.Errorf("regex: %w", err)
	}
	return Pattern{regex: re}, nil
}

// IsRegex reports whether p is a regular expression rather than a glob.
func (p Pattern) IsRegex() bool {
	return p.regex != nil
}

// Glob returns the glob, or "" for a regular expression.
func (p Pattern) Glob() string {
	return p.glob
}

// String returns the glob, or the regular expression.
func (p Pattern) String() string {
	if p.regex != nil {
		return p.regex.String()
	}
	return p.glob
}

// MatchesPath reports whether the regular expression matches path, a file a
// walk found. A glob matches nothing this way: the disk expands it.
func (p Pattern) MatchesPath(path string) bool {
	return p.regex != nil && p.regex.MatchString(filepath.ToSlash(path))
}

// Admits reports whether path, which the glob matched, is to be watched.
// Editor junk is not, unless the glob names such files on purpose, as '*~'
// does.
func (p Pattern) Admits(path string) bool {
	return !isEditorJunk(path) || isEditorJunk(p.glob)
}

// WalkRoot returns the directory that every path the regular expression can
// match lies under, so its walk can start there rather than at the top of
// the tree: the directories spelled out right after a leading ^, as
// data/2024 in ^data/2024/.*\.csv$. It returns "." for an expression that
// does not start that way.
func (p Pattern) WalkRoot() string {
	if p.regex == nil {
		return "."
	}
	re, err := syntax.Parse(p.regex.String(), syntax.Perl)
	if err != nil || re.Op != syntax.OpConcat || len(re.Sub) < 2 {
		return "."
	}
	anchor, lit := re.Sub[0], re.Sub[1]
	if anchor.Op != syntax.OpBeginText || lit.Op != syntax.OpLiteral || lit.Flags&syntax.FoldCase != 0 {
		return "."
	}
	prefix := string(lit.Rune)
	i := strings.LastIndex(prefix, "/")
	if i <= 0 {
		return "."
	}
	return filepath.FromSlash(prefix[:i])
}
