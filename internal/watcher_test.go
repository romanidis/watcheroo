package internal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
		"a.txt", "b.txt", "c.csv", ".env", ".a.txt.swp",
		"data/jan.csv", "data/feb.csv", "data/notes.md", "data/.jan.csv.swp", "data/old/dec.csv",
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
			name:  "a hidden file named with its dot",
			globs: []string{".env", ".*.swp"},
			want:  []string{".a.txt.swp", ".env"},
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
			name:    "an exclude regex",
			regexes: []string{`\.csv$`},
			opts:    []WatcherOption{WithExcludes(nil, []*regexp.Regexp{regexp.MustCompile(`^data/old/|^c`)})},
			want:    []string{"data/feb.csv", "data/jan.csv"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var res []*regexp.Regexp
			for _, expr := range tt.regexes {
				res = append(res, regexp.MustCompile(expr))
			}
			w := NewWatcher(time.Millisecond, tt.globs, res, tt.opts...)
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
	w := NewWatcher(10*time.Millisecond, []string{"*.txt"}, nil, opts...)
	done := make(chan error)
	go func() { done <- w.Run(ctx, func(_ context.Context, files []string) { runs <- files }) }()
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
	w := NewWatcher(10*time.Millisecond, []string{"*.txt"}, nil, WithRestart())
	done := make(chan error)
	go func() {
		done <- w.Run(ctx, func(ctx context.Context, files []string) {
			events <- fmt.Sprint("start ", files)
			<-ctx.Done() // a command that runs until it is stopped
			events <- fmt.Sprint("stop ", files)
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
