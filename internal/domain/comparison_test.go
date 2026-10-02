package domain_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/romanidis/watcheroo/internal/domain"
)

var finished = domain.OutcomeOK(time.Millisecond)

// spell spells d's lines one a line: two spaces before a line that stayed,
// - before one that went and + before one that came, with the changed spans
// in [brackets], and a run of lines left out as (N unchanged lines).
func spell(d domain.Diff) string {
	var b strings.Builder
	for _, line := range d.Lines {
		switch line.Mark {
		case domain.MarkSkipped:
			fmt.Fprintf(&b, "  (%d unchanged lines)\n", line.Skipped)
			continue
		case domain.MarkGone:
			b.WriteString("- ")
		case domain.MarkCame:
			b.WriteString("+ ")
		default:
			b.WriteString("  ")
		}
		for _, span := range line.Spans {
			if span.Changed {
				b.WriteString("[" + span.Text + "]")
			} else {
				b.WriteString(span.Text)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// compare compares output with prev, and returns the diff as spell spells it.
func compare(t *testing.T, prev, output string, context int) string {
	t.Helper()
	c := domain.NewComparison(domain.BaselinePrevious, context)
	c.Compare(prev, finished)
	d, ok := c.Compare(output, finished)
	if !ok {
		t.Fatal("a run that finished was not compared")
	}
	return spell(d)
}

func TestComparisonMarksLines(t *testing.T) {
	tests := []struct {
		name   string
		prev   string
		output string
		want   string
	}{
		{name: "a changed line", prev: "a 1\nb 2\nc 3\n", output: "a 1\nb 5\nc 3\n", want: "  a 1\n- b [2]\n+ b [5]\n  c 3\n"},
		{name: "an added line", prev: "a 1\n", output: "a 1\nz 9\n", want: "  a 1\n+ z 9\n"},
		{name: "a removed line", prev: "a 1\nz 9\n", output: "a 1\n", want: "  a 1\n- z 9\n"},
		{name: "output from nothing", prev: "", output: "a 1\n", want: "+ a 1\n"},
		{name: "output that ends without a newline", prev: "a 1", output: "a 2", want: "- a [1]\n+ a [2]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compare(t, tt.prev, tt.output, -1); got != tt.want {
				t.Errorf("marked\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestComparisonMarksWords(t *testing.T) {
	tests := []struct {
		name   string
		before string
		after  string
		want   string
	}{
		{
			name:   "a number changed",
			before: "alice,10",
			after:  "alice,14",
			want:   "- alice,[10]\n+ alice,[14]\n",
		},
		{
			name:   "two words changed, and the spacing that aligns one",
			before: "carol     7   120  north",
			after:  "carol    19   120  south",
			want:   "- carol[     7]   120  [north]\n+ carol[    19]   120  [south]\n",
		},
		{
			name:   "a word added",
			before: "total 5",
			after:  "total 5 (est)",
			want:   "- total 5\n+ total 5[ (est)]\n",
		},
		{
			name:   "lines with no word in common are left alone",
			before: "bob 1",
			after:  "zed 5",
			want:   "- bob 1\n+ zed 5\n",
		},
		{
			name:   "every other character is a word of its own",
			before: "x==-3.5",
			after:  "x==-3.6",
			want:   "- x==-3.[5]\n+ x==-3.[6]\n",
		},
		{
			name:   "letters and digits beyond ASCII",
			before: "café 12€",
			after:  "café 13€",
			want:   "- café [12]€\n+ café [13]€\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compare(t, tt.before, tt.after, -1); got != tt.want {
				t.Errorf("marked\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestComparisonContext(t *testing.T) {
	tests := []struct {
		name    string
		prev    string
		output  string
		context int
		want    string
	}{
		{
			name:    "the whole output, with context below 0",
			prev:    "1\n2\n3\n4\n5\n",
			output:  "1\n2\nthree\n4\n5\n",
			context: -1,
			want:    "  1\n  2\n- 3\n+ three\n  4\n  5\n",
		},
		{
			name:    "the lines around a change, and the rest counted",
			prev:    "1\n2\n3\n4\n5\n6\n7\n8\n9\n",
			output:  "1\n2\n3\n4\nfive\n6\n7\n8\n9\n",
			context: 1,
			want:    "  (3 unchanged lines)\n  4\n- 5\n+ five\n  6\n  (3 unchanged lines)\n",
		},
		{
			name:    "only the changes, with context 0",
			prev:    "1\n2\n3\n4\n5\n",
			output:  "one\n2\n3\n4\nfive\n",
			context: 0,
			want:    "- 1\n+ one\n  (3 unchanged lines)\n- 5\n+ five\n",
		},
		{
			name:    "a single line left out is kept rather than counted",
			prev:    "1\n2\n3\n",
			output:  "one\n2\nthree\n",
			context: 0,
			want:    "- 1\n+ one\n  2\n- 3\n+ three\n",
		},
		{
			name:    "nothing changed",
			prev:    "1\n2\n3\n",
			output:  "1\n2\n3\n",
			context: 2,
			want:    "  (3 unchanged lines)\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compare(t, tt.prev, tt.output, tt.context); got != tt.want {
				t.Errorf("marked\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestComparisonFirstRun(t *testing.T) {
	c := domain.NewComparison(domain.BaselinePrevious, 0)
	d, ok := c.Compare("1\n2\n", finished)
	if !ok || !d.First || d.Unchanged {
		t.Fatalf("the first run compared as %+v, %v, want it first, and not unchanged", d, ok)
	}
	if got, want := spell(d), "  1\n  2\n"; got != want {
		t.Errorf("the first run marked\n%s\nwant it whole, even with context 0:\n%s", got, want)
	}
}

func TestComparisonBaseline(t *testing.T) {
	tests := []struct {
		name     string
		baseline domain.Baseline
		outputs  []string
		want     []string
	}{
		{
			name:     "previous compares each run with the one before",
			baseline: domain.BaselinePrevious,
			outputs:  []string{"a 1\n", "a 2\n", "a 2\n"},
			want:     []string{"  a 1\n", "- a [1]\n+ a [2]\n", "(unchanged)  a 2\n"},
		},
		{
			name:     "first compares every run with the first",
			baseline: domain.BaselineFirst,
			outputs:  []string{"a 1\n", "a 2\n", "a 3\n", "a 1\n"},
			want:     []string{"  a 1\n", "- a [1]\n+ a [2]\n", "- a [1]\n+ a [3]\n", "(unchanged)  a 1\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := domain.NewComparison(tt.baseline, -1)
			var got []string
			for _, output := range tt.outputs {
				d, _ := c.Compare(output, finished)
				spelled := spell(d)
				if d.Unchanged {
					spelled = "(unchanged)" + spelled
				}
				got = append(got, spelled)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("compared as\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestComparisonLeavesOutARunThatDidNotFinish(t *testing.T) {
	for _, o := range []domain.Outcome{
		domain.OutcomeStopped(time.Second),
		domain.OutcomeTimedOut(time.Second, time.Second),
	} {
		t.Run(string(o.Ending()), func(t *testing.T) {
			c := domain.NewComparison(domain.BaselinePrevious, -1)
			c.Compare("a 1\n", finished)
			if _, ok := c.Compare("a 2\n", o); ok {
				t.Errorf("a run that %s was compared", o.Ending())
			}
			d, _ := c.Compare("a 1\n", finished)
			if !d.Unchanged {
				t.Errorf("the run after compared as %q, want it compared with the run before the one that %s", spell(d), o.Ending())
			}
		})
	}
	failed := domain.NewComparison(domain.BaselinePrevious, -1)
	if _, ok := failed.Compare("partial\n", domain.OutcomeExit(3, time.Second)); !ok {
		t.Error("a run that failed was not compared, want what it printed shown")
	}
}

func TestComparisonRebase(t *testing.T) {
	c := domain.NewComparison(domain.BaselineFirst, -1)
	if c.Rebase() {
		t.Error("rebased before any run was shown")
	}
	c.Compare("a 1\n", finished)
	c.Compare("a 2\n", finished)
	if !c.Rebase() {
		t.Fatal("did not rebase after two runs")
	}
	d, _ := c.Compare("a 2\n", finished)
	if !d.Unchanged {
		t.Errorf("the run after a rebase compared as %q, want it compared with the run shown last", spell(d))
	}
}
