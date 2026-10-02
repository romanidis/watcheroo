//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package terminal

import (
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// tty reads the keys pressed on the terminal, each as it is pressed and
// without echoing it. It is cbreak mode rather than raw: Ctrl-C still
// interrupts, and output is left as it is, so the command's output looks as
// it would without keys being read.
type tty struct {
	file  *os.File
	saved unix.Termios // the terminal's settings before, which close puts back
	cont  chan os.Signal

	mu     sync.Mutex
	closed bool
}

// openTTY opens the terminal wtr runs in, and sets it up to read keys. It
// fails when there is no terminal.
func openTTY() (*tty, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	k := &tty{file: f, cont: make(chan os.Signal, 1)}
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
		f.Close()
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

// readKey waits for the next key pressed, and returns its first byte.
func (k *tty) readKey() (byte, error) {
	var b [1]byte
	if _, err := k.file.Read(b[:]); err != nil {
		return 0, err
	}
	return b[0], nil
}

// close puts the terminal back as it was.
func (k *tty) close() error {
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
	return errors.Join(err, k.file.Close())
}

// cbreak makes the terminal hand over each key as it is pressed, without
// echoing it, unless close has put it back already.
func (k *tty) cbreak() error {
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
func (k *tty) control(f func(fd int) error) error {
	conn, err := k.file.SyscallConn()
	if err != nil {
		return err
	}
	var ferr error
	if err := conn.Control(func(fd uintptr) { ferr = f(int(fd)) }); err != nil {
		return err
	}
	return ferr
}
