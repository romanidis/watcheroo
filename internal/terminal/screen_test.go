package terminal_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/romanidis/watcheroo/internal/domain"
	"github.com/romanidis/watcheroo/internal/terminal"
	"github.com/romanidis/watcheroo/internal/watch"
)

// The escape codes a Screen writes, spelled out here so a test fails if they
// change.
const (
	clearScreen = "\033[H\033[2J\033[3J"
	topLine     = "\033[H\033[2K"
	bell        = "\a"
	dim         = "\033[2m"
	red         = "\033[31m"
	green       = "\033[32m"
	reverse     = "\033[7m"
	reverseOff  = "\033[27m"
	reset       = "\033[0m"
)

var (
	onTerminal = terminal.Options{Color: true, Clear: true}
	succeeded  = domain.OutcomeOK(4 * time.Millisecond)
)

// show shows a run of cat out.txt in mode for each of outputs, the way the
// watch use case does, each ending with o, and returns all s wrote with the
// colours taken out. In a mode that compares, the output is compared with
// comparison.
func show(t *testing.T, o terminal.Options, mode domain.Mode, comparison *domain.Comparison, outputs ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	s := terminal.NewScreen(&stdout, &stderr, o)
	for _, output := range outputs {
		start := watch.RunStart{At: time.Now(), Argv: domain.Argv{"cat", "out.txt"}, Mode: mode}
		s.ShowStart(start)
		end := watch.RunEnd{RunStart: start, Outcome: succeeded}
		if mode.Compares() {
			if d, ok := comparison.Compare(output, succeeded); ok {
				end.Diff = &d
			}
		} else {
			out, _ := s.Streams()
			out.Write([]byte(output))
		}
		s.ShowEnd(end)
	}
	if stderr.Len() > 0 {
		t.Fatalf("stderr: %s", stderr.String())
	}
	return uncolored(stdout.String())
}

// uncolored is out with the colours taken out.
func uncolored(out string) string {
	return strings.NewReplacer(dim, "", red, "", green, "", reverse, "", reverseOff, "", reset, "").Replace(out)
}

// afterLastClear is what is left on screen after the last clear.
func afterLastClear(out string) string {
	return out[strings.LastIndex(out, clearScreen)+len(clearScreen):]
}

// diffScreens returns the screens diff mode drew whole, one for each run
// that ended, each without the top line a later run rewrote to say it was
// going.
func diffScreens(out string) []string {
	var screens []string
	for _, screen := range strings.Split(out, clearScreen) {
		screen, _, _ = strings.Cut(screen, topLine)
		if strings.Contains(screen, "\n") {
			screens = append(screens, screen)
		}
	}
	return screens
}

// runBody returns what a run printed between its header and its footer.
func runBody(screen string) string {
	_, rest, _ := strings.Cut(screen, "\n")
	return rest[:strings.LastIndex(strings.TrimSuffix(rest, "\n"), "\n")+1]
}

func TestScreenClear(t *testing.T) {
	out := show(t, onTerminal, domain.ModeClear, nil, "one\n", "two\n")
	if strings.Count(out, clearScreen) != 2 {
		t.Errorf("cleared %d times, want once a run:\n%q", strings.Count(out, clearScreen), out)
	}
	screen := afterLastClear(out)
	if strings.Contains(screen, "one") || !strings.Contains(screen, "two\n") {
		t.Errorf("screen shows %q, want only the second run's output", screen)
	}
}

func TestScreenAppend(t *testing.T) {
	out := show(t, onTerminal, domain.ModeAppend, nil, "one\n", "two\n")
	if strings.Contains(out, clearScreen) {
		t.Errorf("cleared the screen in append mode:\n%q", out)
	}
	if one, two := strings.Index(out, "one\n"), strings.Index(out, "two\n"); one < 0 || two < one {
		t.Errorf("printed %q, want the first run's output and then the second's", out)
	}
	if n := strings.Count(out, "cat out.txt"); n != 2 {
		t.Errorf("printed %d headers, want one a run:\n%q", n, out)
	}
}

