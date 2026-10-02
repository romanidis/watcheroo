package process_test

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/romanidis/watcheroo/internal/domain"
	"github.com/romanidis/watcheroo/internal/process"
)

func TestExecute(t *testing.T) {
	tests := []struct {
		name       string
		argv       domain.Argv
		timeout    time.Duration
		want       domain.Ending
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "a command that succeeds", argv: domain.Argv{"echo", "hi"}, want: domain.EndingOK, wantStdout: "hi\n"},
		{
			name:       "a command that fails",
			argv:       domain.Argv{"sh", "-c", "echo boom >&2; exit 3"},
			want:       domain.EndingExit,
			wantCode:   3,
			wantStderr: "boom\n",
		},
		{name: "a command killed by a signal", argv: domain.Argv{"sh", "-c", "kill -9 $$"}, want: domain.EndingError},
		{
			name:       "a command that runs longer than its timeout",
			argv:       domain.Argv{"sh", "-c", "echo partial; sleep 30; true"},
			timeout:    100 * time.Millisecond,
			want:       domain.EndingTimedOut,
			wantStdout: "partial\n",
		},
		{name: "a command within its timeout", argv: domain.Argv{"echo", "quick"}, timeout: time.Minute, want: domain.EndingOK, wantStdout: "quick\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			start := time.Now()
			o := process.Runner{}.Execute(context.Background(), tt.argv, &stdout, &stderr, tt.timeout)
			if took := time.Since(start); took > 2*time.Second {
				t.Errorf("took %v, want a timeout to stop it soon after", took)
			}
			if o.Ending() != tt.want || o.Code() != tt.wantCode {
				t.Errorf("ended %s with %d, want %s with %d", o.Ending(), o.Code(), tt.want, tt.wantCode)
			}
			if stdout.String() != tt.wantStdout || stderr.String() != tt.wantStderr {
				t.Errorf("wrote %q and %q, want %q and %q", stdout.String(), stderr.String(), tt.wantStdout, tt.wantStderr)
			}
		})
	}
}

func TestExecuteStopsWhatTheCommandStarted(t *testing.T) {
	t.Chdir(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan domain.Outcome)
	go func() {
		// sleep holds the stdout pipe open, so the run ends only once sleep
		// does, and the trailing true keeps sh from becoming sleep.
		var stdout bytes.Buffer
		done <- process.Runner{}.Execute(ctx, domain.Argv{"sh", "-c", "touch started; sleep 30; true"}, &stdout, &stdout, 0)
	}()
	for _, err := os.Stat("started"); err != nil; _, err = os.Stat("started") {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case o := <-done:
		if o.Ending() != domain.EndingStopped {
			t.Errorf("a run whose context ended %s, want it stopped", o.Ending())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the run went on after its context was done; sleep was not stopped with sh")
	}
}

func TestFindProgram(t *testing.T) {
	if err := (process.Runner{}).FindProgram("sh"); err != nil {
		t.Errorf("sh was not found: %v", err)
	}
	if err := (process.Runner{}).FindProgram("wtr-no-such-command"); err == nil {
		t.Error("found a program that is not there")
	}
}
