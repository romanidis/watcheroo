package internal

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

// Watcher polls the files its patterns match and reports when any of them changes.
//
// The patterns are matched again on every poll, so a file created after the
// watch started is picked up as soon as a pattern matches it. Hidden files and
// directories, whose names start with a dot, are left out unless a glob names
// them with a leading dot of its own, or WithHidden says to watch them: that
// is where editors keep their swap and lock files, and where git keeps its
// objects. The backups and temporary files editors leave next to the files
// they save are left out too, unless a glob names them so; see editorJunk.
type Watcher struct {
	interval time.Duration
	// patterns are in the order they were given, which orders the files they
	// match; see Files.
	patterns []Pattern
	// regexes are the indexes in patterns of the regexes, grouped by the
	// directory their walk starts from; see walkRoot.
	regexes map[string][]int

	excludeGlobs   []string
	excludeRegexes []*regexp.Regexp
	withHidden     bool
	postpone       bool
	debounce       time.Duration
	restart        bool
	keys           <-chan Key
}

// Pattern names files to watch: Glob as doublestar.FilepathGlob matches it,
// so ** matches any number of directories and {a,b} either a or b, or, when
// Regex is set, every file under the working directory whose slash-separated
// path Regex matches.
type Pattern struct {
	Glob  string
	Regex *regexp.Regexp
}

// String returns the glob, or the regular expression.
func (p Pattern) String() string {
	if p.Regex != nil {
		return p.Regex.String()
	}
	return p.Glob
}

// Change is what Run calls back with: the files watched, and how they differ
// from the files of the call before.
type Change struct {
	Files []string // every file watched, in the order Files gives

	// Added, Removed and Modified are in path order. All three are empty for
	// the first call, and for one a KeyRun asks for when nothing changed.
	Added    []string
	Removed  []string
	Modified []string

	Unmatched []Pattern // the patterns that match no file
}

// Key is what someone watching asks Run for from the keyboard; see WithKeys.
type Key int

const (
	KeyRun   Key = iota // call back now, as for a change
	KeyStop             // stop the call going on
	KeyPause            // stop calling back for changes, or start again
	KeyQuit             // end the watch
)

// WatcherOption changes what a Watcher built by NewWatcher watches, or when it
// calls back.
type WatcherOption func(*Watcher)

// WithExcludes leaves out every file and directory that one of globs or
// regexes names, and everything under such a directory; see excludes.
func WithExcludes(globs []string, regexes []*regexp.Regexp) WatcherOption {
	return func(w *Watcher) {
		for _, glob := range globs {
			w.excludeGlobs = append(w.excludeGlobs, filepath.Clean(filepath.FromSlash(glob)))
		}
		w.excludeRegexes = append(w.excludeRegexes, regexes...)
	}
}

// WithHidden watches hidden files and directories as well.
func WithHidden() WatcherOption {
	return func(w *Watcher) {
		w.withHidden = true
	}
}

// WithPostpone leaves out the call Run makes at the start, so the first call
// waits for the first change.
func WithPostpone() WatcherOption {
	return func(w *Watcher) {
		w.postpone = true
	}
}

// WithDebounce waits until the files have stayed as they are for d before
// calling back, so a file written a piece at a time is read once, when whole.
func WithDebounce(d time.Duration) WatcherOption {
	return func(w *Watcher) {
		w.debounce = d
	}
}

// WithRestart makes a change that comes while a call is still going stop that
// call, cancelling its context and waiting for it to return, before making
// the next one. Without it, the next call waits for the one going on to end.
func WithRestart() WatcherOption {
	return func(w *Watcher) {
		w.restart = true
	}
}

// WithKeys makes Run do what each Key from keys asks for, as it comes.
func WithKeys(keys <-chan Key) WatcherOption {
	return func(w *Watcher) {
		w.keys = keys
	}
}

// NewWatcher builds a Watcher for patterns. A directory a glob matches stands
// for every file under it.
func NewWatcher(interval time.Duration, patterns []Pattern, opts ...WatcherOption) Watcher {
	byRoot := map[string][]int{}
	for i, p := range patterns {
		if p.Regex != nil {
			root := walkRoot(p.Regex.String())
			byRoot[root] = append(byRoot[root], i)
		}
	}
	w := Watcher{
		interval: interval,
		patterns: patterns,
		regexes:  byRoot,
	}
	for _, opt := range opts {
		opt(&w)
	}
	return w
}

