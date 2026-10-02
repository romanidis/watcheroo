package terminal

import (
	"io"

	"github.com/romanidis/watcheroo/internal/watch"
)

var _ watch.Requests = (*Keys)(nil)

// bindings are the keys a watch answers to, and what each asks for. The
// words Screen writes name these keys.
var bindings = map[byte]watch.Request{
	' ': watch.RequestRun,
	's': watch.RequestStop,
	'p': watch.RequestPause,
	'b': watch.RequestRebase,
	'q': watch.RequestQuit,
}

// Keys hands over what the person watching asks for with the keys they
// press, when wtr writes to a terminal. Keys are read from the terminal
// itself, not from stdin, which the command may be given.
type Keys struct {
	stdout io.Writer
}

// NewKeys returns Keys for a watch that writes to stdout.
func NewKeys(stdout io.Writer) *Keys {
	return &Keys{stdout: stdout}
}

// Listen sets the terminal up to hand over each key as it is pressed, and
// reads them until the func it returns is called, which puts the terminal
// back as it was. Where stdout is not a terminal, or the terminal cannot be
// set up, nothing is read, and the channel is nil.
func (k *Keys) Listen() (<-chan watch.Request, func()) {
	if !IsTerminal(k.stdout) {
		return nil, func() {}
	}
	t, err := openTTY()
	if err != nil {
		return nil, func() {}
	}
	requests := make(chan watch.Request)
	done := make(chan struct{})
	go func() {
		for {
			key, err := t.readKey()
			if err != nil {
				return
			}
			req, ok := bindings[key]
			if !ok {
				continue
			}
			select {
			case requests <- req:
			case <-done:
				return
			}
		}
	}()
	return requests, func() {
		close(done)
		t.close()
	}
}
