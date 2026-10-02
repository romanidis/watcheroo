// Package process runs the command: once a run, in a process group of its
// own so that stopping it stops everything it started, within a timeout
// when there is one. It is the adapter for watch.Executor and
// watch.ProgramFinder.
package process

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"time"

	"github.com/romanidis/watcheroo/internal/domain"
	"github.com/romanidis/watcheroo/internal/watch"
)

// Compile-time proof the runner covers both ports.
var (
	_ watch.Executor      = Runner{}
	_ watch.ProgramFinder = Runner{}
)

// Runner runs commands as processes of this machine, found on PATH.
type Runner struct{}

// FindProgram returns exec's error for a program that is not on PATH.
func (Runner) FindProgram(name string) error {
	_, err := exec.LookPath(name)
	return err
}

// Execute runs argv to its end, or until ctx is done or timeout passes. A
// run ctx ended is stopped whatever its exit status, since a command that
// is sent SIGTERM fails as a rule. It is the run's context, not the timeout
// of its own, that says so: a run that ran out of time was not stopped by
// the watch, and is reported as a failure.
func (Runner) Execute(ctx context.Context, argv domain.Argv, stdout, stderr io.Writer, timeout time.Duration) domain.Outcome {
	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	stopGroup(cmd)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	start := time.Now()
	err := cmd.Run()
	took := time.Since(start).Round(time.Millisecond)

	var exit *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return domain.OutcomeStopped(took)
	case err != nil && runCtx.Err() != nil:
		return domain.OutcomeTimedOut(timeout, took)
	case errors.As(err, &exit) && exit.ExitCode() >= 0:
		return domain.OutcomeExit(exit.ExitCode(), took)
	case err != nil:
		return domain.OutcomeError(err.Error(), took)
	}
	return domain.OutcomeOK(took)
}