func TestScreenDiff(t *testing.T) {
	tests := []struct {
		name     string
		baseline domain.Baseline
		context  int
		outputs  []string
		want     []string
	}{
		{
			name:     "a changed line",
			baseline: domain.BaselinePrevious,
			context:  -1,
			outputs:  []string{"a 1\nb 2\nc 3\n", "a 1\nb 5\nc 3\n"},
			want:     []string{"  a 1\n  b 2\n  c 3\n", "  a 1\n- b 2\n+ b 5\n  c 3\n"},
		},
		{
			name:     "nothing changed",
			baseline: domain.BaselinePrevious,
			context:  -1,
			outputs:  []string{"a 1\n", "a 1\n"},
			want:     []string{"  a 1\n", "(output unchanged)\n  a 1\n"},
		},
		{
			name:     "back as in the first run",
			baseline: domain.BaselineFirst,
			context:  -1,
			outputs:  []string{"a 1\n", "a 2\n", "a 1\n"},
			want:     []string{"  a 1\n", "- a 1\n+ a 2\n", "(output as in the baseline run)\n  a 1\n"},
		},
		{
			name:     "lines left out are counted",
			baseline: domain.BaselinePrevious,
			context:  0,
			outputs:  []string{"1\n2\n3\n4\n", "1\n2\n3\nfour\n"},
			want:     []string{"  1\n  2\n  3\n  4\n", "  (3 unchanged lines)\n- 4\n+ four\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			screens := diffScreens(show(t, onTerminal, domain.ModeDiff, domain.NewComparison(tt.baseline, tt.context), tt.outputs...))
			if len(screens) != len(tt.want) {
				t.Fatalf("drew %d screens, want one a run", len(screens))
			}
			for i, want := range tt.want {
				if body := runBody(screens[i]); body != want {
					t.Errorf("run %d showed\n%s\nwant\n%s", i+1, body, want)
				}
			}
		})
	}
}

func TestScreenDiffColours(t *testing.T) {
	c := domain.NewComparison(domain.BaselinePrevious, -1)
	c.Compare("a 1\nb 2\n", succeeded)
	d, _ := c.Compare("a 1\nb 3\n", succeeded)
	var stdout bytes.Buffer
	s := terminal.NewScreen(&stdout, &stdout, terminal.Options{Color: true})
	s.ShowEnd(watch.RunEnd{RunStart: watch.RunStart{Mode: domain.ModeDiff}, Outcome: succeeded, Diff: &d})
	hl := func(s string) string { return reverse + s + reverseOff }
	want := "  a 1\n" +
		red + "- b " + hl("2") + reset + "\n" +
		green + "+ b " + hl("3") + reset + "\n"
	if got, _, _ := strings.Cut(stdout.String(), dim+"ok"); got != want {
		t.Errorf("wrote %q, want %q", got, want)
	}
}

func TestScreenDiffKeepsTheScreenUpWhileRunning(t *testing.T) {
	out := show(t, onTerminal, domain.ModeDiff, domain.NewComparison(domain.BaselinePrevious, -1), "a 1\n", "a 2\n")
	// One clear before the first run, and one as each run's screen is drawn.
	if n := strings.Count(out, clearScreen); n != 3 {
		t.Errorf("cleared the screen %d times, want 3:\n%q", n, out)
	}
	_, second, _ := strings.Cut(out, "ok  ") // after the first run's footer
	status, _, _ := strings.Cut(second, clearScreen)
	if !strings.Contains(status, topLine) || !strings.HasSuffix(status, "  cat out.txt  running") {
		t.Errorf("between the runs wrote %q, want only the top line rewritten to say the next is running", status)
	}
}

func TestScreenHeaderAndFooter(t *testing.T) {
	var stdout bytes.Buffer
	s := terminal.NewScreen(&stdout, &stdout, terminal.Options{})
	start := watch.RunStart{
		At:     time.Date(2026, 10, 3, 23, 33, 28, 0, time.Local),
		Change: domain.Change{Modified: []string{"sales.csv"}, Unmatched: []domain.Pattern{must(domain.NewGlob("reprot.awk"))}},
		Argv:   domain.Argv{"echo", "hi"},
		Mode:   domain.ModeAppend,
	}
	s.ShowStart(start)
	stdout.WriteString("hi\n")
	s.ShowEnd(watch.RunEnd{RunStart: start, Outcome: succeeded})
	want := "23:33:28  sales.csv changed  echo hi\nnothing matches reprot.awk yet\nhi\nok  4ms\n"
	if stdout.String() != want {
		t.Errorf("printed\n%s\nwant a header, a line for the pattern that matches nothing, the output and a footer:\n%s", stdout.String(), want)
	}
}

func TestScreenTrigger(t *testing.T) {
	tests := []struct {
		change domain.Change
		want   string
	}{
		{change: domain.Change{Files: []string{"a.csv"}}, want: "00:00:00  cat\n"},
		{change: domain.Change{Modified: []string{"a.csv"}}, want: "00:00:00  a.csv changed  cat\n"},
		{change: domain.Change{Added: []string{"march.csv"}}, want: "00:00:00  march.csv added  cat\n"},
		{change: domain.Change{Removed: []string{"old.csv"}}, want: "00:00:00  old.csv removed  cat\n"},
		{change: domain.Change{Added: []string{"b"}, Removed: []string{"c"}, Modified: []string{"a"}}, want: "00:00:00  a changed, and 2 more  cat\n"},
	}
	for _, tt := range tests {
		var stdout bytes.Buffer
		terminal.NewScreen(&stdout, &stdout, terminal.Options{}).ShowStart(watch.RunStart{
			At:     time.Date(2026, 10, 3, 0, 0, 0, 0, time.Local),
			Change: tt.change,
			Argv:   domain.Argv{"cat"},
			Mode:   domain.ModeAppend,
		})
		if stdout.String() != tt.want {
			t.Errorf("for %+v, the header is %q, want %q", tt.change, stdout.String(), tt.want)
		}
	}
}

