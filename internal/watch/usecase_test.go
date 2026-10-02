package watch_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/romanidis/watcheroo/internal/domain"
	"github.com/romanidis/watcheroo/internal/watch"
)

// files is a disk for a test to change while a watch looks at it: each
// file's path, and the second it was last modified.
type files struct {
	mu       sync.Mutex
	modified map[string]int
}

func newFiles(paths ...string) *files {
	f := &files{modified: map[string]int{}}
	for _, path := range paths {
		f.modified[path] = 1
	}
	return f
}

// write creates path, or modifies it.
func (f *files) write(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.modified[path]++
}

func (f *files) remove(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.modified, path)
}

// scanner matches the globs of a watchlist against the files.
func (f *files) scanner() FakeScanner {
	return FakeScanner{ScanFunc: func(list domain.Watchlist) ([]domain.Match, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var matches []domain.Match
		for i, p := range list.Patterns() {
			for _, path := range slices.Sorted(maps.Keys(f.modified)) {
				if ok, _ := filepath.Match(p.Glob(), path); ok {
					matches = append(matches, domain.Match{Path: path, Pattern: i, Stamp: domain.NewStamp(time.Unix(int64(f.modified[path]), 0), 1)})
				}
			}
		}
		return matches, nil
	}}
}

// found finds every program.
var found = FakeProgramFinder{FindProgramFunc: func(string) error { return nil }}

var wallClock = FakeClock{NowFunc: time.Now}

// display records the runs it is shown, and the notes.
type display struct {
	starts chan watch.RunStart
	ends   chan watch.RunEnd
	notes  chan string
}

func newDisplay() *display {
	return &display{starts: make(chan watch.RunStart, 100), ends: make(chan watch.RunEnd, 100), notes: make(chan string, 100)}
}

func (d *display) fake() FakeDisplay {
	return FakeDisplay{
		StreamsFunc:     func() (io.Writer, io.Writer) { return io.Discard, io.Discard },
		ShowStartFunc:   func(r watch.RunStart) { d.starts <- r },
		ShowEndFunc:     func(r watch.RunEnd) { d.ends <- r },
		ShowPausedFunc:  func(paused bool) { d.notes <- fmt.Sprint("paused ", paused) },
		ShowRebasedFunc: func() { d.notes <- "rebased" },
	}
}

// noRequests is a terminal no one can ask anything from.
var noRequests = FakeRequests{ListenFunc: func() (<-chan watch.Request, func()) { return nil, func() {} }}

// quick runs and ends at once.
var quick = FakeExecutor{ExecuteFunc: func(context.Context, domain.Argv, io.Writer, io.Writer, time.Duration) domain.Outcome {
	return domain.OutcomeOK(0)
}}

// untilStopped runs until it is stopped.
var untilStopped = FakeExecutor{ExecuteFunc: func(ctx context.Context, _ domain.Argv, _, _ io.Writer, _ time.Duration) domain.Outcome {
	<-ctx.Done()
	return domain.OutcomeStopped(0)
}}

// watchTxt returns the command to watch *.txt and cat the files, looking
// every 10ms.
func watchTxt() watch.WatchFilesCommand {
	return watch.WatchFilesCommand{
		Watchlist: watch.WatchlistInput{Patterns: []watch.PatternInput{{Expr: "*.txt"}}},
		Command:   watch.CommandInput{Argv: []string{"cat", "{}"}},
		Mode:      "clear",
		Baseline:  "previous",
		Context:   -1,
		Interval:  10 * time.Millisecond,
	}
}

// start starts uc on cmd, and returns a function that stops it and checks it
// ended well.
func start(t *testing.T, uc *watch.WatchFilesUsecase, cmd watch.WatchFilesCommand) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		_, err := uc.Handle(ctx, cmd)
		done <- err
	}()
	t.Cleanup(cancel)
	return func() {
		t.Helper()
		cancel()
		if err := <-done; err != nil {
			t.Errorf("the watch returned %v, want nil once its context is done", err)
		}
	}
}

// expect fails the test unless ch has a value soon, and returns it.
func expect[T any](t *testing.T, ch chan T, why string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatalf("nothing after %s", why)
		panic("unreachable")
	}
}

// expectNone fails the test if ch has a value within 100ms.
func expectNone[T any](t *testing.T, ch chan T, why string) {
	t.Helper()
	select {
	case v := <-ch:
		t.Fatalf("got %v after %s, want nothing", v, why)
	case <-time.After(100 * time.Millisecond):
	}
}

