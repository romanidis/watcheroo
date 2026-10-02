package domain

import (
	"errors"
	"fmt"
)

// ErrUnknownMode is a mode that is none of ModeValues.
var ErrUnknownMode = errors.New("unknown mode")

// Mode is how a run's output is shown next to the output of the runs before
// it. The set is closed.
type Mode string

const (
	// ModeClear clears the screen before each run, so only the latest output shows.
	ModeClear Mode = "clear"
	// ModeAppend keeps earlier output and prints each run below the last.
	ModeAppend Mode = "append"
	// ModeDiff shows how the output differs from the previous run's.
	ModeDiff Mode = "diff"
)

// String returns m as its text, which for Mode is the value itself.
func (m Mode) String() string {
	return string(m)
}

// IsValid reports whether m is one of the declared Mode constants.
func (m Mode) IsValid() bool {
	switch m {
	case ModeClear, ModeAppend, ModeDiff:
		return true
	}
	return false
}

// Compares reports whether runs in m are compared with the run before. Their
// output is kept to compare, rather than shown as it comes.
func (m Mode) Compares() bool {
	return m == ModeDiff
}

// ParseMode returns the Mode whose String is s.
func ParseMode(s string) (Mode, error) {
	mode := Mode(s)
	if mode.IsValid() {
		return mode, nil
	}
	return "", fmt.Errorf("%w %q: it is one of %v", ErrUnknownMode, s, ModeValues())
}

// ModeValues returns the declared Mode constants, in declaration order.
func ModeValues() []Mode {
	return []Mode{
		ModeClear,
		ModeAppend,
		ModeDiff,
	}
}
