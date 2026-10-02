package domain

import (
	"strings"
	"unicode"

	"github.com/aymanbagabas/go-udiff/lcs"
)

// Diff is how one run's output differs from the output it is compared with:
// the whole output, each line marked.
type Diff struct {
	First     bool     // there was no run to compare with, so nothing is marked
	Unchanged bool     // the output is the same as the one compared with
	Baseline  Baseline // the run compared with
	Lines     []Line
}

// Mark is what became of a line. The set is closed.
type Mark int

const (
	MarkKept    Mark = iota // the line stayed
	MarkGone                // the line went
	MarkCame                // the line came
	MarkSkipped             // lines that stayed, left out as far from any change
)

// Line is one line of a Diff, or, marked MarkSkipped, a run of lines left
// out in its place. In a line that went and the line that came in its place,
// the spans that differ between the two are marked Changed.
type Line struct {
	Mark    Mark
	Spans   []Span
	Skipped int // for MarkSkipped, how many lines were left out
}

// Text returns the line, its spans joined.
func (l Line) Text() string {
	var b strings.Builder
	for _, s := range l.Spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

// Span is a piece of a line, and whether it differs from the line it went or
// came in place of.
type Span struct {
	Text    string
	Changed bool
}

// markLines returns the whole of output, each line marked against prev.
//
// With context at 0 or more, a line that stayed is kept only when it is
// within context lines of one that went or came. Each run of lines left out
// is counted in their place, unless it is a single line, which takes no more
// room than the count would.
func markLines(prev, output string, context int) []Line {
	var marked []Line
	before, after := lines(prev), lines(output)
	next := 0 // the first line of after not yet marked
	for _, d := range lcs.DiffLines(before, after) {
		for ; next < d.ReplStart; next++ {
			marked = append(marked, plainLine(MarkKept, after[next]))
		}
		gone := before[d.Start:d.End]
		came := after[d.ReplStart:d.ReplEnd]
		goneSpans := make([][]Span, len(gone))
		cameSpans := make([][]Span, len(came))
		for i, line := range gone {
			goneSpans[i] = []Span{{Text: line}}
		}
		for i, line := range came {
			cameSpans[i] = []Span{{Text: line}}
		}
		for i := range min(len(gone), len(came)) {
			goneSpans[i], cameSpans[i] = changedWords(gone[i], came[i])
		}
		for _, spans := range goneSpans {
			marked = append(marked, Line{Mark: MarkGone, Spans: spans})
		}
		for _, spans := range cameSpans {
			marked = append(marked, Line{Mark: MarkCame, Spans: spans})
		}
		next = d.ReplEnd
	}
	for ; next < len(after); next++ {
		marked = append(marked, plainLine(MarkKept, after[next]))
	}

	// near[i] is whether marked[i] is kept: it went or came, or it is within
	// context lines of one that did.
	near := make([]bool, len(marked))
	changed := -1 // the last line that went or came
	for i, line := range marked {
		if line.Mark != MarkKept {
			changed = i
		}
		near[i] = context < 0 || changed >= 0 && i-changed <= context
	}
	changed = -1
	for i := len(marked) - 1; i >= 0; i-- {
		if marked[i].Mark != MarkKept {
			changed = i
		}
		near[i] = near[i] || changed >= 0 && changed-i <= context
	}

	var kept []Line
	for i := 0; i < len(marked); {
		far := 0 // the lines from i on that are left out
		for i+far < len(marked) && !near[i+far] {
			far++
		}
		if far > 1 {
			kept = append(kept, Line{Mark: MarkSkipped, Skipped: far})
			i += far
			continue
		}
		kept = append(kept, marked[i])
		i++
	}
	return kept
}

// plainLine returns text as a Line marked m, with nothing in it changed.
func plainLine(m Mark, text string) Line {
	return Line{Mark: m, Spans: []Span{{Text: text}}}
}

// lines splits text into its lines, without their newlines.
func lines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// changedWords returns before and after as spans, with the words that differ
// between them marked Changed. Lines that share no word come back whole and
// unmarked: they are two different lines rather than one line changed, and
// marking all of both would say nothing.
func changedWords(before, after string) ([]Span, []Span) {
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
		return []Span{{Text: before}}, []Span{{Text: after}}
	}
	return spans(a, changedA), spans(b, changedB)
}

// words splits a line into the units changedWords compares: runs of letters
// and digits, runs of spaces, and every other character on its own.
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

// spans joins words into spans, each run of changed words one span.
func spans(words []string, changed []bool) []Span {
	var out []Span
	for i, word := range words {
		if len(out) > 0 && out[len(out)-1].Changed == changed[i] {
			out[len(out)-1].Text += word
			continue
		}
		out = append(out, Span{Text: word, Changed: changed[i]})
	}
	return out
}
