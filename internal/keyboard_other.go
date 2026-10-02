//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package internal

import "errors"

// Keyboard reads the keys pressed on the terminal, where wtr knows how to.
type Keyboard struct{}

// OpenKeyboard fails: wtr does not know how to read keys here.
func OpenKeyboard() (*Keyboard, error) {
	return nil, errors.New("no keyboard here")
}

// ReadKey is never called, as OpenKeyboard always fails.
func (k *Keyboard) ReadKey() (byte, error) {
	return 0, errors.New("no keyboard here")
}

// Close does nothing.
func (k *Keyboard) Close() error {
	return nil
}
