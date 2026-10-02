package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/romanidis/watcheroo/internal"
	"github.com/spf13/cobra"
)

// NewRootCmd builds the watcheroo command.
func NewRootCmd() *cobra.Command {
	var (
		patterns     = &patternList{}
		mode         string
		interval     time.Duration
		exclude      []string
		excludeRegex []string
		hidden       bool
		list         bool
		postpone     bool
		restart      bool
		debounce     time.Duration
		noColor      bool
		baseline     string
		contextLines int
		mergeStderr  bool
		timeout      time.Duration
		shell        bool
		ringBell     bool
	)
	cmd := &cobra.Command{
		Use:   "wtr [--watch] GLOB... [--regex REGEX]... -- COMMAND [ARG...]",
		Short: "Run a command again whenever the files it reads change",
		Long: `Run a command, then run it again each time one of the watched files is
created, deleted or changed.

Everything before -- that is not a flag is another file to watch, so
--watch a.txt b.txt watches both. A name can be a glob, which wtr expands
again on every poll, so a file created later is picked up too. Quote it so
the shell does not expand it first. In a glob, ** matches any number of
directories, and {csv,tsv} either csv or tsv. A directory stands for every
file under it.

A --regex is matched against the path of every file under the working
directory, anywhere in the path unless it is anchored with ^. Anchor it with
a directory and only that directory is searched:

    --regex '^data/.*\.csv$'

Hidden files and directories, whose names start with a dot, are not watched
unless a glob names them with the dot, like .env, or --hidden is given. That
keeps editor swap files and .git out. Nor are the backups and temporary files
editors leave, like report.awk~ and #report.awk#, unless a glob names them
so, like '*~'.

--exclude leaves out a file or a directory and everything under it. A glob
with no slash in it is matched against each name along the path, so
--exclude node_modules leaves out every directory of that name; one with a
slash is matched against the whole path. --exclude-regex is matched against
the path as --regex is. --list prints the files watched, the patterns that
match none, and the command they make, then stops, which shows what a
pattern matches.

An argument that is exactly {} becomes the watched files, one argument each,
in the order their globs and regexes were given; the files one glob or regex
matches come in path order. A file the command already names as an argument
of its own, like the script in awk -f report.awk, is watched but left out of
{}.

Above each run's output is when it started, the file whose change started
it, and the command, and below that each pattern that matches no file, which
may be a typo. Below the output is how the run ended, and how long it took.
--bell rings the terminal bell when a run fails.

--mode says what happens to the previous run's output:

    clear    clear the screen first, so only the latest output shows (default)
    append   keep it, and print the next run below it
    diff     mark what changed since the last run

In diff mode, the screen stays as it is while the next run goes on, and is
drawn again once that run ends. A change that comes while a run is still
going stops it and starts the command again, since what it would show is
already out of date. --baseline first compares every run with the first
rather than with the one before it, and --context N shows only the lines
that changed and N lines around each. --merge-stderr sends the command's
stderr where its stdout goes, so diff mode compares it too.

Clearing the screen clears the terminal's scrollback as well, so scrolling
up shows the latest run only. Colours, and clearing the screen, are left out
when the output is not a terminal. --no-color, or NO_COLOR set to anything,
leaves out the colours.

--timeout stops a run that takes longer than it, in any mode, and says so.

A command that does not end by itself, like a server, needs --restart. A
change then stops it, with SIGTERM to it and to every process it started,
and runs it again. It is killed if it has not ended 5 seconds later. Diff
mode, --timeout and the s key stop a command the same way. The command runs
in a process group of its own, so it cannot read from the terminal.

On a terminal, keys steer the watch: space runs the command now, s stops the
run going on, p pauses the watch and goes on with it, b makes diff mode
compare the next run with the last one shown, and q quits.

The command is run as given, not through a shell. For a pipe, give --shell
and the command as one argument. It runs with sh -c, the watched files in
"$@", and a file the script names, like report.awk here, is left out of them
as it is of {}:

    --shell -- 'awk -f report.awk "$@" | sort'`,
		Example: `  wtr --watch file1.txt file2.txt -- awk '{ print $1 }' {}
  wtr -w report.awk -w 'data/*.csv' -m diff -- awk -f report.awk {}
  wtr -r '^logs/.*\.log$' -m append -s -- 'awk "/ERROR/" "$@" | sort | uniq -c'
  wtr -r '\.go$' -x vendor --restart -- go run .`,
		Version:      version(),
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dash := cmd.ArgsLenAtDash()
			if len(args) == 0 && dash < 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dash < 0 || dash == len(args) {
				return errors.New("give the command to run after --")
			}
			watched, err := patterns.patterns(args[:dash])
			if err != nil {
				return err
			}
			argv := args[dash:]
			if shell {
				if len(argv) != 1 {
					return errors.New(`with --shell, give the command as one argument after --, quoted, with "$@" where the files go`)
				}
				argv = []string{"sh", "-c", argv[0], "sh", "{}"}
			}
			if len(watched) == 0 {
				return errors.New("nothing to watch: give --watch or --regex")
			}
			excludeRes, err := compile("--exclude-regex", excludeRegex)
			if err != nil {
				return err
			}
			for _, glob := range exclude {
				if !doublestar.ValidatePathPattern(glob) {
					return fmt.Errorf("--exclude %q: %w", glob, doublestar.ErrBadPattern)
				}
			}
			m, err := internal.ParseMode(mode)
			if err != nil {
				return fmt.Errorf("--mode is one of %v, not %q", internal.ModeValues(), mode)
			}
			b, err := internal.ParseBaseline(baseline)
			if err != nil {
				return fmt.Errorf("--baseline is one of %v, not %q", internal.BaselineValues(), baseline)
			}
			if interval <= 0 {
				return errors.New("--interval must be more than zero")
			}
			if debounce < 0 {
				return errors.New("--debounce cannot be less than zero")
			}
			if contextLines < 0 {
				return errors.New("--context cannot be less than zero")
			}
			if timeout < 0 {
				return errors.New("--timeout cannot be less than zero")
			}
			flags := cmd.Flags()
			if m != internal.ModeDiff && (flags.Changed("baseline") || flags.Changed("context")) {
				return errors.New("--baseline and --context work only with --mode diff")
			}
			if restart && m == internal.ModeDiff {
				return errors.New("--restart does not work with --mode diff, which shows the output only once the command ends")
			}
			if restart && timeout > 0 {
				return errors.New("--timeout does not work with --restart, whose command runs until a change stops it")
			}
			if _, err := exec.LookPath(argv[0]); err != nil {
				return err
			}

			watchOpts := []internal.WatcherOption{
				internal.WithExcludes(exclude, excludeRes),
				internal.WithDebounce(debounce),
			}
			if hidden {
				watchOpts = append(watchOpts, internal.WithHidden())
			}
			if postpone {
				watchOpts = append(watchOpts, internal.WithPostpone())
			}
			// Diff mode stops a run a change makes stale, as --restart does.
			if restart || m == internal.ModeDiff {
				watchOpts = append(watchOpts, internal.WithRestart())
			}
			out := cmd.OutOrStdout()
			runOpts := []internal.RunnerOption{internal.WithBaseline(b)}
			if flags.Changed("context") {
				runOpts = append(runOpts, internal.WithContextLines(contextLines))
			}
			if mergeStderr {
				runOpts = append(runOpts, internal.WithMergedStderr())
			}
			if timeout > 0 {
				runOpts = append(runOpts, internal.WithTimeout(timeout))
			}
			if ringBell {
				runOpts = append(runOpts, internal.WithBell())
			}
			if !isTerminal(out) {
				runOpts = append(runOpts, internal.WithoutClear())
			}
			if !isTerminal(out) || noColor || os.Getenv("NO_COLOR") != "" {
				runOpts = append(runOpts, internal.WithoutColor())
			}

			r := internal.NewRunner(argv, m, out, cmd.ErrOrStderr(), runOpts...)
			if list {
				return printList(out, internal.NewWatcher(interval, watched, watchOpts...), r)
			}
			if isTerminal(out) {
				if kb, err := internal.OpenKeyboard(); err == nil {
					defer kb.Close()
					keys := make(chan internal.Key)
					go readKeys(kb, keys, r)
					watchOpts = append(watchOpts, internal.WithKeys(keys))
				}
			}
			w := internal.NewWatcher(interval, watched, watchOpts...)
			return w.Run(cmd.Context(), r.Run)
		},
	}
	patterns.flags = cmd.Flags()
	cmd.Flags().VarP(patterns.value(false), "watch", "w", "a file or glob to watch; repeat it, or list more before --")
	cmd.Flags().VarP(patterns.value(true), "regex", "r", "a regular expression for the paths of files to watch; repeat it for more")
	cmd.Flags().StringVarP(&mode, "mode", "m", string(internal.ModeClear), "what to do with the previous output: clear, append or diff")
	cmd.Flags().DurationVar(&interval, "interval", 300*time.Millisecond, "how often to look for changes")
	cmd.Flags().StringArrayVarP(&exclude, "exclude", "x", nil, "a file, directory or glob not to watch; repeat it for more")
	cmd.Flags().StringArrayVar(&excludeRegex, "exclude-regex", nil, "a regular expression for the paths not to watch; repeat it for more")
	cmd.Flags().BoolVar(&hidden, "hidden", false, "watch hidden files and directories too")
	cmd.Flags().BoolVar(&list, "list", false, "print the files watched and the command they make, then stop")
	cmd.Flags().BoolVar(&postpone, "postpone", false, "wait for the first change before running the command")
	cmd.Flags().BoolVar(&restart, "restart", false, "stop the command when a change comes while it is still running, and start it again")
	cmd.Flags().DurationVar(&debounce, "debounce", 0, "how long the files must stay unchanged before a run")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "leave out the colours, as NO_COLOR does")
	cmd.Flags().StringVar(&baseline, "baseline", string(internal.BaselinePrevious), "the run diff mode compares with: previous or first")
	cmd.Flags().IntVar(&contextLines, "context", 0, "in diff mode, show only the lines that changed and this many lines around each")
	cmd.Flags().BoolVar(&mergeStderr, "merge-stderr", false, "send the command's stderr where its stdout goes, so diff mode compares it too")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "stop a run that takes longer than this")
	cmd.Flags().BoolVarP(&shell, "shell", "s", false, `run the command, one argument after --, with sh -c, the watched files in "$@"`)
	cmd.Flags().BoolVar(&ringBell, "bell", false, "ring the terminal bell when a run fails or times out")
	_ = cmd.RegisterFlagCompletionFunc("mode", completeWatcherooMode)
	_ = cmd.RegisterFlagCompletionFunc("baseline", completeWatcherooBaseline)
	return cmd
}

