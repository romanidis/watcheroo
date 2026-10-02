package watch

import "context"

// Handler is one use case: it handles C and answers R. Consumers depend on
// the aliases below rather than on the *Usecase structs, so the composition
// root can wrap any use case without them knowing. It lives in this slice,
// not in an app package of its own, while this is the only slice.
type Handler[C, R any] interface {
	Handle(ctx context.Context, c C) (R, error)
}

type (
	WatchFilesHandler  = Handler[WatchFilesCommand, WatchFilesResult]
	ListWatchedHandler = Handler[ListWatchedQuery, ListWatchedResult]
)

// Compile-time proof that every use case satisfies its handler shape.
var (
	_ WatchFilesHandler  = (*WatchFilesUsecase)(nil)
	_ ListWatchedHandler = (*ListWatchedUsecase)(nil)
)
