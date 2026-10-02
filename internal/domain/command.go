package domain

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
)

var (
	// ErrNoCommand is a command with nothing in it to run.
	ErrNoCommand = errors.New("command: nothing to run")
	// ErrShellScript is a shell command given as more than one argument.
	ErrShellScript = errors.New(`command: give a shell command as one argument, quoted, with "$@" where the files go`)
)

// Command is what a watch runs each time: a program and its arguments, in
// which an argument that is exactly {} stands for the watched files.
type Command struct {
	argv []string
}

// NewCommand returns the Command argv spells. With shell, argv is one
// script, run with sh -c and the files in "$@" rather than in a {} of the
// script's own, since awk programs are full of {}. It is sh, not $SHELL,
// because fish has no "$@".
func NewCommand(argv []string, shell bool) (Command, error) {
	switch {
	case len(argv) == 0:
		return Command{}, ErrNoCommand
	case shell && len(argv) != 1:
		return Command{}, ErrShellScript
	case shell:
		return Command{argv: []string{"sh", "-c", argv[0], "sh", "{}"}}, nil
	}
	return Command{argv: slices.Clone(argv)}, nil
}

// Program returns the program the command runs.
func (c Command) Program() string {
	return c.argv[0]
}

// For returns the command as it runs for files: every argument that is
// exactly {} replaced by files. A file the command already names itself, as
// awk -f names its script, is left out of {} rather than passed twice; see
// names.
func (c Command) For(files []string) Argv {
	names := c.names()
	named := func(file string) bool {
		return slices.ContainsFunc(names, func(name string) bool {
			return filepath.Clean(name) == filepath.Clean(file)
		})
	}
	var argv Argv
	for _, arg := range c.argv {
		if arg != "{}" {
			argv = append(argv, arg)
			continue
		}
		for _, file := range files {
			if !named(file) {
				argv = append(argv, file)
			}
		}
	}
	return argv
}

// names returns the words the command can name a file with: its arguments,
// and when it has a shell run a script with -c, the words of the script.
// Every argument is not split into words that way: awk programs name files
// in strings, as FILENAME == "a.csv" does, and those files would go missing
// from {}.
func (c Command) names() []string {
	names := slices.Clone(c.argv)
	if script, ok := shellScript(c.argv); ok {
		names = append(names, shellWords(script)...)
	}
	return names
}

// Argv is a command as it runs: a program and its arguments, the files in.
type Argv []string

// String spells argv as one line that a shell reads back as the same
// arguments, quoting the ones that need it, so it can be copied and run.
func (a Argv) String() string {
	quoted := make([]string, len(a))
	for i, arg := range a {
		quoted[i] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}
