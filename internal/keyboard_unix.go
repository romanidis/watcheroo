//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package internal

import (
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// Keyboard reads the keys pressed on the terminal, each as it is pressed and
// without echoing it. Ctrl-C still interrupts, and output is left as it is,
// so the command's output looks as it would without a Keyboard.
type Keyboard struct {
	tty   *os.File
	saved unix.Termios // the terminal's settings before, which Close puts back
	cont  chan os.Signal

	mu     sync.Mutex
	closed bool
}

// OpenKeyboard opens the terminal wtr runs in, and sets it up to read keys.
// It fails when there is no terminal.
func OpenKeyboard() (*Keyboard, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	k := &Keyboard{tty: tty, cont: make(chan os.Signal, 1)}
	err = k.control(func(fd int) error {
		saved, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
		if err == nil {
			k.saved = *saved
		}
		return err
	})
	if err == nil {
		err = k.cbreak()
	}
	if err != nil {
		tty.Close()
		return nil, err
	}
	// A shell that takes the terminal back after Ctrl-Z may reset it, so set
	// it up again when wtr is brought back.
	signal.Notify(k.cont, syscall.SIGCONT)
	go func() {
		for range k.cont {
			k.cbreak()
		}
	}()
	return k, nil
}

// ReadKey waits for the next key pressed, and returns its first byte.
func (k *Keyboard) ReadKey() (byte, error) {
	var b [1]byte
	if _, err := k.tty.Read(b[:]); err != nil {
		return 0, err
	}
	return b[0], nil
}

// Close puts the terminal back as it was.
func (k *Keyboard) Close() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.closed {
		return nil
	}
	k.closed = true
	signal.Stop(k.cont)
	close(k.cont)
	err := k.control(func(fd int) error {
		return unix.IoctlSetTermios(fd, ioctlWriteTermios, &k.saved)
	})
	return errors.Join(err, k.tty.Close())
}

// cbreak makes the terminal hand over each key as it is pressed, without
// echoing it, unless Close has put it back already.
func (k *Keyboard) cbreak() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.closed {
		return nil
	}
	t := k.saved
	t.Lflag &^= unix.ICANON | unix.ECHO
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
	return k.control(func(fd int) error {
		return unix.IoctlSetTermios(fd, ioctlWriteTermios, &t)
	})
}

// control calls f with the terminal's file descriptor.
func (k *Keyboard) control(f func(fd int) error) error {
	conn, err := k.tty.SyscallConn()
	if err != nil {
		return err
	}
	var ferr error
	if err := conn.Control(func(fd uintptr) { ferr = f(int(fd)) }); err != nil {
		return err
	}
	return ferr
}
