package internal

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// runTwice runs `cat out.txt` in mode with out.txt holding first, then second,
// and returns all it printed with the colours taken out.
func runTwice(t *testing.T, mode Mode, first, second string) string {
	t.Helper()
	return runEach(t, mode, []string{first, second})
}

// runEach runs `cat out.txt` in mode once for each of contents, with out.txt
// holding it, and returns all it printed with the colours taken out.
func runEach(t *testing.T, mode Mode, contents []string, opts ...RunnerOption) string {
	t.Helper()
	t.Chdir(t.TempDir())
	var stdout, stderr bytes.Buffer
	r := NewRunner([]string{"cat", "out.txt"}, mode, &stdout, &stderr, opts...)
	for _, content := range contents {
		if err := os.WriteFile("out.txt", []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		r.Run(context.Background(), Change{})
	}
	if stderr.Len() > 0 {
		t.Fatalf("stderr: %s", stderr.String())
	}
	return strings.NewReplacer(dim, "", red, "", green, "", reverse, "", reverseOff, "", reset, "").Replace(stdout.String())
}

// afterLastClear is what is left on screen after the last clear.
func afterLastClear(out string) string {
	return out[strings.LastIndex(out, clearScreen)+len(clearScreen):]
}

// diffScreens returns the screens ModeDiff drew whole, one for each run that
// ended, each without the top line a later run rewrote to say it was going.
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

func TestRunnerClear(t *testing.T) {
	out := runTwice(t, ModeClear, "one\n", "two\n")
	if strings.Count(out, clearScreen) != 2 {
		t.Errorf("cleared %d times, want once a run:\n%q", strings.Count(out, clearScreen), out)
	}
	screen := afterLastClear(out)
	if strings.Contains(screen, "one") || !strings.Contains(screen, "two\n") {
		t.Errorf("screen shows %q, want only the second run's output", screen)
	}
}

func TestRunnerAppend(t *testing.T) {
	out := runTwice(t, ModeAppend, "one\n", "two\n")
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

func TestRunnerDiff(t *testing.T) {
	tests := []struct {
		name       string
		first      string
		second     string
		wantFirst  string
		wantSecond string
	}{
		{
			name:       "a changed line",
			first:      "a 1\nb 2\nc 3\n",
			second:     "a 1\nb 5\nc 3\n",
			wantFirst:  "  a 1\n  b 2\n  c 3\n",
			wantSecond: "  a 1\n- b 2\n+ b 5\n  c 3\n",
		},
		{
			name:       "an added line",
			first:      "a 1\n",
			second:     "a 1\nz 9\n",
			wantFirst:  "  a 1\n",
			wantSecond: "  a 1\n+ z 9\n",
		},
		{
			name:       "a removed line",
			first:      "a 1\nz 9\n",
			second:     "a 1\n",
			wantFirst:  "  a 1\n  z 9\n",
			wantSecond: "  a 1\n- z 9\n",
		},
		{
			name:       "nothing changed",
			first:      "a 1\n",
			second:     "a 1\n",
			wantFirst:  "  a 1\n",
			wantSecond: "(output unchanged)\n  a 1\n",
		},
		{
			name:       "output from nothing",
			first:      "",
			second:     "a 1\n",
			wantFirst:  "",
			wantSecond: "+ a 1\n",
		},
		{
			name:       "output that ends without a newline",
			first:      "a 1",
			second:     "a 2",
			wantFirst:  "  a 1\n",
			wantSecond: "- a 1\n+ a 2\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			screens := diffScreens(runTwice(t, ModeDiff, tt.first, tt.second))
			if len(screens) != 2 {
				t.Fatalf("drew %d screens, want one a run", len(screens))
			}
			for i, want := range []string{tt.wantFirst, tt.wantSecond} {
				if body := runBody(screens[i]); body != want {
					t.Errorf("run %d showed\n%s\nwant\n%s", i+1, body, want)
				}
			}
		})
	}
}

func TestRunnerReportsFailureAndCarriesOn(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := NewRunner([]string{"sh", "-c", "echo boom >&2; exit 3"}, ModeAppend, &stdout, &stderr, WithoutColor())
	r.Run(context.Background(), Change{})
	r.Run(context.Background(), Change{})
	if n := strings.Count(stdout.String(), "\nexit 3  "); n != 2 {
		t.Errorf("printed %q, want the exit status reported after each run", stdout.String())
	}
	if !strings.Contains(stderr.String(), "boom") {
		t.Errorf("stderr is %q, want the command's own stderr passed through", stderr.String())
	}
}

func TestRunnerFillsInFiles(t *testing.T) {
	tests := []struct {
		name  string
		argv  []string
		files []string
		want  string
	}{
		{
			name:  "{} becomes one argument a file",
			argv:  []string{"sh", "-c", `printf '<%s>' "$@"; echo`, "sh", "{}"},
			files: []string{"a.csv", "b c.csv"},
			want:  "<a.csv><b c.csv>\n",
		},
		{
			name:  "{} between other arguments",
			argv:  []string{"echo", "first", "{}", "last"},
			files: []string{"a.csv"},
			want:  "first a.csv last\n",
		},
		{
			name:  "{} when nothing matches",
			argv:  []string{"echo", "first", "{}", "last"},
			files: nil,
			want:  "first last\n",
		},
		{
			name:  "{} leaves out the files the command names itself",
			argv:  []string{"echo", "-f", "./report.awk", "{}"},
			files: []string{"data/a.csv", "report.awk"},
			want:  "-f ./report.awk data/a.csv\n",
		},
		{
			name:  "{} leaves out the files the script given to sh -c names",
			argv:  []string{"sh", "-c", `true -f report.awk; printf '<%s>' "$@"; echo`, "sh", "{}"},
			files: []string{"report.awk", "a.csv"},
			want:  "<a.csv>\n",
		},
		{
			name:  "{} keeps a file only a string in an awk program names",
			argv:  []string{"sh", "-c", `printf '<%s>' "$@"; awk 'FILENAME == "a.csv"' /dev/null; echo`, "sh", "{}"},
			files: []string{"a.csv"},
			want:  "<a.csv>\n",
		},
		{
			name:  "{} keeps a file an argument names only as part of a longer word",
			argv:  []string{"echo", "FILENAME == a.csv", "{}"},
			files: []string{"a.csv"},
			want:  "FILENAME == a.csv a.csv\n",
		},
		{
			name:  "{} inside a longer argument is left alone",
			argv:  []string{"echo", "x{}"},
			files: []string{"a.csv"},
			want:  "x{}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			NewRunner(tt.argv, ModeAppend, &stdout, &stderr).Run(context.Background(), Change{Files: tt.files})
			if output := runBody(stdout.String()); output != tt.want {
				t.Errorf("printed %q after the header, want %q (stderr %q)", output, tt.want, stderr.String())
			}
		})
	}
}

