## watcheroo

Run a command again whenever the files it reads change

### Synopsis

Run a command, then run it again each time one of the watched files is
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

    -- sh -c 'awk -f report.awk "$@" | sort' sh {}

```
watcheroo [--watch] GLOB... [--regex REGEX]... -- COMMAND [ARG...] [flags]
```

### Examples

```
  watcheroo --watch file1.txt file2.txt -- awk '{ print $1 }' {}
  watcheroo -w report.awk -w 'data/*.csv' -m diff -- awk -f report.awk {}
  watcheroo -r '^logs/.*\.log$' -m append -- sh -c 'awk "/ERROR/" "$@" | sort | uniq -c' sh {}
  watcheroo -r '\.go$' -x vendor --restart -- go run .
```

### Options

```
      --baseline string             the run diff mode compares with: previous or first (default "previous")
      --context int                 in diff mode, show only the lines that changed and this many lines around each
      --debounce duration           how long the files must stay unchanged before a run
  -x, --exclude stringArray         a file, directory or glob not to watch; repeat it for more
      --exclude-regex stringArray   a regular expression for the paths not to watch; repeat it for more
  -h, --help                        help for watcheroo
      --hidden                      watch hidden files and directories too
      --interval duration           how often to look for changes (default 300ms)
      --list                        print the files watched and the command they make, then stop
      --merge-stderr                send the command's stderr where its stdout goes, so diff mode compares it too
  -m, --mode string                 what to do with the previous output: clear, append or diff (default "clear")
      --no-color                    leave out the colours, as NO_COLOR does
      --postpone                    wait for the first change before running the command
  -r, --regex stringArray           a regular expression for the paths of files to watch; repeat it for more
      --restart                     stop the command when a change comes while it is still running, and start it again
  -w, --watch stringArray           a file or glob to watch; repeat it, or list more before --
```