// Files returns the files the patterns match now, in the order of the patterns
// that match them, and in path order among the files one pattern matches. A
// file two patterns match takes the place of the first.
func (w *Watcher) Files() ([]string, error) {
	found, _, err := w.scan()
	if err != nil {
		return nil, err
	}
	return ordered(found), nil
}

// Unmatched returns the patterns that match no file now.
func (w *Watcher) Unmatched() ([]Pattern, error) {
	_, matched, err := w.scan()
	if err != nil {
		return nil, err
	}
	return w.unmatched(matched), nil
}

// Run calls onChange in the background once at the start, and again each
// time a poll finds a watched file added, removed or modified, or a Key asks
// for it, until ctx is done or a KeyQuit comes. A poll that fails ends the
// watch with its error. Run returns only once the call going on has returned
// too, its context cancelled.
func (w *Watcher) Run(ctx context.Context, onChange func(ctx context.Context, c Change)) error {
	called, matched, err := w.scan() // the files as they were at the last call, or at the start
	if err != nil {
		return err
	}

	// The call going on: cancel stops it, and done is closed once it has returned.
	cancel, done := context.CancelFunc(func() {}), make(chan struct{})
	close(done)
	stop := func() {
		cancel()
		<-done
	}
	defer func() { stop() }()
	busy := func() bool {
		select {
		case <-done:
			return false
		default:
			return true
		}
	}
	call := func(prev, cur map[string]watched, matched []bool) {
		stop()
		c := w.change(prev, cur, matched)
		var callCtx context.Context
		callCtx, cancel = context.WithCancel(ctx)
		returned := make(chan struct{})
		done = returned
		go func() {
			defer close(returned)
			onChange(callCtx, c)
		}()
	}

	if !w.postpone {
		call(called, called, matched)
	}
	seen, changedAt := called, time.Now() // the files at the last poll, and when they last changed
	paused := false
	tick := time.NewTicker(w.interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case key := <-w.keys:
			switch key {
			case KeyRun:
				cur, matched, err := w.scan()
				if err != nil {
					return err
				}
				prev := called
				called = cur
				call(prev, cur, matched)
			case KeyStop:
				stop()
			case KeyPause:
				paused = !paused
			case KeyQuit:
				return nil
			}
		case <-tick.C:
			cur, matched, err := w.scan()
			if err != nil {
				return err
			}
			if !maps.Equal(seen, cur) {
				seen, changedAt = cur, time.Now()
			}
			if paused || maps.Equal(called, cur) || time.Since(changedAt) < w.debounce || busy() && !w.restart {
				continue
			}
			prev := called
			called = cur
			call(prev, cur, matched)
		}
	}
}

// change returns the Change from prev, the files of the call before, to cur,
// with matched saying which patterns match a file.
func (w *Watcher) change(prev, cur map[string]watched, matched []bool) Change {
	c := Change{Files: ordered(cur), Unmatched: w.unmatched(matched)}
	for path, f := range cur {
		if p, ok := prev[path]; !ok {
			c.Added = append(c.Added, path)
		} else if p.stamp != f.stamp {
			c.Modified = append(c.Modified, path)
		}
	}
	for path := range prev {
		if _, ok := cur[path]; !ok {
			c.Removed = append(c.Removed, path)
		}
	}
	slices.Sort(c.Added)
	slices.Sort(c.Removed)
	slices.Sort(c.Modified)
	return c
}

// unmatched returns the patterns matched says match no file.
func (w *Watcher) unmatched(matched []bool) []Pattern {
	var patterns []Pattern
	for i, ok := range matched {
		if !ok {
			patterns = append(patterns, w.patterns[i])
		}
	}
	return patterns
}

// stamp is what one poll compares with the last to tell that a file changed.
type stamp struct {
	modTime int64
	size    int64
}

// watched is what scan finds for a file: its stamp, and the index of the first
// pattern that matches it, which orders it among the others.
type watched struct {
	stamp
	pattern int
}

// ordered returns the files in found in the order of the patterns that match
// them, and in path order among the files one pattern matches.
func ordered(found map[string]watched) []string {
	return slices.SortedFunc(maps.Keys(found), func(a, b string) int {
		return cmp.Or(cmp.Compare(found[a].pattern, found[b].pattern), strings.Compare(a, b))
	})
}

