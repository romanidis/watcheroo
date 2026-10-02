package watch_test

import (
	"context"
	"io"
	"time"

	"github.com/romanidis/watcheroo/internal/domain"
	"github.com/romanidis/watcheroo/internal/watch"
)

// FakeScanner is a test double for Scanner: set the fields a test needs,
// and any method left unset panics with its name.
type FakeScanner struct {
	ScanFunc func(list domain.Watchlist) ([]domain.Match, error)
}

// Scan calls ScanFunc, or panics when the test did not set it.
func (f FakeScanner) Scan(list domain.Watchlist) ([]domain.Match, error) {
	if f.ScanFunc == nil {
		panic("FakeScanner.Scan: ScanFunc is not set")
	}
	return f.ScanFunc(list)
}

// FakeProgramFinder is a test double for ProgramFinder: set the fields a test needs,
// and any method left unset panics with its name.
type FakeProgramFinder struct {
	FindProgramFunc func(name string) error
}

// FindProgram calls FindProgramFunc, or panics when the test did not set it.
func (f FakeProgramFinder) FindProgram(name string) error {
	if f.FindProgramFunc == nil {
		panic("FakeProgramFinder.FindProgram: FindProgramFunc is not set")
	}
	return f.FindProgramFunc(name)
}

// FakeExecutor is a test double for Executor: set the fields a test needs,
// and any method left unset panics with its name.
type FakeExecutor struct {
	ExecuteFunc func(ctx context.Context, argv domain.Argv, stdout, stderr io.Writer, timeout time.Duration) domain.Outcome
}

// Execute calls ExecuteFunc, or panics when the test did not set it.
func (f FakeExecutor) Execute(
	ctx context.Context,
	argv domain.Argv,
	stdout, stderr io.Writer,
	timeout time.Duration,
) domain.Outcome {
	if f.ExecuteFunc == nil {
		panic("FakeExecutor.Execute: ExecuteFunc is not set")
	}
	return f.ExecuteFunc(ctx, argv, stdout, stderr, timeout)
}

// FakeDisplay is a test double for Display: set the fields a test needs,
// and any method left unset panics with its name.
type FakeDisplay struct {
	StreamsFunc     func() (stdout, stderr io.Writer)
	ShowStartFunc   func(r watch.RunStart)
	ShowEndFunc     func(r watch.RunEnd)
	ShowPausedFunc  func(paused bool)
	ShowRebasedFunc func()
}

// Streams calls StreamsFunc, or panics when the test did not set it.
func (f FakeDisplay) Streams() (stdout, stderr io.Writer) {
	if f.StreamsFunc == nil {
		panic("FakeDisplay.Streams: StreamsFunc is not set")
	}
	return f.StreamsFunc()
}

// ShowStart calls ShowStartFunc, or panics when the test did not set it.
func (f FakeDisplay) ShowStart(r watch.RunStart) {
	if f.ShowStartFunc == nil {
		panic("FakeDisplay.ShowStart: ShowStartFunc is not set")
	}
	f.ShowStartFunc(r)
}

// ShowEnd calls ShowEndFunc, or panics when the test did not set it.
func (f FakeDisplay) ShowEnd(r watch.RunEnd) {
	if f.ShowEndFunc == nil {
		panic("FakeDisplay.ShowEnd: ShowEndFunc is not set")
	}
	f.ShowEndFunc(r)
}

// ShowPaused calls ShowPausedFunc, or panics when the test did not set it.
func (f FakeDisplay) ShowPaused(paused bool) {
	if f.ShowPausedFunc == nil {
		panic("FakeDisplay.ShowPaused: ShowPausedFunc is not set")
	}
	f.ShowPausedFunc(paused)
}

// ShowRebased calls ShowRebasedFunc, or panics when the test did not set it.
func (f FakeDisplay) ShowRebased() {
	if f.ShowRebasedFunc == nil {
		panic("FakeDisplay.ShowRebased: ShowRebasedFunc is not set")
	}
	f.ShowRebasedFunc()
}

// FakeRequests is a test double for Requests: set the fields a test needs,
// and any method left unset panics with its name.
type FakeRequests struct {
	ListenFunc func() (requests <-chan watch.Request, stop func())
}

// Listen calls ListenFunc, or panics when the test did not set it.
func (f FakeRequests) Listen() (requests <-chan watch.Request, stop func()) {
	if f.ListenFunc == nil {
		panic("FakeRequests.Listen: ListenFunc is not set")
	}
	return f.ListenFunc()
}

// FakeClock is a test double for Clock: set the fields a test needs,
// and any method left unset panics with its name.
type FakeClock struct {
	NowFunc func() time.Time
}

// Now calls NowFunc, or panics when the test did not set it.
func (f FakeClock) Now() time.Time {
	if f.NowFunc == nil {
		panic("FakeClock.Now: NowFunc is not set")
	}
	return f.NowFunc()
}