func TestScreenFooter(t *testing.T) {
	tests := []struct {
		outcome  domain.Outcome
		want     string
		wantBell bool
	}{
		{outcome: succeeded, want: "ok  4ms\n"},
		{outcome: domain.OutcomeExit(3, 1200*time.Millisecond), want: "exit 3  1.2s\n", wantBell: true},
		{outcome: domain.OutcomeError("signal: killed", time.Second), want: "signal: killed  1s\n", wantBell: true},
		{outcome: domain.OutcomeTimedOut(100*time.Millisecond, 110*time.Millisecond), want: "timed out after 100ms\n", wantBell: true},
		{outcome: domain.OutcomeStopped(time.Second), want: "stopped  1s\n"},
	}
	for _, tt := range tests {
		var stdout bytes.Buffer
		s := terminal.NewScreen(&stdout, &stdout, terminal.Options{Bell: true})
		s.ShowEnd(watch.RunEnd{RunStart: watch.RunStart{Mode: domain.ModeAppend}, Outcome: tt.outcome})
		got, rang := strings.CutSuffix(stdout.String(), bell)
		if got != tt.want || rang != tt.wantBell {
			t.Errorf("a run that ended %s wrote %q, rang the bell: %v; want %q, %v", tt.outcome.Ending(), got, rang, tt.want, tt.wantBell)
		}
	}
}

func TestScreenNoBellUnlessAsked(t *testing.T) {
	var stdout bytes.Buffer
	terminal.NewScreen(&stdout, &stdout, terminal.Options{}).ShowEnd(watch.RunEnd{Outcome: domain.OutcomeExit(1, 0)})
	if strings.Contains(stdout.String(), bell) {
		t.Errorf("rang the bell without --bell: %q", stdout.String())
	}
}

func TestScreenDiffDrawsNothingOfAStoppedRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	s := terminal.NewScreen(&stdout, &stderr, onTerminal)
	start := watch.RunStart{Argv: domain.Argv{"cat"}, Mode: domain.ModeDiff}
	s.ShowStart(start)
	s.ShowEnd(watch.RunEnd{RunStart: start, Outcome: domain.OutcomeStopped(time.Second), Stderr: []byte("partial\n")})
	out := uncolored(stdout.String())
	if !strings.HasSuffix(out, "  cat  stopped") || strings.Count(out, clearScreen) != 1 {
		t.Errorf("wrote %q, want only the top line to say the run was stopped", out)
	}
	if stderr.Len() > 0 {
		t.Errorf("wrote %q to stderr, want nothing of a run that was stopped", stderr.String())
	}
}

func TestScreenDiffShowsStderrAboveTheOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	s := terminal.NewScreen(&stdout, &stderr, terminal.Options{})
	c := domain.NewComparison(domain.BaselinePrevious, -1)
	d, _ := c.Compare("a 1\n", succeeded)
	start := watch.RunStart{Mode: domain.ModeDiff}
	s.ShowEnd(watch.RunEnd{RunStart: start, Outcome: succeeded, Stderr: []byte("warning\n"), Diff: &d})
	if stderr.String() != "warning\n" || !strings.HasPrefix(stdout.String(), "  a 1\n") {
		t.Errorf("wrote %q and %q, want the stderr kept from the run and the output", stdout.String(), stderr.String())
	}
}

func TestScreenWithoutColorOrClear(t *testing.T) {
	out := show(t, terminal.Options{}, domain.ModeDiff, domain.NewComparison(domain.BaselinePrevious, -1), "a 1\n", "a 2\n")
	if strings.Contains(out, "\033") {
		t.Errorf("printed %q, want no escape codes", out)
	}
	if !strings.Contains(out, "- a 1\n+ a 2\n") {
		t.Errorf("printed %q, want the change marked without colours", out)
	}
}

func TestScreenNotes(t *testing.T) {
	var stdout bytes.Buffer
	s := terminal.NewScreen(&stdout, &stdout, terminal.Options{})
	s.ShowPaused(true)
	s.ShowPaused(false)
	s.ShowRebased()
	want := "paused: changes wait until p is pressed again\nwatching again\n(the next run is compared with this one)\n"
	if stdout.String() != want {
		t.Errorf("wrote %q, want %q", stdout.String(), want)
	}
}

func TestOptionsFor(t *testing.T) {
	if got := terminal.OptionsFor(&bytes.Buffer{}, false, true); got != (terminal.Options{Bell: true}) {
		t.Errorf("for output that is not a terminal, got %+v, want no colours and no clearing", got)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
