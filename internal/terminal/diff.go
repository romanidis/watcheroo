package terminal

import (
	"fmt"
	"strings"

	"github.com/romanidis/watcheroo/internal/domain"
)

// writeDiff writes d: two spaces before a line that stayed, - in red before
// one that went, + in green before one that came, and in those two, the
// words that changed in reverse video. Lines left out are counted in their
// place. A note above says when nothing changed.
func (s *Screen) writeDiff(d domain.Diff) {
	p := s.palette
	switch {
	case d.Unchanged && d.Baseline == domain.BaselineFirst:
		fmt.Fprintf(s.stdout, "%s(output as in the baseline run)%s\n", p.dim, p.reset)
	case d.Unchanged:
		fmt.Fprintf(s.stdout, "%s(output unchanged)%s\n", p.dim, p.reset)
	}
	for _, line := range d.Lines {
		switch line.Mark {
		case domain.MarkSkipped:
			fmt.Fprintf(s.stdout, "%s  (%d unchanged lines)%s\n", p.dim, line.Skipped, p.reset)
		case domain.MarkGone:
			fmt.Fprintf(s.stdout, "%s- %s%s\n", p.red, s.spans(line), p.reset)
		case domain.MarkCame:
			fmt.Fprintf(s.stdout, "%s+ %s%s\n", p.green, s.spans(line), p.reset)
		default:
			fmt.Fprintf(s.stdout, "  %s\n", line.Text())
		}
	}
}

// spans returns line with its changed spans in reverse video.
func (s *Screen) spans(line domain.Line) string {
	var b strings.Builder
	for _, span := range line.Spans {
		if span.Changed {
			b.WriteString(s.palette.reverse + span.Text + s.palette.reverseOff)
		} else {
			b.WriteString(span.Text)
		}
	}
	return b.String()
}
