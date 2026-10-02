package internal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// writeFiles creates each file under the working directory, with its directories.
func writeFiles(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWatcherScan(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFiles(t,
		"a.txt", "b.txt", "c.csv", ".env", ".a.txt.swp", "b.txt~",
		"data/jan.csv", "data/feb.csv", "data/notes.md", "data/.jan.csv.swp", "data/#jan.csv#", "data/old/dec.csv",
		".git/config", ".git/objects/ab/cdef",
	)

	tests := []struct {
		name    string
		globs   []string
		regexes []string
		opts    []WatcherOption
		want    []string
	}{
		{
			name:  "a plain name",
			globs: []string{"a.txt"},
			want:  []string{"a.txt"},
		},
		{
			name:  "a name that does not exist yet",
			globs: []string{"later.txt"},
			want:  nil,
		},
		{
			name:  "a wildcard",
			globs: []string{"*.txt"},
			want:  []string{"a.txt", "b.txt"},
		},
		{
			name:  "a wildcard in a directory",
			globs: []string{"data/*.csv"},
			want:  []string{"data/feb.csv", "data/jan.csv"},
		},
		{
			name:  "a wildcard skips hidden files",
			globs: []string{"*", "data/*"},
			want: []string{
				"a.txt", "b.txt", "c.csv",
				"data/feb.csv", "data/jan.csv", "data/notes.md", "data/old/dec.csv",
			},
		},
		{
			name:  "** matches any number of directories",
			globs: []string{"data/**/*.csv"},
			want:  []string{"data/feb.csv", "data/jan.csv", "data/old/dec.csv"},
		},
		{
			name:  "** does not go into hidden directories",
			globs: []string{"**/config"},
			want:  nil,
		},
		{
			name:  "braces match either name",
			globs: []string{"*.{txt,csv}"},
			want:  []string{"a.txt", "b.txt", "c.csv"},
		},
		{
			name:    "editor backups are left out of what a regex matches",
			regexes: []string{`jan`},
			want:    []string{"data/jan.csv"},
		},
		{
			name:  "editor backups a glob names on purpose",
			globs: []string{"*~", "data/#*#"},
			want:  []string{"b.txt~", "data/#jan.csv#"},
		},
		{
			name:  "a hidden file named with its dot",
			globs: []string{".env", ".*.swp"},
			want:  []string{".env", ".a.txt.swp"},
		},
		{
			name:  "a directory stands for every file under it",
			globs: []string{"data"},
			want:  []string{"data/feb.csv", "data/jan.csv", "data/notes.md", "data/old/dec.csv"},
		},
		{
			name:  "a hidden directory named with its dot",
			globs: []string{".git"},
			want:  []string{".git/config", ".git/objects/ab/cdef"},
		},
		{
			name:    "a regex reaches into directories",
			regexes: []string{`\.csv$`},
			want:    []string{"c.csv", "data/feb.csv", "data/jan.csv", "data/old/dec.csv"},
		},
		{
			name:    "a regex matches files, not directories",
			regexes: []string{`^data`},
			want:    []string{"data/feb.csv", "data/jan.csv", "data/notes.md", "data/old/dec.csv"},
		},
		{
			name:    "a regex skips hidden files and directories",
			regexes: []string{`config|cdef|env|swp`},
			want:    nil,
		},
		{
			name:    "an anchored regex",
			regexes: []string{`^data/old/`},
			want:    []string{"data/old/dec.csv"},
		},
		{
			name:    "globs and regexes together, overlapping",
			globs:   []string{"a.txt", "c.csv"},
			regexes: []string{`\.md$`, `^c\.`},
			want:    []string{"a.txt", "c.csv", "data/notes.md"},
		},
		{
			name:  "--hidden lets a wildcard match hidden files",
			globs: []string{"*"},
			opts:  []WatcherOption{WithHidden()},
			want: []string{
				".a.txt.swp", ".env", ".git/config", ".git/objects/ab/cdef",
				"a.txt", "b.txt", "c.csv",
				"data/.jan.csv.swp", "data/feb.csv", "data/jan.csv", "data/notes.md", "data/old/dec.csv",
			},
		},
		{
			name:  "--hidden lets ** go into hidden directories",
			globs: []string{"**/config"},
			opts:  []WatcherOption{WithHidden()},
			want:  []string{".git/config"},
		},
		{
			name:    "--hidden lets a regex reach into hidden directories",
			regexes: []string{`config|env`},
			opts:    []WatcherOption{WithHidden()},
			want:    []string{".env", ".git/config"},
		},
		{
			name:  "an exclude without a slash names a directory wherever it is",
			globs: []string{"data"},
			opts:  []WatcherOption{WithExcludes([]string{"old"}, nil)},
			want:  []string{"data/feb.csv", "data/jan.csv", "data/notes.md"},
		},
		{
			name:    "an exclude without a slash is a glob for names",
			regexes: []string{`\.csv$`},
			opts:    []WatcherOption{WithExcludes([]string{"j*"}, nil)},
			want:    []string{"c.csv", "data/feb.csv", "data/old/dec.csv"},
		},
		{
			name:  "an exclude with a slash is matched against the whole path",
			globs: []string{"*.txt", "data"},
			opts:  []WatcherOption{WithExcludes([]string{"data/*.md", "a.txt/"}, nil)},
			want:  []string{"b.txt", "data/feb.csv", "data/jan.csv", "data/old/dec.csv"},
		},
		{
			name:  "an exclude leaves out what a glob matches under it",
			globs: []string{"data/old/*.csv", "./data/*.csv"},
			opts:  []WatcherOption{WithExcludes([]string{"./data/old", "feb.csv"}, nil)},
			want:  []string{"data/jan.csv"},
		},
		{
			name:  "an exclude with **",
			globs: []string{"data"},
			opts:  []WatcherOption{WithExcludes([]string{"**/old"}, nil)},
			want:  []string{"data/feb.csv", "data/jan.csv", "data/notes.md"},
		},
		{
			name:    "an exclude regex",
			regexes: []string{`\.csv$`},
			opts:    []WatcherOption{WithExcludes(nil, []*regexp.Regexp{regexp.MustCompile(`^data/old/|^c`)})},
			want:    []string{"data/feb.csv", "data/jan.csv"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var patterns []Pattern
			for _, glob := range tt.globs {
				patterns = append(patterns, Pattern{Glob: glob})
			}
			for _, expr := range tt.regexes {
				patterns = append(patterns, Pattern{Regex: regexp.MustCompile(expr)})
			}
			w := NewWatcher(time.Millisecond, patterns, tt.opts...)
			got, err := w.Files()
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("matched %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWatcherOrder(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFiles(t, "process.awk", "data.tsv", "a.csv", "b.csv", "logs/y.log", "logs/z.log")
	re := regexp.MustCompile

	tests := []struct {
		name     string
		patterns []Pattern
		want     []string
	}{
		{
			name:     "files in the order of their patterns",
			patterns: []Pattern{{Glob: "process.awk"}, {Glob: "data.tsv"}},
			want:     []string{"process.awk", "data.tsv"},
		},
		{
			name:     "the files one glob matches in path order",
			patterns: []Pattern{{Glob: "data.tsv"}, {Glob: "*.csv"}},
			want:     []string{"data.tsv", "a.csv", "b.csv"},
		},
		{
			name:     "a file two globs match in the place of the first",
			patterns: []Pattern{{Glob: "b.csv"}, {Glob: "*.csv"}},
			want:     []string{"b.csv", "a.csv"},
		},
		{
			name:     "a file a regex and a later glob match in the place of the regex",
			patterns: []Pattern{{Regex: re(`\.awk$`)}, {Glob: "data.tsv"}, {Glob: "process.awk"}},
			want:     []string{"process.awk", "data.tsv"},
		},
		{
			name:     "regexes walked from one directory",
			patterns: []Pattern{{Regex: re(`\.awk$`)}, {Regex: re(`\.tsv$`)}},
			want:     []string{"process.awk", "data.tsv"},
		},
		{
			name:     "regexes walked from different directories",
			patterns: []Pattern{{Regex: re(`^logs/`)}, {Glob: "data.tsv"}, {Regex: re(`\.csv$`)}},
			want:     []string{"logs/y.log", "logs/z.log", "data.tsv", "a.csv", "b.csv"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewWatcher(time.Millisecond, tt.patterns)
			got, err := w.Files()
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("listed %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWalkRoot(t *testing.T) {
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
			if got := walkRoot(tt.expr); got != tt.want {
				t.Errorf("walkRoot(%q) = %q, want %q", tt.expr, got, tt.want)
			}
		})
	}
}

// watchTxt starts a Watcher with opts on *.txt in the working directory,
// polling every 10ms, and returns the files of each call it makes, and a
// function that stops it and checks it ended well.
func watchTxt(t *testing.T, opts ...WatcherOption) (runs chan []string, stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	runs = make(chan []string, 100)
	w := NewWatcher(10*time.Millisecond, []Pattern{{Glob: "*.txt"}}, opts...)
	done := make(chan error)
	go func() { done <- w.Run(ctx, func(_ context.Context, c Change) { runs <- c.Files }) }()
	stop = func() {
		t.Helper()
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Run returned %v, want nil once its context is done", err)
		}
	}
	t.Cleanup(cancel)
	return runs, stop
}

// expectRun fails the test unless runs has a call with want soon.
func expectRun(t *testing.T, runs chan []string, why string, want ...string) {
	t.Helper()
	select {
	case files := <-runs:
		if !slices.Equal(files, want) {
			t.Errorf("after %s, ran with %v, want %v", why, files, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no run after %s", why)
	}
}

// expectNoRun fails the test if runs has a call within 100ms.
func expectNoRun(t *testing.T, runs chan []string, why string) {
	t.Helper()
	select {
	case <-runs:
		t.Fatalf("ran after %s", why)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestWatcherRun(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFiles(t, "a.txt", "other.log")
	runs, stop := watchTxt(t)

	expectRun(t, runs, "starting", "a.txt")
	expectNoRun(t, runs, "nothing changed")

	if err := os.WriteFile("a.txt", []byte("changed, and longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	expectRun(t, runs, "a watched file changed", "a.txt")

	writeFiles(t, "b.txt")
	expectRun(t, runs, "a file the glob matches was created", "a.txt", "b.txt")

	if err := os.Remove("a.txt"); err != nil {
		t.Fatal(err)
	}
	expectRun(t, runs, "a watched file was removed", "b.txt")

	if err := os.WriteFile("other.log", []byte("changed, and longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	expectNoRun(t, runs, "a file nothing matches changed")

	writeFiles(t, ".b.txt.swp")
	expectNoRun(t, runs, "an editor's swap file was written")

	stop()
}

func TestWatcherPostpone(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFiles(t, "a.txt")
	runs, stop := watchTxt(t, WithPostpone())

	expectNoRun(t, runs, "starting")
	writeFiles(t, "b.txt")
	expectRun(t, runs, "the first change", "a.txt", "b.txt")
	stop()
}

func TestWatcherDebounce(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFiles(t, "a.txt")
	runs, stop := watchTxt(t, WithDebounce(300*time.Millisecond))
	expectRun(t, runs, "starting", "a.txt")

	// Write a.txt a piece at a time, more often than the debounce.
	for i := range 10 {
		if err := os.WriteFile("a.txt", []byte(strings.Repeat("x", i+1)), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(runs) > 0 {
		t.Fatalf("ran %d times while a.txt was still being written", len(runs))
	}
	expectRun(t, runs, "a.txt stopped changing", "a.txt")
	expectNoRun(t, runs, "nothing changed since")
	stop()
}

func TestWatcherRestart(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFiles(t, "a.txt")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan string, 10)
	w := NewWatcher(10*time.Millisecond, []Pattern{{Glob: "*.txt"}}, WithRestart())
	done := make(chan error)
	go func() {
		done <- w.Run(ctx, func(ctx context.Context, c Change) {
			events <- fmt.Sprint("start ", c.Files)
			<-ctx.Done() // a command that runs until it is stopped
			events <- fmt.Sprint("stop ", c.Files)
		})
	}()
	expect := func(why, want string) {
		t.Helper()
		select {
		case got := <-events:
			if got != want {
				t.Errorf("after %s, got %q, want %q", why, got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("nothing after %s, want %q", why, want)
		}
	}

	expect("starting", "start [a.txt]")
	writeFiles(t, "b.txt")
	expect("a change while the call was going on", "stop [a.txt]")
	expect("the call before was stopped", "start [a.txt b.txt]")
	cancel()
	expect("the watch was stopped", "stop [a.txt b.txt]")
	if err := <-done; err != nil {
		t.Errorf("Run returned %v, want nil once its context is done", err)
	}
}

// runWatcher starts w, calling back with onChange, and returns a function
// that stops it and checks it ended well.
func runWatcher(t *testing.T, w Watcher, onChange func(context.Context, Change)) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- w.Run(ctx, onChange) }()
	t.Cleanup(cancel)
	return func() {
		t.Helper()
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Run returned %v, want nil once its context is done", err)
		}
	}
}

// expectEvent fails the test unless events has want soon.
func expectEvent(t *testing.T, events chan string, why, want string) {
	t.Helper()
	select {
	case got := <-events:
		if got != want {
			t.Errorf("after %s, got %q, want %q", why, got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("nothing after %s, want %q", why, want)
	}
}

// expectNoEvent fails the test if events has anything within 100ms.
func expectNoEvent(t *testing.T, events chan string, why string) {
	t.Helper()
	select {
	case got := <-events:
		t.Fatalf("got %q after %s, want nothing", got, why)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestWatcherChange(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFiles(t, "a.txt", "b.txt")
	changes := make(chan Change, 10)
	w := NewWatcher(10*time.Millisecond, []Pattern{{Glob: "*.txt"}, {Glob: "later.csv"}}, WithDebounce(100*time.Millisecond))
	stop := runWatcher(t, w, func(_ context.Context, c Change) { changes <- c })
	expect := func(why string, want Change) {
		t.Helper()
		select {
		case got := <-changes:
			if !reflect.DeepEqual(got, want) {
				t.Errorf("after %s, called back with %+v, want %+v", why, got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("no call after %s", why)
		}
	}

	later := Pattern{Glob: "later.csv"}
	expect("starting", Change{Files: []string{"a.txt", "b.txt"}, Unmatched: []Pattern{later}})

	if err := os.WriteFile("a.txt", []byte("changed, and longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove("b.txt"); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, "c.txt")
	expect("a change, a removal and an addition", Change{
		Files:     []string{"a.txt", "c.txt"},
		Added:     []string{"c.txt"},
		Removed:   []string{"b.txt"},
		Modified:  []string{"a.txt"},
		Unmatched: []Pattern{later},
	})

	writeFiles(t, "later.csv")
	expect("the file a pattern named appeared", Change{Files: []string{"a.txt", "c.txt", "later.csv"}, Added: []string{"later.csv"}})
	stop()
}

func TestWatcherWaitsForTheCallGoingOn(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFiles(t, "a.txt")
	events := make(chan string, 10)
	release := make(chan struct{})
	w := NewWatcher(10*time.Millisecond, []Pattern{{Glob: "*.txt"}})
	stop := runWatcher(t, w, func(ctx context.Context, c Change) {
		events <- fmt.Sprint("start ", c.Files)
		select {
		case <-release:
		case <-ctx.Done():
		}
		events <- "end"
	})

	expectEvent(t, events, "starting", "start [a.txt]")
	writeFiles(t, "b.txt")
	expectNoEvent(t, events, "a change while the call was going on")
	release <- struct{}{}
	expectEvent(t, events, "the call was let end", "end")
	expectEvent(t, events, "the call before ended", "start [a.txt b.txt]")
	stop()
	expectEvent(t, events, "the watch was stopped", "end")
}

func TestWatcherKeys(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFiles(t, "a.txt")
	events := make(chan string, 10)
	keys := make(chan Key)
	w := NewWatcher(10*time.Millisecond, []Pattern{{Glob: "*.txt"}}, WithKeys(keys))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error)
	go func() {
		done <- w.Run(ctx, func(ctx context.Context, c Change) {
			events <- "start"
			<-ctx.Done() // a command that runs until it is stopped
			events <- "stop"
		})
	}()

	expectEvent(t, events, "starting", "start")
	keys <- KeyStop
	expectEvent(t, events, "KeyStop", "stop")
	keys <- KeyRun
	expectEvent(t, events, "KeyRun", "start")
	keys <- KeyRun
	expectEvent(t, events, "KeyRun while a call was going on", "stop")
	expectEvent(t, events, "KeyRun while a call was going on", "start")
	keys <- KeyStop
	expectEvent(t, events, "KeyStop", "stop")

	keys <- KeyPause
	writeFiles(t, "b.txt")
	expectNoEvent(t, events, "a change while paused")
	keys <- KeyPause
	expectEvent(t, events, "going on after a change while paused", "start")

	keys <- KeyQuit
	expectEvent(t, events, "KeyQuit", "stop")
	if err := <-done; err != nil {
		t.Errorf("Run returned %v after KeyQuit, want nil", err)
	}
}
