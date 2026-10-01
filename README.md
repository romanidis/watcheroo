# watcheroo

watcheroo runs a command, then runs it again every time one of the files you're
watching changes.

It was made for prototyping and scripting in awk. An awk program usually grows
by trial: change a pattern, run it on the data, read what came out, change it
again. watcheroo takes the rerunning out of that loop. Keep the script open in
your editor and watcheroo in a terminal beside it, and every save shows the new
output. In diff mode it also marks what changed since the last run, down to the
one number in a wide line that moved.

```sh
watcheroo -w report.awk -w 'data/*.csv' -m diff -- awk -f report.awk {}
```

Several of its choices come from that use:

- `{}` passes the data files to awk and leaves out the script that `awk -f`
  already names. Editing the script reruns it, without awk also reading the
  script as input.
- Wildcards are expanded again on every check, so a new CSV dropped into
  `data/` is part of the next run.
- Diff mode highlights the words that changed, not just the lines, because
  awk's output is usually columns of numbers. `--baseline first` shows how far
  your edits have moved the result since you started.
- The command runs directly, not through a shell. When a prototype grows into
  a pipeline, `sh -c` takes the files as `"$@"`.

It works with any command, whether a Python script, `psql -f query.sql` or
`go run .`, but awk is what it was built around.

Every flag is listed in [docs/watcheroo.md](docs/watcheroo.md), which is
generated from `watcheroo --help` by `task docs`.

## Install

```sh
go install github.com/romanidis/watcheroo@latest
```

Or, from a clone:

```sh
task install   # go install . — puts watcheroo in $GOBIN, or ~/go/bin
```

For tab completion of `--mode` and `--baseline` values, load the script
`watcheroo completion zsh` prints (or `bash`, `fish`, `powershell`), e.g. in
`~/.zshrc`:

```sh
source <(watcheroo completion zsh)
```

## What to watch

Everything before `--` says what to watch. The command to run comes after it.

- `-w`, `--watch`: a file, a wildcard or a directory. Repeat the flag, or list
  more names after it: `--watch a.txt b.txt`.
  - Quote wildcards (`'data/*.csv'`) so watcheroo expands them itself on every
    check. That way a file created later is picked up too.
  - A directory means every file under it, in subdirectories as well.
- `-r`, `--regex`: a regular expression matched against the path of every file
  under the current directory. Unless you anchor it with `^`, it can match
  anywhere in the path.
  - In a large tree, anchor it with a directory (`'^data/.*\.csv$'`) so only
    that directory is searched.
- Hidden files and directories (names starting with `.`) are skipped unless a
  pattern includes the dot itself, as in `.env` or `'.*.conf'`, or you give
  `--hidden`. That keeps editor swap files and `.git` out.
