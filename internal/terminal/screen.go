// Package terminal is where a watch meets the person watching it: Screen
// draws each run on the terminal, or writes it plainly where the output is
// not one, and Keys reads the keys they press. It is the adapter for
// watch.Display and watch.Requests, and the one place that knows which key
// does what, so the words on the screen name the right keys.
package terminal

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/romanidis/watcheroo/internal/domain"
	"github.com/romanidis/watcheroo/internal/watch"
)

var _ watch.Display = (*Screen)(nil)

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

// palette is the escape codes a Screen colours its output with.
type palette struct {
	dim, red, green, reverse, reverseOff, reset string
}

var (
	colors   = palette{dim, red, green, reverse, reverseOff, reset}
	noColors = palette{}
)

// Options say how a Screen draws.
type Options struct {
	Color bool // colour the output
	Clear bool // clear the screen as the mode says, and redraw it in diff mode
	Bell  bool // ring the bell when a run fails or times out
}

// OptionsFor returns the Options for drawing on stdout: colours and
// clearing only on a terminal, colours not even there with noColor, and the
// bell when bell asks for it.
func OptionsFor(stdout io.Writer, noColor, bell bool) Options {
	tty := IsTerminal(stdout)
	return Options{Color: tty && !noColor, Clear: tty, Bell: bell}
}

// IsTerminal reports whether w is a terminal, the only place where colours,
// clearing the screen and keys make sense.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// Screen shows each run under a header that says when, why and what, and
// above a footer that says how it went and how long it took.
//
// In diff mode on a screen it can clear, it leaves the last run's screen up
// while the next one goes on, with only its top line saying so, and draws
// the screen whole once the run ends.
type Screen struct {
	stdout, stderr io.Writer
	palette        palette
	clear          bool
	bell           bool

	drawn bool // whether diff mode has drawn a run's screen yet
}

// NewScreen returns a Screen that draws on stdout, and writes a command's
// stderr to stderr.
func NewScreen(stdout, stderr io.Writer, o Options) *Screen {
	s := &Screen{stdout: stdout, stderr: stderr, palette: noColors, clear: o.Clear, bell: o.Bell}
	if o.Color {
		s.palette = colors
	}
	return s
}

// Streams returns where the Screen writes, for a command to write to as well.
func (s *Screen) Streams() (io.Writer, io.Writer) {
	return s.stdout, s.stderr
}

// ShowStart clears the screen in clear mode, and writes the header. In diff
// mode on a screen it redraws, it only says on the top line that the run is
// going.
func (s *Screen) ShowStart(r watch.RunStart) {
	head := s.header(r)
	switch {
	case s.redraws(r.Mode):
		if !s.drawn {
			fmt.Fprint(s.stdout, clearScreen)
		}
		s.status(head, "running")
	case s.clear && r.Mode == domain.ModeClear:
		fmt.Fprint(s.stdout, clearScreen)
		s.writeHead(head, r.Change.Unmatched)
	default:
		s.writeHead(head, r.Change.Unmatched)
	}
}

// ShowEnd writes the footer, after the stderr and the diff of a mode that
// compares, all of it on a screen of its own when the Screen redraws. A run
// that was stopped shows nothing but that, as what it printed is out of
// date.
func (s *Screen) ShowEnd(r watch.RunEnd) {
	o := r.Outcome
	if o.Ending() == domain.EndingStopped {
		if s.redraws(r.Mode) {
			s.status(s.header(r.RunStart), "stopped")
		} else {
			fmt.Fprintf(s.stdout, "%sstopped  %v%s\n", s.palette.dim, o.Took(), s.palette.reset)
		}
		return
	}
	if s.redraws(r.Mode) {
		fmt.Fprint(s.stdout, clearScreen)
		s.writeHead(s.header(r.RunStart), r.Change.Unmatched)
		s.drawn = true
	}
	s.stderr.Write(r.Stderr)
	if r.Diff != nil {
		s.writeDiff(*r.Diff)
	}
	p := s.palette
	switch o.Ending() {
	case domain.EndingTimedOut:
		fmt.Fprintf(s.stdout, "%stimed out after %v%s\n", p.red, o.Timeout(), p.reset)
	case domain.EndingExit:
		fmt.Fprintf(s.stdout, "%sexit %d  %v%s\n", p.red, o.Code(), o.Took(), p.reset)
	case domain.EndingError:
		fmt.Fprintf(s.stdout, "%s%s  %v%s\n", p.red, o.Reason(), o.Took(), p.reset)
	default:
		fmt.Fprintf(s.stdout, "%sok  %v%s\n", p.dim, o.Took(), p.reset)
	}
	if o.Failed() && s.bell {
		fmt.Fprint(s.stdout, bell)
	}
}

// ShowPaused says the watch is paused, and how to go on with it, or that it
// went on.
func (s *Screen) ShowPaused(paused bool) {
	if paused {
		s.note("paused: changes wait until p is pressed again")
	} else {
		s.note("watching again")
	}
}

// ShowRebased says the next run is compared with the one on screen.
func (s *Screen) ShowRebased() {
	s.note("(the next run is compared with this one)")
}

// redraws reports whether the Screen draws a run in mode on a screen of its
// own, once it ends.
func (s *Screen) redraws(mode domain.Mode) bool {
	return mode.Compares() && s.clear
}

// header returns the line above a run's output: when it started, what
// changed to start it, and the command.
func (s *Screen) header(r watch.RunStart) string {
	p := s.palette
	header := p.dim + r.At.Format(time.TimeOnly) + p.reset
	if why := trigger(r.Change); why != "" {
		header += "  " + why
	}
	return header + "  " + p.dim + r.Argv.String() + p.reset
}

// trigger says what changed to start a run: the first file changed, added or
// removed, and how many more were.
func trigger(c domain.Change) string {
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
// no file, which is likely a typo. It is under every run rather than once at
// the start, where the first clear would wipe it.
func (s *Screen) writeHead(header string, unmatched []domain.Pattern) {
	fmt.Fprintln(s.stdout, header)
	for _, p := range unmatched {
		fmt.Fprintf(s.stdout, "%snothing matches %s yet%s\n", s.palette.red, p, s.palette.reset)
	}
}

// status writes header on the top line of the screen, saying how the run is
// going, and leaves the rest of the screen as it is.
func (s *Screen) status(header, how string) {
	fmt.Fprintf(s.stdout, "%s%s  %s%s%s", topLine, header, s.palette.dim, how, s.palette.reset)
}

// note writes text on a line of its own, dimmed, as a word from wtr rather
// than from the command.
func (s *Screen) note(text string) {
	fmt.Fprintf(s.stdout, "%s%s%s\n", s.palette.dim, text, s.palette.reset)
}
