# watcheroo

Rerun a command every time the files it reads change.

I made this for awk. Prototyping an awk script usually goes like this: tweak
the script, switch to the terminal, up-arrow, enter, then squint at the output
to work out what changed. Then do it all again, a few dozen times. watcheroo
does the switching and the squinting for you. Leave it running next to your
editor, and every time you save the script or the data changes, it reruns and
shows you the new output with the changes highlighted.

```sh
watcheroo -w report.awk -w 'data/*.csv' -m diff -- awk -f report.awk {}
```

Everything before `--` is what to watch, and everything after it is what to
run. `{}` gets replaced with the watched files.

It isn't awk-only. Anything that reads files and prints something works fine.
It's just tuned for the edit, run, look loop that awk scripting is.

## Install

```sh
go install github.com/romanidis/watcheroo@latest
```

For tab completion, put `source <(watcheroo completion zsh)` in your `.zshrc`.
bash and fish work too.

## Examples

Rerun a report whenever the script or any CSV changes, and show what changed:

```sh
watcheroo -w report.awk -w 'data/*.csv' -m diff -- awk -f report.awk {}
```

Count errors across a bunch of logs. watcheroo doesn't run your command through
a shell, so for a pipe you bring your own:

```sh
watcheroo -r '^logs/.*\.log$' -- sh -c 'awk "/ERROR/" "$@" | sort | uniq -c' sh {}
```

Keep a dev server running and restart it whenever the code changes:

```sh
watcheroo -r '\.go$' -x vendor --restart -- go run .
```

Not sure what a pattern matches? Ask it:

```
$ watcheroo --list -w report.awk -w '*.csv' -x out.csv -- awk -f report.awk {}
watching:
  a.csv
  b.csv
  report.awk
running:
  awk -f report.awk a.csv b.csv
```

## Things worth knowing

- **Quote your wildcards.** Write `'data/*.csv'`, not `data/*.csv`. When
  watcheroo gets the wildcard itself, it expands it again on every check, so a
  CSV you add later gets picked up too.
- **`{}` skips the script.** In `awk -f report.awk {}`, `report.awk` is watched,
  but it isn't passed again through `{}`. Otherwise awk would read your script
  as data.
- **Dotfiles are ignored**, so `.git` and editor swap files don't trigger
  reruns. To watch one, name it with the dot, like `.env`, or pass `--hidden`.
- **Don't watch your own output.** If the command writes into a directory
  you're watching, it'll keep triggering itself forever. Exclude the output
  with `-x out` or `-x '*.tmp'`. An exclude without a slash matches any name
  in the path, so `-x node_modules` skips that directory wherever it is.
- **A failing command doesn't stop anything.** You'll see its exit status in
  red, and the next change runs it again. Hit Ctrl-C to quit.
- **`--restart` cleans up after itself.** It sends SIGTERM to the command and to
  everything the command started, so the server that `go run` builds actually
  stops and frees its port. If the command is still running 5 seconds later,
  it gets killed. It doesn't work together with diff mode.
- **Piping the output is fine.** When stdout isn't a terminal, as in
  `| tee log`, watcheroo skips the colours and the screen clearing. To turn off
  colour everywhere, use `--no-color` or set `NO_COLOR`.

## Diff mode

Diff mode is the main reason watcheroo exists. Each run clears the screen,
prints the whole output, and marks what changed since the run before. The old
version of a line gets a `-` and red, and the new version gets a `+` and green.
Within a changed line, only the words that changed are highlighted, so one
number moving in a wide table is easy to spot. Below, `[brackets]` stand in for
that highlight.

Say you have `sales.csv` and this `report.awk`:

```awk
BEGIN { FS = "," }
{ total[$1] += $2; region[$1] = $3 }
END { for (name in total) printf "%-6s %4d  %s\n", name, total[name], region[name] | "sort" }
```

```sh
watcheroo -w report.awk -w '*.csv' -m diff -- awk -f report.awk {}
```

The first run has nothing to compare against:

```
23:33:28  awk -f report.awk sales.csv
  alice    10  north
  bob      20  south
```

Change bob's row from `bob,20,south` to `bob,35,east`:

```diff
23:33:29  awk -f report.awk sales.csv
  alice    10  north
- bob      [20]  [south]
+ bob      [35]  [east]
```

Add `carol,7,west`:

```diff
23:33:30  awk -f report.awk sales.csv
  alice    10  north
  bob      35  east
+ carol     7  west
```

Drop a new `march.csv` in with `alice,4,north`. The wildcard picks it up, `{}`
passes it to awk, and alice's total moves:

```diff
23:33:30  awk -f report.awk march.csv sales.csv
- alice    [10]  north
+ alice    [14]  north
  bob      35  east
  carol     7  west
```

If you edit the script without changing what it prints, it tells you so with
`(output unchanged)`.

A few flags change how this works:

- `--baseline first` compares every run with the first one instead of the
  previous one, which shows how far you've drifted since you started.
- `--context 2` shows only the changed lines and 2 lines around each one. The
  lines it hides are counted, like `(12 unchanged lines)`. Handy when the
  output is longer than your screen.
- `--merge-stderr` diffs stderr along with stdout. Without it, error lines are
  printed above the output as they come.

## All the flags

| flag | what it does |
| --- | --- |
| `-w`, `--watch` | a file, wildcard or directory to watch |
| `-r`, `--regex` | a regex for paths to watch; anchor it like `'^data/'` to keep it fast |
| `-x`, `--exclude` | a file, directory or wildcard to skip |
| `--exclude-regex` | a regex for paths to skip |
| `--hidden` | watch dotfiles too |
| `-m`, `--mode` | `clear` (the default), `append` or `diff` |
| `--baseline` | in diff mode, compare with the `previous` run (the default) or the `first` |
| `--context N` | in diff mode, show only changed lines plus `N` around them |
| `--merge-stderr` | treat stderr as part of the output |
| `--restart` | stop the command and start it again on every change |
| `--debounce 500ms` | wait until the files stop changing before running |
| `--postpone` | don't run until the first change |
| `--interval` | how often to check for changes (default 300ms) |
| `--list` | print what's watched and what would run, then quit |
| `--no-color` | no colours |

The full reference is in [docs/watcheroo.md](docs/watcheroo.md), generated from
`watcheroo --help`.

## Hacking on it

```sh
task build    # bin/watcheroo
task test     # go vet, then the tests with -race
task docs     # regenerate docs/ from --help
```

## License

MIT, see [LICENSE](LICENSE).
