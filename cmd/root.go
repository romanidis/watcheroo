package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/romanidis/watcheroo/internal"
	"github.com/spf13/cobra"
)

// NewRootCmd builds the watcheroo command.
func NewRootCmd() *cobra.Command {
	var (
		watch        []string
		regexes      []string
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
	)
	cmd := &cobra.Command{
		Use:   "watcheroo [--watch] GLOB... [--regex REGEX]... -- COMMAND [ARG...]",
		Short: "Run a command again whenever the files it reads change",
		Long: `Run a command, then run it again each time one of the watched files is
created, deleted or changed.

Everything before -- that is not a flag is another file to watch, so
--watch a.txt b.txt watches both. A name can be a glob, which watcheroo
expands again on every poll, so a file created later is picked up too. Quote
it so the shell does not expand it first. A directory stands for every file
under it.

A --regex is matched against the path of every file under the working
directory, anywhere in the path unless it is anchored with ^. Anchor it with
a directory and only that directory is searched:

    --regex '^data/.*\.csv$'

Hidden files and directories, whose names start with a dot, are not watched
unless a glob names them with the dot, like .env, or --hidden is given. That
keeps editor swap files and .git out.

--exclude leaves out a file or a directory and everything under it. A glob
with no slash in it is matched against each name along the path, so
--exclude node_modules leaves out every directory of that name; one with a
slash is matched against the whole path. --exclude-regex is matched against
the path as --regex is. --list prints the files watched and the command they
make, then stops, which shows what a pattern matches.

An argument that is exactly {} becomes the watched files, sorted, one
argument each. A file the command already names as an argument of its own,
like the script in awk -f report.awk, is watched but left out of {}.

--mode says what happens to the previous run's output:

    clear    clear the screen first, so only the latest output shows (default)
    append   keep it, and print the next run below it
    diff     clear the screen first, and mark what changed since the last run

In diff mode, --baseline first compares every run with the first rather than
with the one before it, and --context N shows only the lines that changed and
N lines around each. --merge-stderr sends the command's stderr where its
stdout goes, so diff mode compares it too.

Colours, and clearing the screen, are left out when the output is not a
terminal. --no-color, or NO_COLOR set to anything, leaves out the colours.

A command that does not end by itself, like a server, needs --restart. A
change then stops it, with SIGTERM to it and to every process it started, and
runs it again. It is killed if it has not ended 5 seconds later.

The command is run as given, not through a shell. For a pipe, run a shell
and pass it the files as its arguments:

    -- sh -c 'awk -f report.awk "$@" | sort' sh {}`,
		Example: `  watcheroo --watch file1.txt file2.txt -- awk '{ print $1 }' {}
  watcheroo -w report.awk -w 'data/*.csv' -m diff -- awk -f report.awk {}
  watcheroo -r '^logs/.*\.log$' -m append -- sh -c 'awk "/ERROR/" "$@" | sort | uniq -c' sh {}
  watcheroo -r '\.go$' -x vendor --restart -- go run .`,
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dash := cmd.ArgsLenAtDash()
			if dash < 0 || dash == len(args) {
				return errors.New("give the command to run after --")
			}
			globs, argv := slices.Concat(watch, args[:dash]), args[dash:]
			if len(globs) == 0 && len(regexes) == 0 {
				return errors.New("nothing to watch: give --watch or --regex")
			}
			res, err := compile("--regex", regexes)
			if err != nil {
				return err
			}
			excludeRes, err := compile("--exclude-regex", excludeRegex)
			if err != nil {
				return err
			}
			for _, glob := range exclude {
				if _, err := filepath.Match(glob, ""); err != nil {
					return fmt.Errorf("--exclude %q: %w", glob, err)
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
			flags := cmd.Flags()
			if m != internal.ModeDiff && (flags.Changed("baseline") || flags.Changed("context")) {
				return errors.New("--baseline and --context work only with --mode diff")
			}
			if restart && m == internal.ModeDiff {
				return errors.New("--restart does not work with --mode diff, which shows the output only once the command ends")
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
			if restart {
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
			if restart {
				runOpts = append(runOpts, internal.WithProcessGroup())
			}
			if !isTerminal(out) {
				runOpts = append(runOpts, internal.WithoutClear())
			}
			if !isTerminal(out) || noColor || os.Getenv("NO_COLOR") != "" {
				runOpts = append(runOpts, internal.WithoutColor())
			}

			w := internal.NewWatcher(interval, globs, res, watchOpts...)
			r := internal.NewRunner(argv, m, out, cmd.ErrOrStderr(), runOpts...)
			if list {
				return printList(out, w, r)
			}
			return w.Run(cmd.Context(), r.Run)
		},
	}
	cmd.Flags().StringArrayVarP(&watch, "watch", "w", nil, "a file or glob to watch; repeat it, or list more before --")
	cmd.Flags().StringArrayVarP(&regexes, "regex", "r", nil, "a regular expression for the paths of files to watch; repeat it for more")
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
	_ = cmd.RegisterFlagCompletionFunc("mode", completeWatcherooMode)
	_ = cmd.RegisterFlagCompletionFunc("baseline", completeWatcherooBaseline)
	return cmd
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

// printList prints the files w watches now, and the command r runs for them.
func printList(out io.Writer, w internal.Watcher, r *internal.Runner) error {
	files, err := w.Files()
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
	fmt.Fprintf(out, "running:\n  %s\n", strings.Join(r.Command(files), " "))
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
