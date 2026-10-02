// Package cmd is the command line, and where wtr is put together: it reads
// the flags and the arguments, says what is wrong with how they were given,
// plugs the adapters into the watch use cases, and hands those the rest as
// it was given, for them to make sense of. It knows flags, cobra and the help
// text, and nothing of how a watch works.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/romanidis/watcheroo/internal/cli"
	"github.com/romanidis/watcheroo/internal/disk"
	"github.com/romanidis/watcheroo/internal/domain"
	"github.com/romanidis/watcheroo/internal/process"
	"github.com/romanidis/watcheroo/internal/terminal"
	"github.com/romanidis/watcheroo/internal/watch"
	"github.com/spf13/cobra"
)

// Execute runs the wtr command, until it ends or wtr is interrupted. main
// calls it, and nothing else should.
func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := NewRootCmd().ExecuteContext(ctx)
	stop()
	if err != nil {
		os.Exit(1)
	}
}

// NewRootCmd builds the wtr command.
func NewRootCmd() *cobra.Command {
	var (
		patterns     *cli.PatternList // set once cmd, whose flags it counts, is built
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
			flags := cmd.Flags()
			if mode != string(domain.ModeDiff) && (flags.Changed("baseline") || flags.Changed("context")) {
				return errors.New("--baseline and --context work only with --mode diff")
			}
			if contextLines < 0 {
				return errors.New("--context cannot be less than zero")
			}
			if !flags.Changed("context") {
				contextLines = -1 // every line
			}

			watchFiles, listWatched := wire(cmd.OutOrStdout(), cmd.ErrOrStderr(), noColor || os.Getenv("NO_COLOR") != "", ringBell)
			watchlist := watch.WatchlistInput{
				Patterns:     patterns.Inputs(args[:dash]),
				Exclude:      exclude,
				ExcludeRegex: excludeRegex,
				Hidden:       hidden,
			}
			command := watch.CommandInput{Argv: args[dash:], Shell: shell}
			if list {
				res, err := listWatched.Handle(cmd.Context(), watch.ListWatchedQuery{Watchlist: watchlist, Command: command})
				if err != nil {
					return err
				}
				printList(cmd.OutOrStdout(), res)
				return nil
			}
			_, err := watchFiles.Handle(cmd.Context(), watch.WatchFilesCommand{
				Watchlist:   watchlist,
				Command:     command,
				Mode:        mode,
				Baseline:    baseline,
				Context:     contextLines,
				MergeStderr: mergeStderr,
				Interval:    interval,
				Debounce:    debounce,
				Timeout:     timeout,
				Restart:     restart,
				Postpone:    postpone,
			})
			return err
		},
	}
	patterns = cli.NewPatternList(cmd.Flags())
	cmd.Flags().VarP(patterns.Value(false), "watch", "w", "a file or glob to watch; repeat it, or list more before --")
	cmd.Flags().VarP(patterns.Value(true), "regex", "r", "a regular expression for the paths of files to watch; repeat it for more")
	cmd.Flags().StringVarP(&mode, "mode", "m", string(domain.ModeClear), "what to do with the previous output: clear, append or diff")
	cmd.Flags().DurationVar(&interval, "interval", 300*time.Millisecond, "how often to look for changes")
	cmd.Flags().StringArrayVarP(&exclude, "exclude", "x", nil, "a file, directory or glob not to watch; repeat it for more")
	cmd.Flags().StringArrayVar(&excludeRegex, "exclude-regex", nil, "a regular expression for the paths not to watch; repeat it for more")
	cmd.Flags().BoolVar(&hidden, "hidden", false, "watch hidden files and directories too")
	cmd.Flags().BoolVar(&list, "list", false, "print the files watched and the command they make, then stop")
	cmd.Flags().BoolVar(&postpone, "postpone", false, "wait for the first change before running the command")
	cmd.Flags().BoolVar(&restart, "restart", false, "stop the command when a change comes while it is still running, and start it again")
	cmd.Flags().DurationVar(&debounce, "debounce", 0, "how long the files must stay unchanged before a run")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "leave out the colours, as NO_COLOR does")
	cmd.Flags().StringVar(&baseline, "baseline", string(domain.BaselinePrevious), "the run diff mode compares with: previous or first")
	cmd.Flags().IntVar(&contextLines, "context", 0, "in diff mode, show only the lines that changed and this many lines around each")
	cmd.Flags().BoolVar(&mergeStderr, "merge-stderr", false, "send the command's stderr where its stdout goes, so diff mode compares it too")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "stop a run that takes longer than this")
	cmd.Flags().BoolVarP(&shell, "shell", "s", false, `run the command, one argument after --, with sh -c, the watched files in "$@"`)
	cmd.Flags().BoolVar(&ringBell, "bell", false, "ring the terminal bell when a run fails or times out")
	_ = cmd.RegisterFlagCompletionFunc("mode", completeMode)
	_ = cmd.RegisterFlagCompletionFunc("baseline", completeBaseline)
	return cmd
}

// wire plugs the adapters into the use cases, showing things on stdout and
// stderr. It is the one place the concrete use cases are named; the rest of
// the command line knows them by their handler shapes.
func wire(stdout, stderr io.Writer, noColor, bell bool) (watch.WatchFilesHandler, watch.ListWatchedHandler) {
	files := disk.Scanner{}
	commands := process.Runner{}
	screen := terminal.NewScreen(stdout, stderr, terminal.OptionsFor(stdout, noColor, bell))
	return watch.NewWatchFilesUsecase(files, commands, commands, screen, terminal.NewKeys(stdout), wallClock{}),
		watch.NewListWatchedUsecase(files, commands)
}

var _ watch.Clock = wallClock{}

// wallClock is the time of day.
type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

// version returns the version of the module wtr was built from, which go
// install, and go build in a git checkout, write into the binary.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// completeBaseline completes --baseline.
//
// The directive is the second half of the answer: NoFileComp stops the
// shell offering file names on top of the words returned, which is what
// a list of words almost always wants.
func completeBaseline(
	cmd *cobra.Command,
	args []string,
	toComplete string,
) ([]string, cobra.ShellCompDirective) {
	return words(domain.BaselineValues()), cobra.ShellCompDirectiveNoFileComp
}

// completeMode completes --mode, the same way.
func completeMode(
	cmd *cobra.Command,
	args []string,
	toComplete string,
) ([]string, cobra.ShellCompDirective) {
	return words(domain.ModeValues()), cobra.ShellCompDirectiveNoFileComp
}

// words spells each of values as its String does.
func words[T fmt.Stringer](values []T) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, v.String())
	}
	return out
}

// printList prints what --list found: the files watched, the patterns that
// match none, and the command a run would make.
func printList(out io.Writer, res watch.ListWatchedResult) {
	fmt.Fprintln(out, "watching:")
	for _, file := range res.Files {
		fmt.Fprintf(out, "  %s\n", file)
	}
	if len(res.Files) == 0 {
		fmt.Fprintln(out, "  (nothing matches yet)")
	}
	if len(res.Unmatched) > 0 {
		fmt.Fprintln(out, "matching nothing yet:")
		for _, p := range res.Unmatched {
			fmt.Fprintf(out, "  %s\n", p)
		}
	}
	fmt.Fprintf(out, "running:\n  %s\n", res.Command)
}