// expectRun fails the test unless a run starts soon for files.
func expectRun(t *testing.T, d *display, why string, files ...string) watch.RunStart {
	t.Helper()
	r := expect(t, d.starts, why)
	if !slices.Equal(r.Change.Files, files) {
		t.Errorf("after %s, ran for %v, want %v", why, r.Change.Files, files)
	}
	return r
}

func TestWatchFilesRunsOnChange(t *testing.T) {
	disk := newFiles("a.txt", "other.log")
	d := newDisplay()
	uc := watch.NewWatchFilesUsecase(disk.scanner(), found, quick, d.fake(), noRequests, wallClock)
	stop := start(t, uc, watchTxt())

	r := expectRun(t, d, "starting", "a.txt")
	if got := r.Argv.String(); got != "cat a.txt" {
		t.Errorf("ran %s, want cat a.txt", got)
	}
	expectNone(t, d.starts, "nothing changed")

	disk.write("a.txt")
	r = expectRun(t, d, "a watched file changed", "a.txt")
	if !slices.Equal(r.Change.Modified, []string{"a.txt"}) {
		t.Errorf("the change is %+v, want a.txt modified", r.Change)
	}
	disk.write("b.txt")
	expectRun(t, d, "a file the glob matches was created", "a.txt", "b.txt")
	disk.remove("a.txt")
	expectRun(t, d, "a watched file was removed", "b.txt")
	disk.write("other.log")
	expectNone(t, d.starts, "a file nothing matches changed")
	stop()
}

func TestWatchFilesPostpone(t *testing.T) {
	disk := newFiles("a.txt")
	d := newDisplay()
	cmd := watchTxt()
	cmd.Postpone = true
	stop := start(t, watch.NewWatchFilesUsecase(disk.scanner(), found, quick, d.fake(), noRequests, wallClock), cmd)
	expectNone(t, d.starts, "starting")
	disk.write("b.txt")
	expectRun(t, d, "the first change", "a.txt", "b.txt")
	stop()
}

func TestWatchFilesRestart(t *testing.T) {
	for _, tt := range []struct {
		name string
		cmd  func(*watch.WatchFilesCommand)
	}{
		{name: "with restart", cmd: func(c *watch.WatchFilesCommand) { c.Restart = true }},
		{name: "in diff mode", cmd: func(c *watch.WatchFilesCommand) { c.Mode = "diff" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			disk := newFiles("a.txt")
			d := newDisplay()
			cmd := watchTxt()
			tt.cmd(&cmd)
			stop := start(t, watch.NewWatchFilesUsecase(disk.scanner(), found, untilStopped, d.fake(), noRequests, wallClock), cmd)
			expectRun(t, d, "starting", "a.txt")
			disk.write("b.txt")
			if end := expect(t, d.ends, "a change while the run was going on"); end.Outcome.Ending() != domain.EndingStopped {
				t.Errorf("the run going on ended %s, want it stopped", end.Outcome.Ending())
			}
			expectRun(t, d, "the run before was stopped", "a.txt", "b.txt")
			stop()
			expect(t, d.ends, "the watch was stopped")
		})
	}
}

func TestWatchFilesWaitsForTheRunGoingOn(t *testing.T) {
	disk := newFiles("a.txt")
	d := newDisplay()
	release := make(chan struct{})
	executor := FakeExecutor{ExecuteFunc: func(ctx context.Context, _ domain.Argv, _, _ io.Writer, _ time.Duration) domain.Outcome {
		select {
		case <-release:
			return domain.OutcomeOK(0)
		case <-ctx.Done():
			return domain.OutcomeStopped(0)
		}
	}}
	stop := start(t, watch.NewWatchFilesUsecase(disk.scanner(), found, executor, d.fake(), noRequests, wallClock), watchTxt())
	expectRun(t, d, "starting", "a.txt")
	disk.write("b.txt")
	expectNone(t, d.starts, "a change while the run was going on")
	release <- struct{}{}
	expect(t, d.ends, "the run was let end")
	expectRun(t, d, "the run before ended", "a.txt", "b.txt")
	stop()
}

