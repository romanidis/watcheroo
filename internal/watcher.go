package internal

import (
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
)

// Watcher polls the files its patterns match and reports when any of them changes.
//
// The patterns are matched again on every poll, so a file created after the
// watch started is picked up as soon as a pattern matches it. Hidden files and
// directories, whose names start with a dot, are left out unless a glob names
// them with a leading dot of its own, or WithHidden says to watch them: that
// is where editors keep their swap and lock files, and where git keeps its
// objects.
type Watcher struct {
	interval time.Duration
	globs    []string
	// regexes are grouped by the directory their walk starts from; see walkRoot.
	regexes map[string][]*regexp.Regexp

	excludeGlobs   []string
	excludeRegexes []*regexp.Regexp
	withHidden     bool
	postpone       bool
	debounce       time.Duration
	restart        bool
}

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

// WithRestart makes Run call back in the background and go on watching
// meanwhile. A change that comes while a call is still going cancels its
// context and waits for it to return before making the next call.
func WithRestart() WatcherOption {
	return func(w *Watcher) {
		w.restart = true
	}
}

// NewWatcher builds a Watcher. Globs are matched as filepath.Glob matches them,
// and a directory one matches stands for every file under it. Regexes are
// matched against the slash-separated path of every file under the working
// directory.
func NewWatcher(interval time.Duration, globs []string, regexes []*regexp.Regexp, opts ...WatcherOption) Watcher {
	byRoot := map[string][]*regexp.Regexp{}
	for _, re := range regexes {
		root := walkRoot(re.String())
		byRoot[root] = append(byRoot[root], re)
	}
	w := Watcher{
		interval: interval,
		globs:    globs,
		regexes:  byRoot,
	}
	for _, opt := range opts {
		opt(&w)
	}
	return w
}

// Files returns the files the patterns match now, sorted.
func (w *Watcher) Files() ([]string, error) {
	found, err := w.scan()
	if err != nil {
		return nil, err
	}
	return slices.Sorted(maps.Keys(found)), nil
}

// Run calls onChange with the watched files, sorted, once at the start and
// again each time a poll finds one added, removed or modified, until ctx is
// done. A poll that fails ends the watch with its error. With WithRestart,
// Run returns only once the call in the background has returned too.
func (w *Watcher) Run(ctx context.Context, onChange func(ctx context.Context, files []string)) error {
	called, err := w.scan() // the files as they were at the last call, or at the start
	if err != nil {
		return err
	}

	stop := func() {} // cancels the call going on in the background, and waits for it
	defer func() { stop() }()
	call := func(found map[string]stamp) {
		files := slices.Sorted(maps.Keys(found))
		if !w.restart {
			onChange(ctx, files)
			return
		}
		stop()
		callCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			onChange(callCtx, files)
		}()
		stop = func() {
			cancel()
			<-done
		}
	}

	if !w.postpone {
		call(called)
	}
	seen, changedAt := called, time.Now() // the files at the last poll, and when they last changed
	tick := time.NewTicker(w.interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			cur, err := w.scan()
			if err != nil {
				return err
			}
			if !maps.Equal(seen, cur) {
				seen, changedAt = cur, time.Now()
			}
			if maps.Equal(called, cur) || time.Since(changedAt) < w.debounce {
				continue
			}
			called = cur
			call(cur)
		}
	}
}

// stamp is what one poll compares with the last to tell that a file changed.
type stamp struct {
	modTime int64
	size    int64
}

// scan stamps every file a pattern matches now.
func (w *Watcher) scan() (map[string]stamp, error) {
	found := map[string]stamp{}
	for _, glob := range w.globs {
		paths, err := filepath.Glob(glob)
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w", glob, err)
		}
		for _, path := range paths {
			// As in a shell, a wildcard does not match a leading dot.
			if hidden(path) && !hidden(glob) && !w.withHidden {
				continue
			}
			if err := w.addFiles(found, path, func(string) bool { return true }); err != nil {
				return nil, err
			}
		}
	}
	for root, res := range w.regexes {
		matches := func(path string) bool {
			return slices.ContainsFunc(res, func(re *regexp.Regexp) bool {
				return re.MatchString(filepath.ToSlash(path))
			})
		}
		if err := w.addFiles(found, root, matches); err != nil {
			return nil, err
		}
	}
	return found, nil
}

// addFiles stamps root when it is a file, or every file under it that match
// accepts when it is a directory. What an exclude names is skipped, as is
// root when an exclude names a directory above it. Hidden files and
// directories below root are skipped unless WithHidden says otherwise, and so
// is a file that disappears before it is read, as if it had not matched.
func (w *Watcher) addFiles(found map[string]stamp, root string, match func(path string) bool) error {
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
		if d.IsDir() || !match(path) {
			return nil
		}
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		found[path] = stamp{modTime: info.ModTime().UnixNano(), size: info.Size()}
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
		if ok, _ := filepath.Match(glob, against); ok {
			return true
		}
	}
	return slices.ContainsFunc(w.excludeRegexes, func(re *regexp.Regexp) bool {
		return re.MatchString(filepath.ToSlash(path))
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
