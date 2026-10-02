package internal

import (
	"path/filepath"
	"strings"
)

// JoinCommand spells argv as one line that a shell reads back as the same
// arguments, quoting the ones that need it.
func JoinCommand(argv []string) string {
	quoted := make([]string, len(argv))
	for i, arg := range argv {
		quoted[i] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

// shellQuote quotes s for sh, unless it is safe as it is.
func shellQuote(s string) string {
	unsafe := func(c rune) bool {
		return !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.ContainsRune("_@%+=:,./-", c))
	}
	if s != "" && !strings.ContainsFunc(s, unsafe) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellScript returns the script argv has a shell run with -c, as in
// sh -c 'awk -f report.awk "$@"' sh {}, if it has one.
func shellScript(argv []string) (string, bool) {
	switch filepath.Base(argv[0]) {
	case "sh", "bash", "dash", "ksh", "zsh":
	default:
		return "", false
	}
	for i := 1; i+1 < len(argv); i++ {
		arg := argv[i]
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.ContainsRune(arg, 'c') {
			return argv[i+1], true
		}
	}
	return "", false
}

// shellWords splits script into its words as sh would, closely enough to
// find the file names among them: quotes and backslashes are taken out, and
// | ; & < > ( ) end a word as a blank does. Expansions are left as written,
// so a quoted string inside an awk program stays part of the program's word.
func shellWords(script string) []string {
	var (
		words  []string
		word   strings.Builder
		inWord bool
		quote  rune // the quote the word is inside, or 0
		escape bool
	)
	for _, c := range script {
		switch {
		case escape:
			word.WriteRune(c)
			escape = false
		case quote != 0 && c == quote:
			quote = 0
		case quote == '\'':
			word.WriteRune(c)
		case c == '\\':
			escape, inWord = true, true
		case quote == '"':
			word.WriteRune(c)
		case c == '\'' || c == '"':
			quote, inWord = c, true
		case strings.ContainsRune(" \t\n|;&<>()", c):
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(c)
			inWord = true
		}
	}
	if inWord {
		words = append(words, word.String())
	}
	return words
}
