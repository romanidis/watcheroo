# watch

The watching feature: run a command, then run it again each time the files it
reads change, and show each run. The domain decides when a run is due, what the
command is for the files, and how the output differs from the run before; this
package looks at the files, runs the command and shows it, through its ports.

## Ports
- `Scanner` looks at the files a watchlist names (`disk`).
- `ProgramFinder` and `Executor` find and run the command (`process`).
- `Display` shows each run, and `Requests` hands over the keys pressed (`terminal`).
- `Clock` tells the time, for the header and the debounce.

## Usecases
Each usecase is one operation (`Command`/`Query` → `Handle` → `Result`) and is
injected as a `Handler[C, R]` alias (see `handlers.go`).
- `WatchFilesUsecase` runs the watch until it is told to stop: it looks on every
  tick, asks the domain's `Watch` whether a run is due, keeps one run going at a
  time in the background, and in diff mode compares each run through the
  domain's `Comparison`.
- `ListWatchedUsecase` looks once, and returns the files, the patterns that match
  none, and the command a run would make, for `--list`.
