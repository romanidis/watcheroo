package internal

import "fmt"

// Mode is how a run's output is shown next to the output of the runs before it.
type Mode string

const (
	// ModeClear clears the screen before each run, so only the latest output shows.
	ModeClear Mode = "clear"
	// ModeAppend keeps earlier output and prints each run below the last.
	ModeAppend Mode = "append"
	// ModeDiff clears the screen and shows how the output differs from the previous run's.
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

// ParseMode returns the Mode whose String is s.
func ParseMode(s string) (Mode, error) {
	mode := Mode(s)
	if mode.IsValid() {
		return mode, nil
	}
	return "", fmt.Errorf("unknown Mode: %q", s)
}

// ModeValues returns the declared Mode constants, in declaration order.
func ModeValues() []Mode {
	return []Mode{
		ModeClear,
		ModeAppend,
		ModeDiff,
	}
}
