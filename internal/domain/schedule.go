package domain

import (
	"errors"
	"time"
)

var (
	ErrInterval = errors.New("interval must be more than zero")
	ErrDebounce = errors.New("debounce cannot be less than zero")
	ErrTimeout  = errors.New("timeout cannot be less than zero")
	// ErrRestartInDiff is restart asked for in diff mode. A command that
	// needs restart, like a server, does not end by itself, and diff mode
	// shows the output only once the command ends.
	ErrRestartInDiff = errors.New("restart does not work with diff mode, which shows the output only once the command ends")
	// ErrTimeoutWithRestart is a timeout asked for with restart, whose
	// command runs until a change stops it.
	ErrTimeoutWithRestart = errors.New("timeout does not work with restart, whose command runs until a change stops it")
)

// Schedule says when a watch runs its command: how often it looks at the
// files, how long they must stay as they are before a run, whether the first
// run waits for a change, how long a run may take, and whether a change
// stops a run still going.
type Schedule struct {
	interval  time.Duration
	debounce  time.Duration
	timeout   time.Duration
	stopStale bool
	postpone  bool
}

// ScheduleOptions are what a Schedule is made from, as asked for. A zero
// Debounce runs on the first look that sees a change, and a zero Timeout
// lets a run take as long as it likes.
type ScheduleOptions struct {
	Mode     Mode
	Interval time.Duration
	Debounce time.Duration
	Timeout  time.Duration
	Restart  bool // stop a run a change catches still going, and run again
	Postpone bool // wait for the first change before the first run
}

// NewSchedule returns the Schedule o asks for.
//
// Diff mode stops a run a change catches still going, as restart does: what
// it would show is out of date already, and an awk script stuck in a loop
// would otherwise hold the watch until Ctrl-C. Clear and append mode let the
// run end first, as it may be doing something wanted done, unless restart
// says otherwise.
func NewSchedule(o ScheduleOptions) (Schedule, error) {
	switch {
	case o.Interval <= 0:
		return Schedule{}, ErrInterval
	case o.Debounce < 0:
		return Schedule{}, ErrDebounce
	case o.Timeout < 0:
		return Schedule{}, ErrTimeout
	case o.Restart && o.Mode == ModeDiff:
		return Schedule{}, ErrRestartInDiff
	case o.Restart && o.Timeout > 0:
		return Schedule{}, ErrTimeoutWithRestart
	}
	return Schedule{
		interval:  o.Interval,
		debounce:  o.Debounce,
		timeout:   o.Timeout,
		stopStale: o.Restart || o.Mode == ModeDiff,
		postpone:  o.Postpone,
	}, nil
}

// Interval returns how often the watch looks at the files.
func (s Schedule) Interval() time.Duration {
	return s.interval
}

// Timeout returns how long a run may take, or 0 for as long as it likes.
func (s Schedule) Timeout() time.Duration {
	return s.timeout
}
