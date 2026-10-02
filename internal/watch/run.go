package watch

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/romanidis/watcheroo/internal/domain"
)

// runner makes one run of the command, and shows it: the part of Watch
// Files that happens off its loop.
type runner struct {
	executor    Executor
	display     Display
	clock       Clock
	command     domain.Command
	mode        domain.Mode
	timeout     time.Duration
	mergeStderr bool

	mu         sync.Mutex         // guards comparison, which a rebase asked for changes while a run goes on
	comparison *domain.Comparison // nil unless the mode compares
}

// run runs the command for c, and shows it start and end. In a mode that
// compares, the output is kept and compared rather than shown as it comes.
func (r *runner) run(ctx context.Context, c domain.Change) {
	start := RunStart{At: r.clock.Now(), Change: c, Argv: r.command.For(c.Files), Mode: r.mode}
	r.display.ShowStart(start)
	stdout, stderr := r.display.Streams()
	var out, errOut bytes.Buffer
	if r.comparison != nil {
		stdout, stderr = &out, &errOut
	}
	if r.mergeStderr {
		stderr = stdout
	}
	end := RunEnd{RunStart: start}
	end.Outcome = r.executor.Execute(ctx, start.Argv, stdout, stderr, r.timeout)
	if r.comparison != nil {
		end.Stderr = errOut.Bytes()
		r.mu.Lock()
		if d, ok := r.comparison.Compare(out.String(), end.Outcome); ok {
			end.Diff = &d
		}
		r.mu.Unlock()
	}
	r.display.ShowEnd(end)
}

// rebase makes the next run compared with the last one shown, and reports
// whether there was one, in a mode that compares.
func (r *runner) rebase() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.comparison != nil && r.comparison.Rebase()
}

// background keeps one run going at a time, off the watch's loop, so the
// loop goes on looking and listening while it runs.
type background struct {
	cancel context.CancelFunc // stops the run going on
	done   chan struct{}      // closed once it has returned
}

// start stops the run going on, if any, and waits for it to return, then
// starts run with a context of its own, which stop cancels.
func (b *background) start(ctx context.Context, run func(context.Context)) {
	b.stop()
	ctx, b.cancel = context.WithCancel(ctx)
	done := make(chan struct{})
	b.done = done
	go func() {
		defer close(done)
		run(ctx)
	}()
}

// stop stops the run going on, and waits for it to return.
func (b *background) stop() {
	if b.cancel == nil {
		return
	}
	b.cancel()
	<-b.done
}

// busy reports whether a run is going on.
func (b *background) busy() bool {
	if b.done == nil {
		return false
	}
	select {
	case <-b.done:
		return false
	default:
		return true
	}
}
