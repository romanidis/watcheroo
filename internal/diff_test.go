package internal

import (
	"bytes"
	"slices"
	"testing"
)

// hl spells s the way highlightWords marks it.
func hl(s string) string { return reverse + s + reverseOff }

func TestHighlightWords(t *testing.T) {
	tests := []struct {
		name       string
		before     string
		after      string
		wantBefore string
		wantAfter  string
	}{
		{
			name:       "a number changed",
			before:     "alice,10",
			after:      "alice,14",
			wantBefore: "alice," + hl("10"),
			wantAfter:  "alice," + hl("14"),
		},
		{
			name:       "two words changed, and the spacing that aligns one",
			before:     "carol     7   120  north",
			after:      "carol    19   120  south",
			wantBefore: "carol" + hl("     7") + "   120  " + hl("north"),
			wantAfter:  "carol" + hl("    19") + "   120  " + hl("south"),
		},
		{
			name:       "a word added",
			before:     "total 5",
			after:      "total 5 (est)",
			wantBefore: "total 5",
			wantAfter:  "total 5" + hl(" (est)"),
		},
		{
			name:       "lines with no word in common are left alone",
			before:     "bob 1",
			after:      "zed 5",
			wantBefore: "bob 1",
			wantAfter:  "zed 5",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBefore, gotAfter := highlightWords(tt.before, tt.after)
			if gotBefore != tt.wantBefore || gotAfter != tt.wantAfter {
				t.Errorf("highlightWords(%q, %q)\n got %q, %q\nwant %q, %q",
					tt.before, tt.after, gotBefore, gotAfter, tt.wantBefore, tt.wantAfter)
			}
		})
	}
}

func TestWords(t *testing.T) {
	tests := []struct {
		line string
		want []string
	}{
		{line: "", want: nil},
		{line: "alice,10", want: []string{"alice", ",", "10"}},
		{line: "a  b_c", want: []string{"a", "  ", "b_c"}},
		{line: "x==-3.5", want: []string{"x", "=", "=", "-", "3", ".", "5"}},
		{line: "café 12€", want: []string{"café", " ", "12", "€"}},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			if got := words(tt.line); !slices.Equal(got, tt.want) {
				t.Errorf("words(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestWriteMarkedColours(t *testing.T) {
	var out bytes.Buffer
	writeMarked(&out, colors, "a 1\nb 2\n", "a 1\nb 3\n", -1)
	want := "  a 1\n" +
		red + "- b " + hl("2") + reset + "\n" +
		green + "+ b " + hl("3") + reset + "\n"
	if out.String() != want {
		t.Errorf("wrote %q, want %q", out.String(), want)
	}
}

func TestWriteMarkedContext(t *testing.T) {
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
			name:    "a single line left out is written rather than counted",
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
			var out bytes.Buffer
			writeMarked(&out, noColors, tt.prev, tt.output, tt.context)
			if out.String() != tt.want {
				t.Errorf("wrote\n%s\nwant\n%s", out.String(), tt.want)
			}
		})
	}
}