func TestWatchFilesRequests(t *testing.T) {
	disk := newFiles("a.txt")
	d := newDisplay()
	requests := make(chan watch.Request)
	listening := true
	keys := FakeRequests{ListenFunc: func() (<-chan watch.Request, func()) {
		return requests, func() { listening = false }
	}}
	cmd := watchTxt()
	cmd.Mode = "diff"
	uc := watch.NewWatchFilesUsecase(disk.scanner(), found, untilStopped, d.fake(), keys, wallClock)
	done := make(chan error)
	go func() {
		_, err := uc.Handle(context.Background(), cmd)
		done <- err
	}()
	stopped := func(why string) {
		t.Helper()
		if end := expect(t, d.ends, why); end.Outcome.Ending() != domain.EndingStopped {
			t.Errorf("after %s, the run ended %s, want it stopped", why, end.Outcome.Ending())
		}
	}

	expectRun(t, d, "starting", "a.txt")
	requests <- watch.RequestStop
	stopped("a stop")
	requests <- watch.RequestRun
	expectRun(t, d, "a run asked for", "a.txt")
	requests <- watch.RequestRun
	stopped("a run asked for while a run was going on")
	expectRun(t, d, "a run asked for while a run was going on", "a.txt")
	requests <- watch.RequestStop
	stopped("a stop")

	requests <- watch.RequestPause
	if note := expect(t, d.notes, "a pause"); note != "paused true" {
		t.Errorf("after a pause, showed %q", note)
	}
	disk.write("b.txt")
	expectNone(t, d.starts, "a change while paused")
	requests <- watch.RequestPause
	if note := expect(t, d.notes, "going on"); note != "paused false" {
		t.Errorf("after going on, showed %q", note)
	}
	expectRun(t, d, "going on after a change while paused", "a.txt", "b.txt")

	requests <- watch.RequestRebase
	expectNone(t, d.notes, "a rebase before any run finished")

	requests <- watch.RequestQuit
	stopped("a quit")
	if err := <-done; err != nil {
		t.Errorf("the watch returned %v after a quit, want nil", err)
	}
	if listening {
		t.Error("the watch returned without stopping listening")
	}
}

func TestWatchFilesDiff(t *testing.T) {
	disk := newFiles("a.txt")
	d := newDisplay()
	outputs := make(chan string, 10)
	executor := FakeExecutor{ExecuteFunc: func(_ context.Context, _ domain.Argv, stdout, stderr io.Writer, _ time.Duration) domain.Outcome {
		io.WriteString(stdout, <-outputs)
		io.WriteString(stderr, "warning\n")
		return domain.OutcomeOK(0)
	}}
	cmd := watchTxt()
	cmd.Mode = "diff"
	requests := make(chan watch.Request)
	keys := FakeRequests{ListenFunc: func() (<-chan watch.Request, func()) { return requests, func() {} }}
	stop := start(t, watch.NewWatchFilesUsecase(disk.scanner(), found, executor, d.fake(), keys, wallClock), cmd)

	outputs <- "a 1\n"
	end := expect(t, d.ends, "starting")
	if end.Diff == nil || !end.Diff.First || string(end.Stderr) != "warning\n" {
		t.Fatalf("the first run ended with %+v, want it compared with nothing, and its stderr kept", end)
	}
	requests <- watch.RequestRebase
	if note := expect(t, d.notes, "a rebase"); note != "rebased" {
		t.Errorf("after a rebase, showed %q", note)
	}
	outputs <- "a 2\n"
	disk.write("a.txt")
	end = expect(t, d.ends, "a change")
	if end.Diff == nil || end.Diff.Unchanged || len(end.Diff.Lines) != 2 {
		t.Errorf("the second run ended with %+v, want a line gone and a line come", end.Diff)
	}
	stop()
}

func TestWatchFilesMergeStderr(t *testing.T) {
	disk := newFiles("a.txt")
	d := newDisplay()
	executor := FakeExecutor{ExecuteFunc: func(_ context.Context, _ domain.Argv, stdout, stderr io.Writer, _ time.Duration) domain.Outcome {
		io.WriteString(stdout, "out\n")
		io.WriteString(stderr, "err\n")
		return domain.OutcomeOK(0)
	}}
	cmd := watchTxt()
	cmd.Mode = "diff"
	cmd.MergeStderr = true
	stop := start(t, watch.NewWatchFilesUsecase(disk.scanner(), found, executor, d.fake(), noRequests, wallClock), cmd)
	end := expect(t, d.ends, "starting")
	var lines []string
	for _, line := range end.Diff.Lines {
		lines = append(lines, line.Text())
	}
	if len(end.Stderr) > 0 || !slices.Equal(lines, []string{"out", "err"}) {
		t.Errorf("kept %q apart and compared %q, want stderr in with stdout", end.Stderr, lines)
	}
	stop()
}

func TestWatchFilesTimeout(t *testing.T) {
	disk := newFiles("a.txt")
	d := newDisplay()
	timeouts := make(chan time.Duration, 1)
	executor := FakeExecutor{ExecuteFunc: func(_ context.Context, _ domain.Argv, _, _ io.Writer, timeout time.Duration) domain.Outcome {
		timeouts <- timeout
		return domain.OutcomeOK(0)
	}}
	cmd := watchTxt()
	cmd.Timeout = 10 * time.Second
	stop := start(t, watch.NewWatchFilesUsecase(disk.scanner(), found, executor, d.fake(), noRequests, wallClock), cmd)
	if got := expect(t, timeouts, "starting"); got != 10*time.Second {
		t.Errorf("ran with a timeout of %v, want 10s", got)
	}
	stop()
}