- `-x`, `--exclude`: a file, directory or wildcard not to watch, along with
  everything under it. Repeat it for more.
  - Without a `/` it's matched against every name in the path, so
    `-x node_modules` skips that directory wherever it is, and `-x '*.tmp'`
    skips every `.tmp` file.
  - With a `/` it's matched against the whole path: `-x 'out/*.csv'`.
  - Exclude whatever the command writes into a watched directory. Otherwise
    each run changes a watched file, and the command reruns forever.
- `--exclude-regex`: a regular expression for paths not to watch, matched the
  way `--regex` is.
- `--list` prints the files that are watched and the command they make, then
  stops. Use it to check what a wildcard or regex matches:

  ```
  $ watcheroo --list -w report.awk -w '*.csv' -x out.csv -- awk -f report.awk {}
  watching:
    a.csv
    b.csv
    report.awk
  running:
    awk -f report.awk a.csv b.csv
  ```

watcheroo checks for changes every 300ms. Change that with `--interval`.
`--debounce 500ms` waits until the files have stopped changing for that long
before running, so a file saved in several writes is read once, when it's
whole. `--postpone` skips the run at the start and waits for the first change.

## `{}`

An argument that is exactly `{}` is replaced by the watched files, sorted, one
argument each. Any watched file the command already names as a separate
argument, such as the script in `awk -f report.awk`, is watched but left out of
`{}`. That way, editing the script reruns the command without awk also reading
the script as input.

The command runs directly, not through a shell. For pipes, run `sh -c` and pass
the files to it as arguments:

```sh
watcheroo -r '^logs/.*\.log$' -- sh -c 'awk "/ERROR/" "$@" | sort | uniq -c' sh {}
```

## Modes

`-m`, `--mode` sets what happens to the previous run's output:

| mode | what you see |
| --- | --- |
| `clear` (default) | the screen is cleared before each run, so only the latest output shows |
| `append` | earlier output stays, with a timestamp line before each run |
| `diff` | the screen is cleared and the whole output is shown, with what changed since the previous run marked |

A command that fails doesn't stop the watch. Its exit status is printed in red,
and the next change runs it again. Stop watching with Ctrl-C.

Colours are left out when you give `--no-color` or set `NO_COLOR`. When the
output isn't a terminal, as in `watcheroo … | tee log`, the screen isn't
cleared either, so the log holds just the text.

## Commands that don't end

A command that keeps running, like a server, would hold up the watch: the next
run waits for the previous one to end. Give `--restart`, and a change stops the
running command and starts it again:

```sh
watcheroo -r '\.go$' -x vendor --restart -- go run .
```

The command is stopped with SIGTERM, sent to it and to every process it
started, so the server `go run` builds stops too and doesn't keep holding its
port. If it hasn't ended 5 seconds later, it's killed. `--restart` doesn't work
with diff mode, which can only show the output once the command has ended.

## Diff mode, step by step

Diff mode shows the whole output on every run, like `clear`, and marks what
changed since the run before:

- two spaces before a line that didn't change
- `-` and red for a line that went away
- `+` and green for a line that is new

When a line changed rather than disappeared, the old version (`-`) and the new
one (`+`) appear one after the other, and only the words that changed are
highlighted. That makes a single changed number easy to spot in a wide line. In
the examples below, `[brackets]` stand for that highlight.

With `sales.csv` and this `report.awk`:

```awk
BEGIN { FS = "," }
{ total[$1] += $2; region[$1] = $3 }
END { for (name in total) printf "%-6s %4d  %s\n", name, total[name], region[name] | "sort" }
```

```sh
watcheroo -w report.awk -w '*.csv' -m diff -- awk -f report.awk {}
```

The first run has nothing to compare with, so nothing is marked:

```
23:33:28  awk -f report.awk sales.csv
  alice    10  north
  bob      20  south
```

Change bob's line in `sales.csv` from `bob,20,south` to `bob,35,east`:

```diff
23:33:29  awk -f report.awk sales.csv
  alice    10  north
- bob      [20]  [south]
+ bob      [35]  [east]
```

Add `carol,7,west` to `sales.csv`:

```diff
23:33:30  awk -f report.awk sales.csv
  alice    10  north
  bob      35  east
+ carol     7  west
```

Create `march.csv` with `alice,4,north` in it. The `*.csv` wildcard picks it
up, `{}` passes it to awk, and alice's total changes:

```diff
23:33:30  awk -f report.awk march.csv sales.csv
- alice    [10]  north
+ alice    [14]  north
  bob      35  east
  carol     7  west
```

Edit the script without changing what it prints, such as adding a comment:

```
23:33:31  awk -f report.awk march.csv sales.csv
(output unchanged)
  alice    14  north
  bob      35  east
  carol     7  west
```

Each run is compared with the run just before it. Three flags change what
diff mode shows:

- `--baseline first` compares every run with the first run instead, which
  shows how far the output has moved overall. A run whose output is back to
  the first one's says `(output as in the first run)`.
- `--context N` shows only the lines that changed and `N` lines around each.
  The lines it leaves out are counted, as in `(12 unchanged lines)`. The first
  run is still shown whole.
- `--merge-stderr` sends the command's stderr where its stdout goes, so error
  lines are diffed in place with the rest of the output. Without it they are
  printed straight away, above the output.

## Development

```sh
task build    # bin/watcheroo
task test     # go vet, then the tests with -race
task docs     # the command reference into docs/
```