func TestRunnerDiffBaselineFirst(t *testing.T) {
	out := runEach(t, ModeDiff, []string{"a 1\n", "a 2\n", "a 3\n", "a 1\n"}, WithBaseline(BaselineFirst))
	screens := diffScreens(out)
	want := []string{
		"  a 1\n",
		"- a 1\n+ a 2\n",
		"- a 1\n+ a 3\n",
		"(output as in the baseline run)\n  a 1\n",
	}
	if len(screens) != len(want) {
		t.Fatalf("drew %d screens, want one a run", len(screens))
	}
	for i := range want {
		if body := runBody(screens[i]); body != want[i] {
			t.Errorf("run %d showed\n%s\nwant\n%s", i+1, body, want[i])
		}
	}
}

func TestRunnerDiffContextShowsTheFirstRunWhole(t *testing.T) {
	out := runEach(t, ModeDiff, []string{"1\n2\n3\n4\n", "1\n2\n3\nfour\n"}, WithContextLines(0))
	screens := diffScreens(out)
	want := []string{
		"  1\n  2\n  3\n  4\n",
		"  (3 unchanged lines)\n- 4\n+ four\n",
	}
	if len(screens) != len(want) {
		t.Fatalf("drew %d screens, want one a run", len(screens))
	}
	for i := range want {
		if body := runBody(screens[i]); body != want[i] {
			t.Errorf("run %d showed\n%s\nwant\n%s", i+1, body, want[i])
		}
	}
}

func TestRunnerMergedStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := NewRunner([]string{"sh", "-c", "echo out; echo err >&2"}, ModeDiff, &stdout, &stderr, WithMergedStderr())
	r.Run(context.Background(), Change{})
	if stderr.Len() > 0 {
		t.Errorf("stderr is %q, want it in with stdout", stderr.String())
	}
	if !strings.Contains(stdout.String(), "  out\n  err\n") {
		t.Errorf("printed %q, want stdout and stderr shown as one output", stdout.String())
	}
}

func TestRunnerWithoutColorOrClear(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout, stderr bytes.Buffer
	r := NewRunner([]string{"sh", "-c", "cat out.txt; exit 1"}, ModeDiff, &stdout, &stderr, WithoutColor(), WithoutClear())
	for _, content := range []string{"a 1\n", "a 2\n"} {
		if err := os.WriteFile("out.txt", []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		r.Run(context.Background(), Change{})
	}
	if out := stdout.String() + stderr.String(); strings.Contains(out, "\033") {
		t.Errorf("printed %q, want no escape codes", out)
	}
	if !strings.Contains(stdout.String(), "- a 1\n+ a 2\n") {
		t.Errorf("printed %q, want the change marked without colours", stdout.String())
	}
}

func TestRunnerProcessGroupStopsWhatTheCommandStarted(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout, stderr bytes.Buffer
	// sleep holds the stdout pipe open, so the run ends only once sleep does,
	// and the trailing true keeps sh from becoming sleep.
	r := NewRunner([]string{"sh", "-c", "touch started; sleep 30; true"}, ModeAppend, &stdout, &stderr)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Run(ctx, Change{})
	}()
	for _, err := os.Stat("started"); err != nil; _, err = os.Stat("started") {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the run went on after its context was done; sleep was not stopped with sh")
	}
	if stderr.Len() > 0 {
		t.Errorf("stderr is %q, want nothing reported for a run that was stopped", stderr.String())
	}
	if !strings.Contains(stdout.String(), "stopped  ") {
		t.Errorf("printed %q, want the footer to say the run was stopped", stdout.String())
	}
}

func TestRunnerDiffDropsAStoppedRun(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout, stderr bytes.Buffer
	// While slow is there, a run prints and then waits to be stopped.
	r := NewRunner([]string{"sh", "-c", "cat out.txt; if [ -e slow ]; then touch started; sleep 30; fi; true"},
		ModeDiff, &stdout, &stderr, WithoutColor())
	run := func(ctx context.Context, content string) {
		t.Helper()
		if err := os.WriteFile("out.txt", []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		r.Run(ctx, Change{})
	}

	run(context.Background(), "a 1\n")

	writeFiles(t, "slow")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		run(ctx, "a 2\n")
	}()
	for _, err := os.Stat("started"); err != nil; _, err = os.Stat("started") {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the run went on after its context was done")
	}
	if err := os.Remove("slow"); err != nil {
		t.Fatal(err)
	}

	run(context.Background(), "a 1\n")

	screens := diffScreens(stdout.String())
	want := []string{
		"  a 1\n",
		// The stopped run drew no screen, and set no baseline.
		"(output unchanged)\n  a 1\n",
	}
	if len(screens) != len(want) {
		t.Fatalf("drew %d screens, want one for each run that ended", len(screens))
	}
	for i := range want {
		if body := runBody(screens[i]); body != want[i] {
			t.Errorf("run %d showed\n%s\nwant\n%s", i+1, body, want[i])
		}
	}
	if stderr.Len() > 0 {
		t.Errorf("stderr is %q, want nothing reported for a run that was stopped", stderr.String())
	}
}

