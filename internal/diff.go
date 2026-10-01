package internal

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"

	"github.com/aymanbagabas/go-udiff/lcs"
)

// writeMarked writes the whole of output, each line marked against prev: two
// spaces for a line that stayed, - in red for one that went, + in green for
// one that came. In a removed line and the added line that took its place,
// the words that differ are highlighted, when p has colours to do it with.
//
// With context at 0 or more, a line that stayed is written only when it is
// within context lines of one that went or came. Each run of lines left out
// is counted in their place, unless it is a single line, which takes no more
// room than the count would.
func writeMarked(w io.Writer, p palette, prev, output string, context int) {
	type markedLine struct {
		sign byte // ' ' for a line that stayed, '-' for one that went, '+' for one that came
		text string
	}
	var marked []markedLine
	before, after := lines(prev), lines(output)
	next := 0 // the first line of after not yet marked
	for _, d := range lcs.DiffLines(before, after) {
		for ; next < d.ReplStart; next++ {
			marked = append(marked, markedLine{' ', after[next]})
		}
		gone := slices.Clone(before[d.Start:d.End])
		came := slices.Clone(after[d.ReplStart:d.ReplEnd])
		if p != noColors {
			for i := range min(len(gone), len(came)) {
				gone[i], came[i] = highlightWords(gone[i], came[i])
			}
		}
		for _, line := range gone {
			marked = append(marked, markedLine{'-', line})
		}
		for _, line := range came {
			marked = append(marked, markedLine{'+', line})
		}
		next = d.ReplEnd
	}
	for ; next < len(after); next++ {
		marked = append(marked, markedLine{' ', after[next]})
	}

	// near[i] is whether marked[i] is to be written: it went or came, or it
	// is within context lines of one that did.
	near := make([]bool, len(marked))
	changed := -1 // the last line that went or came
	for i, line := range marked {
		if line.sign != ' ' {
			changed = i
		}
		near[i] = context < 0 || changed >= 0 && i-changed <= context
	}
	changed = -1
	for i := len(marked) - 1; i >= 0; i-- {
		if marked[i].sign != ' ' {
			changed = i
		}
		near[i] = near[i] || changed >= 0 && changed-i <= context
	}

	for i := 0; i < len(marked); {
		far := 0 // the lines from i on that are not to be written
		for i+far < len(marked) && !near[i+far] {
			far++
		}
		switch {
		case far > 1:
			fmt.Fprintf(w, "%s  (%d unchanged lines)%s\n", p.dim, far, p.reset)
			i += far
			continue
		case marked[i].sign == '-':
			fmt.Fprintf(w, "%s- %s%s\n", p.red, marked[i].text, p.reset)
		case marked[i].sign == '+':
			fmt.Fprintf(w, "%s+ %s%s\n", p.green, marked[i].text, p.reset)
		default:
			fmt.Fprintf(w, "  %s\n", marked[i].text)
		}
		i++
	}
}

// lines splits text into its lines, without their newlines.
func lines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// highlightWords returns before and after with the words that differ between
// them in reverse video. Lines that share no word come back as they are: they
// are two different lines rather than one line changed, and highlighting all
// of both would say nothing.
func highlightWords(before, after string) (string, string) {
	a, b := words(before), words(after)
	changedA, changedB := make([]bool, len(a)), make([]bool, len(b))
	for _, d := range lcs.DiffLines(a, b) {
		for i := d.Start; i < d.End; i++ {
			changedA[i] = true
		}
		for i := d.ReplStart; i < d.ReplEnd; i++ {
			changedB[i] = true
		}
	}
	shared := false
	for i, word := range a {
		if !changedA[i] && strings.TrimSpace(word) != "" {
			shared = true
			break
		}
	}
	if !shared {
		return before, after
	}
	return mark(a, changedA), mark(b, changedB)
}

// words splits a line into the units highlightWords compares: runs of
// letters and digits, runs of spaces, and every other character on its own.
func words(line string) []string {
	class := func(r rune) int {
		switch {
		case unicode.IsSpace(r):
			return 1
		case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			return 2
		}
		return 0
	}
	var out []string
	start, prev := 0, -1
	for i, r := range line {
		c := class(r)
		if i > start && (c != prev || c == 0) {
			out = append(out, line[start:i])
			start = i
		}
		prev = c
	}
	if start < len(line) {
		out = append(out, line[start:])
	}
	return out
}

// mark joins words back into a line, with each run of changed words in reverse video.
func mark(words []string, changed []bool) string {
	var b strings.Builder
	for i, word := range words {
		if changed[i] && (i == 0 || !changed[i-1]) {
			b.WriteString(reverse)
		}
		b.WriteString(word)
		if changed[i] && (i == len(words)-1 || !changed[i+1]) {
			b.WriteString(reverseOff)
		}
	}
	return b.String()
}
