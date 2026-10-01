package internal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	clearScreen = "\033[H\033[2J"
	dim         = "\033[2m"
	red         = "\033[31m"
	green       = "\033[32m"
	reverse     = "\033[7m"
	reverseOff  = "\033[27m"
	reset       = "\033[0m"
)

// palette is the escape codes a Runner colours its output with.
type palette struct {
	dim, red, green, reverse, reverseOff, reset string
}

var (
	colors   = palette{dim, red, green, reverse, reverseOff, reset}
	noColors = palette{}
)

// Runner runs a command and shows its output the way its Mode says.
type Runner struct {
	argv   []string
	mode   Mode
	stdout io.Writer
	stderr io.Writer

	baseline    Baseline
	context     int // the lines ModeDiff shows around a change, or below 0 for all of them
	mergeStderr bool
	palette     palette
	clear       bool
	group       bool

	// ran and last are the output ModeDiff compares against, which baseline picks.
	ran  bool
	last string
}

// RunnerOption changes how a Runner built by NewRunner runs its command, or
// shows what it printed.
type RunnerOption func(*Runner)

// WithBaseline makes ModeDiff compare each run with the run baseline names,
// rather than with the previous one.
func WithBaseline(baseline Baseline) RunnerOption {
	return func(r *Runner) {
		r.baseline = baseline
	}
}

// WithContextLines makes ModeDiff show only the lines that changed and lines
// of those that did not around each, rather than the whole output.
func WithContextLines(lines int) RunnerOption {
	return func(r *Runner) {
		r.context = lines
	}
}

// WithMergedStderr sends the command's stderr where its stdout goes, so
// ModeDiff compares it too.
func WithMergedStderr() RunnerOption {
	return func(r *Runner) {
		r.mergeStderr = true
	}
}

// WithoutColor leaves the escape codes for colours out of what the Runner writes.
func WithoutColor() RunnerOption {
	return func(r *Runner) {
		r.palette = noColors
	}
}

// WithoutClear leaves the screen as it is before each run.
func WithoutClear() RunnerOption {
	return func(r *Runner) {
		r.clear = false
	}
}

// WithProcessGroup runs the command in a process group of its own, so that
// stopping it stops every process it started as well; see stopGroup.
func WithProcessGroup() RunnerOption {
	return func(r *Runner) {
		r.group = true
	}
}

// NewRunner builds a Runner for argv, the command and its arguments.
func NewRunner(argv []string, mode Mode, stdout, stderr io.Writer, opts ...RunnerOption) *Runner {
	r := &Runner{
		argv:     argv,
		mode:     mode,
		stdout:   stdout,
		stderr:   stderr,
		baseline: BaselinePrevious,
		context:  -1,
		palette:  colors,
		clear:    true,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Command returns the command Run runs for files: every argument that is
// exactly {} replaced by files. A file the command already names as an
// argument of its own, as awk -f names its script, is left out of {} rather
// than passed twice.
func (r *Runner) Command(files []string) []string {
	named := func(file string) bool {
		return slices.ContainsFunc(r.argv, func(arg string) bool {
			return filepath.Clean(arg) == filepath.Clean(file)
		})
	}
	var argv []string
	for _, arg := range r.argv {
		if arg != "{}" {
			argv = append(argv, arg)
			continue
		}
		for _, file := range files {
			if !named(file) {
				argv = append(argv, file)
			}
		}
	}
	return argv
}

// Run runs the command once, as Command spells it for files. A command that
// fails is reported rather than returned: the next change runs it again, as
// it would after one that succeeded. When ctx is done the command is stopped,
// and nothing is reported.
func (r *Runner) Run(ctx context.Context, files []string) {
	argv := r.Command(files)

	if r.clear && r.mode != ModeAppend {
		fmt.Fprint(r.stdout, clearScreen)
	}
	fmt.Fprintf(r.stdout, "%s%s  %s%s\n", r.palette.dim, time.Now().Format("15:04:05"), strings.Join(argv, " "), r.palette.reset)

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	if r.group {
		stopGroup(cmd)
	}
	cmd.Stderr = r.stderr
	var out bytes.Buffer
	if r.mode == ModeDiff {
		cmd.Stdout = &out
	} else {
		cmd.Stdout = r.stdout
	}
	if r.mergeStderr {
		cmd.Stderr = cmd.Stdout
	}
	err := cmd.Run()

	if r.mode == ModeDiff {
		r.showDiff(out.String())
	}
	if err != nil && ctx.Err() == nil {
		fmt.Fprintf(r.stderr, "%swatcheroo: %v%s\n", r.palette.red, err, r.palette.reset)
	}
}

// showDiff prints the output with what changed since the baseline run marked
// in place; see writeMarked. The first run is shown whole, since it has
// nothing to compare with.
func (r *Runner) showDiff(output string) {
	prev, ran := r.last, r.ran
	if !ran || r.baseline == BaselinePrevious {
		r.last, r.ran = output, true
	}
	around := r.context
	switch {
	case !ran:
		prev, around = output, -1 // nothing to compare with, so nothing is marked
	case prev == output && r.baseline == BaselineFirst:
		fmt.Fprintf(r.stdout, "%s(output as in the first run)%s\n", r.palette.dim, r.palette.reset)
	case prev == output:
		fmt.Fprintf(r.stdout, "%s(output unchanged)%s\n", r.palette.dim, r.palette.reset)
	}
	writeMarked(r.stdout, r.palette, prev, output, around)
}
