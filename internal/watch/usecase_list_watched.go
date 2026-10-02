/*
USECASE List Watched

Looks once at the files a watch would look at, and spells the command a run
would make for them, without running anything: how to check what a pattern
matches, and which file {} puts where.

Authz posture: none. wtr runs as whoever started it, on their own files and
their own command; there is no one else to check for.
*/
package watch

import (
	"context"

	"github.com/romanidis/watcheroo/internal/domain"
)

// ListWatchedQuery is what a watch would watch and run.
type ListWatchedQuery struct {
	Watchlist WatchlistInput
	Command   CommandInput
}

// ListWatchedResult is the files watched now, in the order {} lists them,
// the patterns that match none, and the command a run would make.
type ListWatchedResult struct {
	Files     []string
	Unmatched []domain.Pattern
	Command   domain.Argv
}

type ListWatchedUsecase struct {
	scanner Scanner
	finder  ProgramFinder
}

func NewListWatchedUsecase(scanner Scanner, finder ProgramFinder) *ListWatchedUsecase {
	return &ListWatchedUsecase{scanner: scanner, finder: finder}
}

func (uc *ListWatchedUsecase) Handle(ctx context.Context, q ListWatchedQuery) (ListWatchedResult, error) {
	list, err := newWatchlist(q.Watchlist)
	if err != nil {
		return ListWatchedResult{}, err
	}
	command, err := domain.NewCommand(q.Command.Argv, q.Command.Shell)
	if err != nil {
		return ListWatchedResult{}, err
	}
	if err := uc.finder.FindProgram(command.Program()); err != nil {
		return ListWatchedResult{}, err
	}
	now, err := look(uc.scanner, list)
	if err != nil {
		return ListWatchedResult{}, err
	}
	files := now.Files()
	return ListWatchedResult{Files: files, Unmatched: now.Unmatched(), Command: command.For(files)}, nil
}
