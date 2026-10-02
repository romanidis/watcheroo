// Package cli is what the command line needs beyond cobra itself: the
// --watch and --regex flags, which keep the order they were given in among
// the names before --. It reads flags, and hands back what was given as the
// watch use cases take it, without making sense of it.
package cli

import (
	"strings"

	"github.com/romanidis/watcheroo/internal/watch"
	"github.com/spf13/pflag"
)

// PatternList collects --watch and --regex in the order they are given, so
// that the names before -- that are not flags can be put in among them where
// they were given too. That order is the order {} lists the files in.
//
// pflag hands back the flag values and the other arguments as two lists, so
// each flag value notes how many of the other arguments came before it. That
// count is right because pflag adds each argument to Args as it reaches it,
// before it parses what follows; TestPatternListOrder pins it.
type PatternList struct {
	flags *pflag.FlagSet
	given []givenPattern
}

// NewPatternList returns an empty PatternList for the flags of flags, whose
// Args it counts as each pattern is given.
func NewPatternList(flags *pflag.FlagSet) *PatternList {
	return &PatternList{flags: flags}
}

// givenPattern is one --watch or --regex.
type givenPattern struct {
	expr  string
	regex bool
	after int // how many of the arguments that are not flags came before it
}

// newGivenPattern returns expr, given to --regex when regex is set and to
// --watch when not, after after of the arguments that are not flags.
func newGivenPattern(expr string, regex bool, after int) givenPattern {
	return givenPattern{
		expr:  expr,
		regex: regex,
		after: after,
	}
}

// Value returns the pflag.Value of --watch, or of --regex when regex is set.
func (l *PatternList) Value(regex bool) pflag.Value {
	return newPatternValue(l, regex)
}

// Inputs returns every pattern given, with names, the arguments before --
// that are not flags, as globs among them in the places they were given.
func (l *PatternList) Inputs(names []string) []watch.PatternInput {
	inputs := make([]watch.PatternInput, 0, len(l.given)+len(names))
	given := l.given
	for i := 0; i <= len(names); i++ {
		for len(given) > 0 && given[0].after <= i {
			inputs = append(inputs, watch.PatternInput{Expr: given[0].expr, Regex: given[0].regex})
			given = given[1:]
		}
		if i < len(names) {
			inputs = append(inputs, watch.PatternInput{Expr: names[i]})
		}
	}
	return inputs
}

// patternValue is the pflag.Value of --watch, or of --regex when regex is set.
type patternValue struct {
	list  *PatternList
	regex bool
}

// newPatternValue returns the pflag.Value that adds to list what is given to
// --watch, or to --regex when regex is set.
func newPatternValue(list *PatternList, regex bool) *patternValue {
	return &patternValue{
		list:  list,
		regex: regex,
	}
}

func (v *patternValue) Set(expr string) error {
	v.list.given = append(v.list.given, newGivenPattern(expr, v.regex, len(v.list.flags.Args())))
	return nil
}

// String lists the values given to the flag. It is empty until one is, so
// --help shows no default.
func (v *patternValue) String() string {
	var exprs []string
	for _, g := range v.list.given {
		if g.regex == v.regex {
			exprs = append(exprs, g.expr)
		}
	}
	return strings.Join(exprs, ",")
}

// Type is what --help says the flag takes, the same as for a StringArray flag.
func (v *patternValue) Type() string {
	return "stringArray"
}
