package internal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

const (
	// clearScreen clears the scrollback too, so scrolling up does not show
	// the runs before.
	clearScreen = "\033[H\033[2J\033[3J"
	topLine     = "\033[H\033[2K" // the cursor to the top line, cleared
	bell        = "\a"
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
	bell        bool
	timeout     time.Duration // how long a run may take, or 0 for as long as it likes

	drawn bool // whether ModeDiff has drawn a run's screen yet

	mu sync.Mutex // guards what follows, which NewBaseline changes while a run goes on
	// ran and last are the output ModeDiff compares against, which baseline
	// picks, and latest is the output of the last run it showed.
	ran    bool
	last   string
	latest string
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

// WithBell rings the terminal bell when a run fails or times out.
func WithBell() RunnerOption {
	return func(r *Runner) {
		r.bell = true
	}
}

// WithTimeout stops a run that takes longer than d, and reports it.
func WithTimeout(d time.Duration) RunnerOption {
	return func(r *Runner) {
		r.timeout = d
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
// exactly {} replaced by files. A file the command already names itself, as
// awk -f names its script, is left out of {} rather than passed twice; see
// names.
func (r *Runner) Command(files []string) []string {
	names := r.names()
	named := func(file string) bool {
		return slices.ContainsFunc(names, func(name string) bool {
			return filepath.Clean(name) == filepath.Clean(file)
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

// names returns the words the command can name a file with: its arguments,
// and when it has a shell run a script with -c, the words of the script.
func (r *Runner) names() []string {
	names := slices.Clone(r.argv)
	if script, ok := shellScript(r.argv); ok {
		names = append(names, shellWords(script)...)
	}
	return names
}

// Run runs the command once, as Command spells it for the files of c, under a
// header that says when, why and what, and above a footer that says how it
// went and how long it took. A command that fails, or runs longer than
// WithTimeout allows, is reported rather than returned: the next change runs
// it again, as it would after one that succeeded. When ctx is done the
// command is stopped, and what it printed so far is not shown, as it is out
// of date.
//
// ModeDiff on a screen it can clear leaves the last run's screen up while the
// next one goes on, with only its top line saying so, and draws the screen
// whole once the run ends.
func (r *Runner) Run(ctx context.Context, c Change) {
	argv := r.Command(c.Files)
	header := r.header(c, argv)
	redraw := r.mode == ModeDiff && r.clear
	switch {
	case redraw:
		if !r.drawn {
			fmt.Fprint(r.stdout, clearScreen)
		}
		r.status(header, "running")
	case r.clear && r.mode != ModeAppend:
		fmt.Fprint(r.stdout, clearScreen)
		r.writeHead(header, c.Unmatched)
	default:
		r.writeHead(header, c.Unmatched)
	}

	runCtx := ctx
	if r.timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	stopGroup(cmd)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = r.stdout, r.stderr
	if r.mode == ModeDiff {
		cmd.Stdout, cmd.Stderr = &out, &errOut
	}
	if r.mergeStderr {
		cmd.Stderr = cmd.Stdout
	}
	start := time.Now()
	err := cmd.Run()
	took := time.Since(start).Round(time.Millisecond)

	if ctx.Err() != nil {
		if redraw {
			r.status(header, "stopped")
		} else {
			fmt.Fprintf(r.stdout, "%sstopped  %v%s\n", r.palette.dim, took, r.palette.reset)
		}
		return
	}
	timedOut := err != nil && runCtx.Err() != nil
	if r.mode == ModeDiff {
		if redraw {
			fmt.Fprint(r.stdout, clearScreen)
			r.writeHead(header, c.Unmatched)
			r.drawn = true
		}
		r.stderr.Write(errOut.Bytes())
		if !timedOut {
			r.showDiff(out.String())
		}
	}
	switch {
	case timedOut:
		fmt.Fprintf(r.stdout, "%stimed out after %v%s\n", r.palette.red, r.timeout, r.palette.reset)
	case err != nil:
		fmt.Fprintf(r.stdout, "%s%s  %v%s\n", r.palette.red, failure(err), took, r.palette.reset)
	default:
		fmt.Fprintf(r.stdout, "%sok  %v%s\n", r.palette.dim, took, r.palette.reset)
	}
	if err != nil && r.bell {
		fmt.Fprint(r.stdout, bell)
	}
}

// header returns the line above a run's output: when it started, what
// changed to start it, and the command.
func (r *Runner) header(c Change, argv []string) string {
	p := r.palette
	header := p.dim + time.Now().Format("15:04:05") + p.reset
	if why := trigger(c); why != "" {
		header += "  " + why
	}
	return header + "  " + p.dim + JoinCommand(argv) + p.reset
}

// trigger says what changed to start a run: the first file changed, added or
// removed, and how many more were.
func trigger(c Change) string {
	var first string
	switch {
	case len(c.Modified) > 0:
		first = c.Modified[0] + " changed"
	case len(c.Added) > 0:
		first = c.Added[0] + " added"
	case len(c.Removed) > 0:
		first = c.Removed[0] + " removed"
	default:
		return ""
	}
	if more := len(c.Modified) + len(c.Added) + len(c.Removed) - 1; more > 0 {
		return fmt.Sprintf("%s, and %d more", first, more)
	}
	return first
}

// writeHead writes header, and below it a line for each pattern that matches
// no file, which is likely a typo.
func (r *Runner) writeHead(header string, unmatched []Pattern) {
	fmt.Fprintln(r.stdout, header)
	for _, p := range unmatched {
		fmt.Fprintf(r.stdout, "%snothing matches %s yet%s\n", r.palette.red, p, r.palette.reset)
	}
}

// status writes header on the top line of the screen, saying how the run is
// going, and leaves the rest of the screen as it is.
func (r *Runner) status(header, how string) {
	fmt.Fprintf(r.stdout, "%s%s  %s%s%s", topLine, header, r.palette.dim, how, r.palette.reset)
}

// failure says how a command that did not succeed ended: its exit status, or
// what stopped it.
func failure(err error) string {
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() >= 0 {
		return fmt.Sprintf("exit %d", exit.ExitCode())
	}
	return err.Error()
}

// NewBaseline makes ModeDiff compare the next run with the last one it
// showed, as it compares every run with the first under BaselineFirst.
func (r *Runner) NewBaseline() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.mode != ModeDiff || !r.ran {
		return
	}
	r.last = r.latest
	r.Note("(the next run is compared with this one)")
}

// Note writes text on a line of its own, dimmed, as a word from wtr rather
// than from the command.
func (r *Runner) Note(text string) {
	fmt.Fprintf(r.stdout, "%s%s%s\n", r.palette.dim, text, r.palette.reset)
}

// showDiff prints the output with what changed since the baseline run marked
// in place; see writeMarked. The first run is shown whole, since it has
// nothing to compare with.
func (r *Runner) showDiff(output string) {
	r.mu.Lock()
	prev, ran := r.last, r.ran
	if !ran || r.baseline == BaselinePrevious {
		r.last, r.ran = output, true
	}
	r.latest = output
	r.mu.Unlock()
	around := r.context
	switch {
	case !ran:
		prev, around = output, -1 // nothing to compare with, so nothing is marked
	case prev == output && r.baseline == BaselineFirst:
		fmt.Fprintf(r.stdout, "%s(output as in the baseline run)%s\n", r.palette.dim, r.palette.reset)
	case prev == output:
		fmt.Fprintf(r.stdout, "%s(output unchanged)%s\n", r.palette.dim, r.palette.reset)
	}
	writeMarked(r.stdout, r.palette, prev, output, around)
}