func TestWatchFilesErrors(t *testing.T) {
	notFound := errors.New("not found")
	tests := []struct {
		name  string
		cmd   func(*watch.WatchFilesCommand)
		find  error
		want  error
		wantS string
	}{
		{name: "nothing to watch", cmd: func(c *watch.WatchFilesCommand) { c.Watchlist.Patterns = nil }, want: domain.ErrNothingToWatch},
		{name: "a glob that does not parse", cmd: func(c *watch.WatchFilesCommand) { c.Watchlist.Patterns = []watch.PatternInput{{Expr: "["}} }, want: domain.ErrBadGlob},
		{name: "a regex that does not parse", cmd: func(c *watch.WatchFilesCommand) {
			c.Watchlist.Patterns = []watch.PatternInput{{Expr: "(", Regex: true}}
		}, wantS: "regex: error parsing regexp"},
		{name: "an exclude that does not parse", cmd: func(c *watch.WatchFilesCommand) { c.Watchlist.Exclude = []string{"["} }, want: domain.ErrBadGlob},
		{name: "a shell command in two arguments", cmd: func(c *watch.WatchFilesCommand) { c.Command.Shell = true }, want: domain.ErrShellScript},
		{name: "an unknown mode", cmd: func(c *watch.WatchFilesCommand) { c.Mode = "fancy" }, want: domain.ErrUnknownMode},
		{name: "an unknown baseline", cmd: func(c *watch.WatchFilesCommand) { c.Baseline = "last" }, want: domain.ErrUnknownBaseline},
		{name: "a zero interval", cmd: func(c *watch.WatchFilesCommand) { c.Interval = 0 }, want: domain.ErrInterval},
		{name: "restart in diff mode", cmd: func(c *watch.WatchFilesCommand) { c.Mode, c.Restart = "diff", true }, want: domain.ErrRestartInDiff},
		{name: "a program that is not there", cmd: func(*watch.WatchFilesCommand) {}, find: notFound, want: notFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finder := FakeProgramFinder{FindProgramFunc: func(string) error { return tt.find }}
			// The executor and the display are left unset, so a run panics.
			uc := watch.NewWatchFilesUsecase(newFiles().scanner(), finder, FakeExecutor{}, FakeDisplay{}, noRequests, wallClock)
			cmd := watchTxt()
			tt.cmd(&cmd)
			_, err := uc.Handle(context.Background(), cmd)
			if tt.want != nil && !errors.Is(err, tt.want) || tt.wantS != "" && (err == nil || !strings.Contains(err.Error(), tt.wantS)) {
				t.Errorf("the watch returned %v, want %v%s", err, tt.want, tt.wantS)
			}
		})
	}
}

func TestWatchFilesEndsWhenALookFails(t *testing.T) {
	broken := errors.New("disk on fire")
	looks := 0
	scanner := FakeScanner{ScanFunc: func(domain.Watchlist) ([]domain.Match, error) {
		if looks++; looks > 1 {
			return nil, broken
		}
		return nil, nil
	}}
	d := newDisplay()
	uc := watch.NewWatchFilesUsecase(scanner, found, quick, d.fake(), noRequests, wallClock)
	if _, err := uc.Handle(context.Background(), watchTxt()); !errors.Is(err, broken) {
		t.Errorf("the watch returned %v, want the look's error", err)
	}
}

func TestListWatched(t *testing.T) {
	disk := newFiles("report.awk", "b.csv", "a.csv")
	uc := watch.NewListWatchedUsecase(disk.scanner(), found)
	res, err := uc.Handle(context.Background(), watch.ListWatchedQuery{
		Watchlist: watch.WatchlistInput{Patterns: []watch.PatternInput{{Expr: "report.awk"}, {Expr: "*.csv"}, {Expr: "reprot.awk"}}},
		Command:   watch.CommandInput{Argv: []string{"awk", "-f", "report.awk", "{}"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"report.awk", "a.csv", "b.csv"}; !slices.Equal(res.Files, want) {
		t.Errorf("listed %v, want %v", res.Files, want)
	}
	if len(res.Unmatched) != 1 || res.Unmatched[0].String() != "reprot.awk" {
		t.Errorf("unmatched %v, want reprot.awk", res.Unmatched)
	}
	if got := res.Command.String(); got != "awk -f report.awk a.csv b.csv" {
		t.Errorf("the command is %s, want the files in, less the script", got)
	}
}