// readKeys sends the Key each key pressed on kb stands for to keys, until kb
// is closed: space runs the command now, s stops the run going on, p pauses
// the watch or goes on with it, and q quits. b makes r compare the next run
// with the last one it showed.
func readKeys(kb *internal.Keyboard, keys chan<- internal.Key, r *internal.Runner) {
	paused := false
	for {
		key, err := kb.ReadKey()
		if err != nil {
			return
		}
		switch key {
		case ' ':
			keys <- internal.KeyRun
		case 's':
			keys <- internal.KeyStop
		case 'p':
			keys <- internal.KeyPause
			paused = !paused
			if paused {
				r.Note("paused: changes wait until p is pressed again")
			} else {
				r.Note("watching again")
			}
		case 'q':
			keys <- internal.KeyQuit
		case 'b':
			r.NewBaseline()
		}
	}
}

// version returns the version of the module wtr was built from, which go
// install, and go build in a git checkout, write into the binary.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// completeWatcherooBaseline completes --baseline on watcheroo.
//
// The directive is the second half of the answer: NoFileComp stops the
// shell offering file names on top of the words returned, which is what
// a list of words almost always wants.
func completeWatcherooBaseline(
	cmd *cobra.Command,
	args []string,
	toComplete string,
) ([]string, cobra.ShellCompDirective) {
	return words(internal.BaselineValues()), cobra.ShellCompDirectiveNoFileComp
}

