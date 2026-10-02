//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package internal

import "golang.org/x/sys/unix"

const (
	ioctlReadTermios  = unix.TIOCGETA
	ioctlWriteTermios = unix.TIOCSETA
)