// scan stamps every file a pattern matches now, and says which patterns
// match one.
func (w *Watcher) scan() (map[string]watched, []bool, error) {
	found := map[string]watched{}
	matched := make([]bool, len(w.patterns))
	for i, p := range w.patterns {
		if p.Regex != nil {
			continue
		}
		var opts []doublestar.GlobOption
		if !w.withHidden {
			// As in a shell, a wildcard does not match a leading dot.
			opts = append(opts, doublestar.WithNoHidden())
		}
		paths, err := doublestar.FilepathGlob(p.Glob, opts...)
		if err != nil {
			return nil, nil, fmt.Errorf("glob %q: %w", p.Glob, err)
		}
		for _, path := range paths {
			if junk(path) && !junk(p.Glob) {
				continue
			}
			match := func(string) (int, bool) {
				matched[i] = true
				return i, true
			}
			if err := w.addFiles(found, path, match); err != nil {
				return nil, nil, err
			}
		}
	}
	for root, indexes := range w.regexes {
		first := func(path string) (int, bool) {
			pattern, ok := 0, false
			for _, i := range indexes {
				if w.patterns[i].Regex.MatchString(filepath.ToSlash(path)) {
					matched[i] = true
					if !ok {
						pattern, ok = i, true
					}
				}
			}
			return pattern, ok
		}
		if err := w.addFiles(found, root, first); err != nil {
			return nil, nil, err
		}
	}
	return found, matched, nil
}

// addFiles stamps root when it is a file, or every file under it that match
// accepts when it is a directory, with the index of the pattern match says
// matched it. A file found already keeps the pattern that comes first. What
// an exclude names is skipped, as is root when an exclude names a directory
// above it. Hidden files and directories below root are skipped unless
// WithHidden says otherwise, editor junk below root is skipped, and so is a
// file that disappears before it is read, as if it had not matched.
func (w *Watcher) addFiles(found map[string]watched, root string, match func(path string) (pattern int, ok bool)) error {
	for dir := filepath.Dir(root); ; dir = filepath.Dir(dir) {
		if w.excludes(dir) {
			return nil
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if w.excludes(path) || path != root && hidden(path) && !w.withHidden {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || path != root && junk(path) {
			return nil
		}
		pattern, ok := match(path)
		if !ok {
			return nil
		}
		if f, ok := found[path]; ok && f.pattern <= pattern {
			return nil
		}
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		found[path] = watched{stamp{modTime: info.ModTime().UnixNano(), size: info.Size()}, pattern}
		return nil
	})
}

// excludes reports whether an exclude names path itself, leaving its
// directories to the caller. A glob with a separator in it is matched against
// the whole path, and one without against its last element, so node_modules
// names a directory of that name wherever it is. A regex is matched against
// the slash-separated path, as a watched one is.
func (w *Watcher) excludes(path string) bool {
	path = filepath.Clean(path)
	name := filepath.Base(path)
	if name == "." || name == ".." || name == string(filepath.Separator) {
		return false
	}
	for _, glob := range w.excludeGlobs {
		against := name
		if strings.ContainsRune(glob, filepath.Separator) {
			against = path
		}
		if ok, _ := doublestar.PathMatch(glob, against); ok {
			return true
		}
	}
	return slices.ContainsFunc(w.excludeRegexes, func(re *regexp.Regexp) bool {
		return re.MatchString(filepath.ToSlash(path))
	})
}

// editorJunk are the names of the files editors leave next to the ones they
// save: the backups and autosaves of emacs, and the temporary files of
// JetBrains IDEs.
var editorJunk = []string{"*~", "#*#", "*___jb_tmp___", "*___jb_old___"}

// junk reports whether the last element of path is a name in editorJunk. For
// a glob, that means the glob names such files on purpose, as '*~' does.
func junk(path string) bool {
	name := filepath.Base(path)
	return slices.ContainsFunc(editorJunk, func(pattern string) bool {
		ok, _ := filepath.Match(pattern, name)
		return ok
	})
}

// hidden reports whether the last element of path names a hidden file or directory.
func hidden(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(base, ".") && base != "." && base != ".."
}

// walkRoot returns the directory that every path expr can match lies under,
// so its walk can start there rather than at the top of the tree: the
// directories spelled out right after a leading ^, as data/2024 in
// ^data/2024/.*\.csv$. It returns "." for an expression that does not start
// that way.
func walkRoot(expr string) string {
	re, err := syntax.Parse(expr, syntax.Perl)
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
