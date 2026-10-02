package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/romanidis/watcheroo/internal"
	"github.com/spf13/pflag"
)

// patternList collects --watch and --regex in the order they are given, so
// that the names before -- that are not flags can be put in among them where
// they were given too. That order is the order {} lists the files in.
//
// pflag hands back the flag values and the other arguments as two lists, so
// each flag value notes how many of the other arguments came before it. That
// count is right because pflag adds each argument to Args as it reaches it,
// before it parses what follows; TestWatcherooList pins it.
type patternList struct {
	flags *pflag.FlagSet
	given []givenPattern
}

// givenPattern is one --watch or --regex.
type givenPattern struct {
	expr  string
	regex bool
	after int // how many of the arguments that are not flags came before it
}

// value returns the pflag.Value of --watch, or of --regex when regex is set.
func (l *patternList) value(regex bool) pflag.Value {
	return &patternValue{list: l, regex: regex}
}

// patterns returns every pattern given, with names, the arguments before --
// that are not flags, as globs among them in the places they were given.
func (l *patternList) patterns(names []string) ([]internal.Pattern, error) {
	patterns := make([]internal.Pattern, 0, len(l.given)+len(names))
	given := l.given
	for i := 0; i <= len(names); i++ {
		for len(given) > 0 && given[0].after <= i {
			p, err := given[0].pattern()
			if err != nil {
				return nil, err
			}
			patterns = append(patterns, p)
			given = given[1:]
		}
		if i < len(names) {
			patterns = append(patterns, internal.Pattern{Glob: names[i]})
		}
	}
	return patterns, nil
}

// pattern returns the Pattern g names, its regex compiled.
func (g givenPattern) pattern() (internal.Pattern, error) {
	if !g.regex {
		return internal.Pattern{Glob: g.expr}, nil
	}
	re, err := regexp.Compile(g.expr)
	if err != nil {
		return internal.Pattern{}, fmt.Errorf("--regex: %w", err)
	}
	return internal.Pattern{Regex: re}, nil
}

// patternValue is the pflag.Value of --watch, or of --regex when regex is set.
type patternValue struct {
	list  *patternList
	regex bool
}

func (v *patternValue) Set(expr string) error {
	v.list.given = append(v.list.given, givenPattern{expr: expr, regex: v.regex, after: len(v.list.flags.Args())})
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
