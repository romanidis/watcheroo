//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package terminal

import "errors"

// tty reads the keys pressed on the terminal, where wtr knows how to.
type tty struct{}

// openTTY fails: wtr does not know how to read keys here.
func openTTY() (*tty, error) {
	return nil, errors.New("no keys here")
}

// readKey is never called, as openTTY always fails.
func (k *tty) readKey() (byte, error) {
	return 0, errors.New("no keys here")
}

// close does nothing.
func (k *tty) close() error {
	return nil
}
