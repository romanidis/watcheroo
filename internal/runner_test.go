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
		r.Run(context.Background(), nil)
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
			screens := strings.Split(runTwice(t, ModeDiff, tt.first, tt.second), clearScreen)[1:]
			if len(screens) != 2 {
				t.Fatalf("cleared the screen %d times, want once a run", len(screens))
			}
			for i, want := range []string{tt.wantFirst, tt.wantSecond} {
				_, body, _ := strings.Cut(screens[i], "\n") // after the header
				if body != want {
					t.Errorf("run %d showed\n%s\nwant\n%s", i+1, body, want)
				}
			}
		})
	}
}

func TestRunnerReportsFailureAndCarriesOn(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := NewRunner([]string{"sh", "-c", "echo boom >&2; exit 3"}, ModeAppend, &stdout, &stderr)
	r.Run(context.Background(), nil)
	r.Run(context.Background(), nil)
	if n := strings.Count(stderr.String(), "watcheroo: exit status 3"); n != 2 {
		t.Errorf("stderr is %q, want the exit status reported after each run", stderr.String())
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
			argv:  []string{"sh", "-c", `printf '<%s>' "$@"`, "sh", "{}"},
			files: []string{"a.csv", "b c.csv"},
			want:  "<a.csv><b c.csv>",
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
			name:  "{} inside a longer argument is left alone",
			argv:  []string{"echo", "x{}"},
			files: []string{"a.csv"},
			want:  "x{}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			NewRunner(tt.argv, ModeAppend, &stdout, &stderr).Run(context.Background(), tt.files)
			_, output, _ := strings.Cut(stdout.String(), "\n")
			if output != tt.want {
				t.Errorf("printed %q after the header, want %q (stderr %q)", output, tt.want, stderr.String())
			}
		})
	}
}

func TestRunnerDiffBaselineFirst(t *testing.T) {
	out := runEach(t, ModeDiff, []string{"a 1\n", "a 2\n", "a 3\n", "a 1\n"}, WithBaseline(BaselineFirst))
	screens := strings.Split(out, clearScreen)[1:]
	want := []string{
		"  a 1\n",
		"- a 1\n+ a 2\n",
		"- a 1\n+ a 3\n",
		"(output as in the first run)\n  a 1\n",
	}
	if len(screens) != len(want) {
		t.Fatalf("cleared the screen %d times, want once a run", len(screens))
	}
	for i := range want {
		_, body, _ := strings.Cut(screens[i], "\n") // after the header
		if body != want[i] {
			t.Errorf("run %d showed\n%s\nwant\n%s", i+1, body, want[i])
		}
	}
}

func TestRunnerDiffContextShowsTheFirstRunWhole(t *testing.T) {
	out := runEach(t, ModeDiff, []string{"1\n2\n3\n4\n", "1\n2\n3\nfour\n"}, WithContextLines(0))
	screens := strings.Split(out, clearScreen)[1:]
	want := []string{
		"  1\n  2\n  3\n  4\n",
		"  (3 unchanged lines)\n- 4\n+ four\n",
	}
	for i := range want {
		_, body, _ := strings.Cut(screens[i], "\n") // after the header
		if body != want[i] {
			t.Errorf("run %d showed\n%s\nwant\n%s", i+1, body, want[i])
		}
	}
}

func TestRunnerMergedStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := NewRunner([]string{"sh", "-c", "echo out; echo err >&2"}, ModeDiff, &stdout, &stderr, WithMergedStderr())
	r.Run(context.Background(), nil)
	if stderr.Len() > 0 {
		t.Errorf("stderr is %q, want it in with stdout", stderr.String())
	}
	if !strings.HasSuffix(stdout.String(), "  out\n  err\n") {
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
		r.Run(context.Background(), nil)
	}
	if out := stdout.String() + stderr.String(); strings.Contains(out, "\033") {
		t.Errorf("printed %q, want no escape codes", out)
	}
	if !strings.HasSuffix(stdout.String(), "- a 1\n+ a 2\n") {
		t.Errorf("printed %q, want the change marked without colours", stdout.String())
	}
}

func TestRunnerProcessGroupStopsWhatTheCommandStarted(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout, stderr bytes.Buffer
	// sleep holds the stdout pipe open, so the run ends only once sleep does,
	// and the trailing true keeps sh from becoming sleep.
	r := NewRunner([]string{"sh", "-c", "touch started; sleep 30; true"}, ModeAppend, &stdout, &stderr, WithProcessGroup())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Run(ctx, nil)
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
}
