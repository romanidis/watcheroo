/*
USECASE Watch Files

Runs the command, then runs it again each time the files of the watchlist
change, or the person watching asks, until ctx is done or they ask to quit.
Each run is shown as it starts and as it ends, and in diff mode compared with
the run its baseline picks. The domain's Watch decides when a run is due;
this use case looks at the files on every tick, keeps one run going at a
time in the background, and stops it when a run due replaces it or the
watch ends. A command that fails is shown, not returned: the watch goes on.

Authz posture: none. wtr runs as whoever started it, on their own files and
their own command; there is no one else to check for.
*/
package watch

import (
	"context"
	"time"

	"github.com/romanidis/watcheroo/internal/domain"
)

// WatchFilesCommand is what to watch, what to run, and how. Context is the
// lines diff mode keeps around each change, or below 0 for every line.
type WatchFilesCommand struct {
	Watchlist   WatchlistInput
	Command     CommandInput
	Mode        string
	Baseline    string
	Context     int
	MergeStderr bool // show stderr in with stdout, so diff mode compares it too
	Interval    time.Duration
	Debounce    time.Duration
	Timeout     time.Duration
	Restart     bool
	Postpone    bool
}

// WatchFilesResult is empty: a watch ends when it is told to, with nothing
// to say but an error.
type WatchFilesResult struct{}

type WatchFilesUsecase struct {
	scanner  Scanner
	finder   ProgramFinder
	executor Executor
	display  Display
	requests Requests
	clock    Clock
}

func NewWatchFilesUsecase(
	scanner Scanner,
	finder ProgramFinder,
	executor Executor,
	display Display,
	requests Requests,
	clock Clock,
) *WatchFilesUsecase {
	return &WatchFilesUsecase{
		scanner:  scanner,
		finder:   finder,
		executor: executor,
		display:  display,
		requests: requests,
		clock:    clock,
	}
}

func (uc *WatchFilesUsecase) Handle(ctx context.Context, cmd WatchFilesCommand) (WatchFilesResult, error) {
	list, err := newWatchlist(cmd.Watchlist)
	if err != nil {
		return WatchFilesResult{}, err
	}
	command, err := domain.NewCommand(cmd.Command.Argv, cmd.Command.Shell)
	if err != nil {
		return WatchFilesResult{}, err
	}
	mode, err := domain.ParseMode(cmd.Mode)
	if err != nil {
		return WatchFilesResult{}, err
	}
	baseline, err := domain.ParseBaseline(cmd.Baseline)
	if err != nil {
		return WatchFilesResult{}, err
	}
	schedule, err := domain.NewSchedule(domain.ScheduleOptions{
		Mode:     mode,
		Interval: cmd.Interval,
		Debounce: cmd.Debounce,
		Timeout:  cmd.Timeout,
		Restart:  cmd.Restart,
		Postpone: cmd.Postpone,
	})
	if err != nil {
		return WatchFilesResult{}, err
	}
	if err := uc.finder.FindProgram(command.Program()); err != nil {
		return WatchFilesResult{}, err
	}

	start, err := look(uc.scanner, list)
	if err != nil {
		return WatchFilesResult{}, err
	}
	watch := domain.NewWatch(schedule, start, uc.clock.Now())
	r := &runner{
		executor:    uc.executor,
		display:     uc.display,
		clock:       uc.clock,
		command:     command,
		mode:        mode,
		timeout:     schedule.Timeout(),
		mergeStderr: cmd.MergeStderr,
	}
	if mode.Compares() {
		r.comparison = domain.NewComparison(baseline, cmd.Context)
	}

	requests, stopListening := uc.requests.Listen()
	defer stopListening()
	var runs background
	defer runs.stop()
	if c, ok := watch.Begin(); ok {
		runs.start(ctx, func(ctx context.Context) { r.run(ctx, c) })
	}
	tick := time.NewTicker(schedule.Interval())
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return WatchFilesResult{}, nil
		case req := <-requests:
			switch req {
			case RequestRun:
				now, err := look(uc.scanner, list)
				if err != nil {
					return WatchFilesResult{}, err
				}
				c := watch.RunNow(now)
				runs.start(ctx, func(ctx context.Context) { r.run(ctx, c) })
			case RequestStop:
				runs.stop()
			case RequestPause:
				uc.display.ShowPaused(watch.TogglePause())
			case RequestRebase:
				if r.rebase() {
					uc.display.ShowRebased()
				}
			case RequestQuit:
				return WatchFilesResult{}, nil
			}
		case <-tick.C:
			now, err := look(uc.scanner, list)
			if err != nil {
				return WatchFilesResult{}, err
			}
			if c, ok := watch.Look(now, uc.clock.Now(), runs.busy()); ok {
				runs.start(ctx, func(ctx context.Context) { r.run(ctx, c) })
			}
		}
	}
}
