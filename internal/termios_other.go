//go:build aix || linux || solaris

package internal

import "golang.org/x/sys/unix"

const (
	ioctlReadTermios  = unix.TCGETS
	ioctlWriteTermios = unix.TCSETS
)
