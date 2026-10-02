// Package watch is the watching feature: run a command, then run it again
// each time the files it reads change, showing each run; or list what a
// watch would look at and run, without running anything. The domain decides
// when a run is due, what it runs, and how its output differs from the run
// before; this package looks, runs and shows, through the ports below.
//
// It is deliberately not a build tool or a process manager: one command,
// files looked at by polling, no config file (decided 2026-10-02).
package watch

import (
	"context"
	"io"
	"time"

	"github.com/romanidis/watcheroo/internal/domain"
)

// Scanner looks at the files list names now, and returns every match it
// finds: a file once for each pattern that matches it, less what the list
// leaves out. A file that disappears while it looks is left out, as if it
// had not matched. It has no sentinels: any error ends the watch.
type Scanner interface {
	Scan(list domain.Watchlist) ([]domain.Match, error)
}

// ProgramFinder checks that the program a command names can be run, so a
// typo in it fails at the start rather than on every run. Its error says
// why not; there are no sentinels.
type ProgramFinder interface {
	FindProgram(name string) error
}

// Executor runs argv once, its output to stdout and stderr, and reports how
// it ended. It stops the command, and everything the command started, when
// ctx is done, which it reports as domain.EndingStopped, or once it has run
// for timeout, when that is above zero. A failing command is an Outcome, not
// an error: the next change runs it again.
type Executor interface {
	Execute(ctx context.Context, argv domain.Argv, stdout, stderr io.Writer, timeout time.Duration) domain.Outcome
}

// Display shows a watch to the person watching it: each run as it starts
// and as it ends, and what came of what they asked for. Runs are shown one
// at a time, but ShowPaused and ShowRebased can come while one goes on.
type Display interface {
	// Streams returns where a run's output goes when it is shown as it
	// comes, which it is unless the mode compares it.
	Streams() (stdout, stderr io.Writer)
	ShowStart(r RunStart)
	ShowEnd(r RunEnd)
	ShowPaused(paused bool)
	ShowRebased()
}

// Requests hands over what the person watching asks for, as they ask.
// Listen starts listening, and the func it returns stops it and gives back
// whatever it took over, such as the terminal; the watch calls it before it
// returns. Where no one can ask, the channel is nil.
type Requests interface {
	Listen() (requests <-chan Request, stop func())
}

// Clock tells the time a run starts, and the time of each look, which the
// debounce measures from. Injected so a test can say what time it is.
type Clock interface {
	Now() time.Time
}

// Request is what the person watching can ask a watch for. The set is
// closed; how each is asked for is the Requests adapter's business.
type Request int

const (
	RequestRun    Request = iota // run the command now, as for a change
	RequestStop                  // stop the run going on
	RequestPause                 // stop running for changes, or go on again
	RequestRebase                // compare the next run with the last one shown
	RequestQuit                  // end the watch
)

// RunStart is a run as it starts: when, what it was made for, the command
// as it runs, and the mode it is shown in.
type RunStart struct {
	At     time.Time
	Change domain.Change
	Argv   domain.Argv
	Mode   domain.Mode
}

// RunEnd is a run as it ended. In a mode that compares, Stderr is what the
// command wrote there, kept to show with the output, and Diff is how the
// output differs from the run compared with, unless the run did not finish.
type RunEnd struct {
	RunStart
	Outcome domain.Outcome
	Stderr  []byte
	Diff    *domain.Diff
}
