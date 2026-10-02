package domain

import (
	"errors"
	"fmt"
)

// ErrUnknownBaseline is a baseline that is none of BaselineValues.
var ErrUnknownBaseline = errors.New("unknown baseline")

// Baseline is the run whose output ModeDiff compares each run's output with.
// The set is closed.
type Baseline string

const (
	// BaselinePrevious compares each run with the run just before it.
	BaselinePrevious Baseline = "previous"
	// BaselineFirst compares each run with the first run.
	BaselineFirst Baseline = "first"
)

// String returns b as its text, which for Baseline is the value itself.
func (b Baseline) String() string {
	return string(b)
}

// IsValid reports whether b is one of the declared Baseline constants.
func (b Baseline) IsValid() bool {
	switch b {
	case BaselinePrevious, BaselineFirst:
		return true
	}
	return false
}

// ParseBaseline returns the Baseline whose String is s.
func ParseBaseline(s string) (Baseline, error) {
	baseline := Baseline(s)
	if baseline.IsValid() {
		return baseline, nil
	}
	return "", fmt.Errorf("%w %q: it is one of %v", ErrUnknownBaseline, s, BaselineValues())
}

// BaselineValues returns the declared Baseline constants, in declaration order.
func BaselineValues() []Baseline {
	return []Baseline{
		BaselinePrevious,
		BaselineFirst,
	}
}