func TestRunnerTimeout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := NewRunner([]string{"sh", "-c", "echo partial; sleep 30; true"}, ModeDiff, &stdout, &stderr,
		WithTimeout(100*time.Millisecond), WithoutColor())
	start := time.Now()
	r.Run(context.Background(), Change{})
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("the run took %v, want it stopped soon after its timeout", took)
	}
	screens := diffScreens(stdout.String())
	if len(screens) != 1 || !strings.HasSuffix(screens[0], "\ntimed out after 100ms\n") {
		t.Fatalf("printed %q, want the timeout reported in the footer", stdout.String())
	}
	if body := runBody(screens[0]); body != "" {
		t.Errorf("showed %q, want nothing of a run that timed out", body)
	}

	stdout.Reset()
	stderr.Reset()
	NewRunner([]string{"echo", "quick"}, ModeAppend, &stdout, &stderr, WithTimeout(time.Minute)).Run(context.Background(), Change{})
	if stderr.Len() > 0 || runBody(stdout.String()) != "quick\n" {
		t.Errorf("printed %q and %q, want a run within its timeout shown as usual", stdout.String(), stderr.String())
	}
}

func TestTrigger(t *testing.T) {
	tests := []struct {
		change Change
		want   string
	}{
		{change: Change{Files: []string{"a.csv"}}, want: ""},
		{change: Change{Modified: []string{"a.csv"}}, want: "a.csv changed"},
		{change: Change{Added: []string{"march.csv"}}, want: "march.csv added"},
		{change: Change{Removed: []string{"old.csv"}}, want: "old.csv removed"},
		{change: Change{Added: []string{"b"}, Removed: []string{"c"}, Modified: []string{"a"}}, want: "a changed, and 2 more"},
	}
	for _, tt := range tests {
		if got := trigger(tt.change); got != tt.want {
			t.Errorf("trigger(%+v) = %q, want %q", tt.change, got, tt.want)
		}
	}
}

func TestRunnerHeaderAndFooter(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := NewRunner([]string{"echo", "hi"}, ModeAppend, &stdout, &stderr, WithoutColor())
	r.Run(context.Background(), Change{Modified: []string{"sales.csv"}, Unmatched: []Pattern{{Glob: "reprot.awk"}}})
	lines := strings.Split(stdout.String(), "\n")
	if len(lines) != 5 {
		t.Fatalf("printed %q, want a header, a line for the pattern that matches nothing, the output and a footer", stdout.String())
	}
	if !strings.HasSuffix(lines[0], "  sales.csv changed  echo hi") {
		t.Errorf("header is %q, want it to say what changed and the command", lines[0])
	}
	if lines[1] != "nothing matches reprot.awk yet" {
		t.Errorf("second line is %q, want it to name the pattern that matches nothing", lines[1])
	}
	if lines[2] != "hi" || !strings.HasPrefix(lines[3], "ok  ") {
		t.Errorf("printed %q, want the output and then a footer saying it went well", stdout.String())
	}
}

func TestRunnerBell(t *testing.T) {
	for _, tt := range []struct {
		argv []string
		want bool
	}{
		{argv: []string{"false"}, want: true},
		{argv: []string{"true"}, want: false},
	} {
		var stdout, stderr bytes.Buffer
		NewRunner(tt.argv, ModeAppend, &stdout, &stderr, WithBell()).Run(context.Background(), Change{})
		if got := strings.Contains(stdout.String(), bell); got != tt.want {
			t.Errorf("%s rang the bell: %v, want %v", tt.argv[0], got, tt.want)
		}
	}
}

func TestRunnerDiffKeepsTheScreenUpWhileRunning(t *testing.T) {
	out := runTwice(t, ModeDiff, "a 1\n", "a 2\n")
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

func TestRunnerNewBaseline(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout, stderr bytes.Buffer
	r := NewRunner([]string{"cat", "out.txt"}, ModeDiff, &stdout, &stderr, WithBaseline(BaselineFirst), WithoutColor())
	for i, content := range []string{"a 1\n", "a 2\n", "a 2\n"} {
		if i == 2 {
			r.NewBaseline()
		}
		if err := os.WriteFile("out.txt", []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		r.Run(context.Background(), Change{})
	}
	screens := diffScreens(stdout.String())
	if body := runBody(screens[len(screens)-1]); body != "(output as in the baseline run)\n  a 2\n" {
		t.Errorf("the run after the new baseline showed\n%s\nwant it compared with the run before", body)
	}
}