// completeWatcherooMode completes --mode on watcheroo.
//
// The directive is the second half of the answer: NoFileComp stops the
// shell offering file names on top of the words returned, which is what
// a list of words almost always wants.
func completeWatcherooMode(
	cmd *cobra.Command,
	args []string,
	toComplete string,
) ([]string, cobra.ShellCompDirective) {
	return words(internal.ModeValues()), cobra.ShellCompDirectiveNoFileComp
}

// words spells each of values as its String does.
func words[T fmt.Stringer](values []T) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, v.String())
	}
	return out
}

// compile compiles every one of exprs, the values of flag.
func compile(flag string, exprs []string) ([]*regexp.Regexp, error) {
	res := make([]*regexp.Regexp, 0, len(exprs))
	for _, expr := range exprs {
		re, err := regexp.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", flag, err)
		}
		res = append(res, re)
	}
	return res, nil
}

// isTerminal reports whether w is a terminal, the only place where colours
// and clearing the screen make sense.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// printList prints the files w watches now, the patterns that match none,
// and the command r runs for the files.
func printList(out io.Writer, w internal.Watcher, r *internal.Runner) error {
	files, err := w.Files()
	if err != nil {
		return err
	}
	unmatched, err := w.Unmatched()
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "watching:")
	for _, file := range files {
		fmt.Fprintf(out, "  %s\n", file)
	}
	if len(files) == 0 {
		fmt.Fprintln(out, "  (nothing matches yet)")
	}
	if len(unmatched) > 0 {
		fmt.Fprintln(out, "matching nothing yet:")
		for _, p := range unmatched {
			fmt.Fprintf(out, "  %s\n", p)
		}
	}
	fmt.Fprintf(out, "running:\n  %s\n", internal.JoinCommand(r.Command(files)))
	return nil
}

// Execute runs the root command. main calls it, and nothing else should.
func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := NewRootCmd().ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
