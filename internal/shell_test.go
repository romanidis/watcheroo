package internal

import (
	"slices"
	"testing"
)

func TestJoinCommand(t *testing.T) {
	tests := []struct {
		argv []string
		want string
	}{
		{argv: []string{"awk", "-f", "report.awk", "data/a.csv"}, want: "awk -f report.awk data/a.csv"},
		{argv: []string{"sh", "-c", `awk -f report.awk "$@" | sort`, "sh"}, want: `sh -c 'awk -f report.awk "$@" | sort' sh`},
		{argv: []string{"cat", "a b.csv", "it's", ""}, want: `cat 'a b.csv' 'it'\''s' ''`},
	}
	for _, tt := range tests {
		if got := JoinCommand(tt.argv); got != tt.want {
			t.Errorf("JoinCommand(%q) = %s, want %s", tt.argv, got, tt.want)
		}
	}
}

func TestShellWords(t *testing.T) {
	tests := []struct {
		script string
		want   []string
	}{
		{script: `awk -f report.awk "$@" | sort`, want: []string{"awk", "-f", "report.awk", "$@", "sort"}},
		{script: `awk 'FILENAME == "a.csv" { n++ }' "$@"`, want: []string{"awk", `FILENAME == "a.csv" { n++ }`, "$@"}},
		{script: `cat <lookup.csv;echo "my file.csv"`, want: []string{"cat", "lookup.csv", "echo", "my file.csv"}},
		{script: `echo it\'s a\ b`, want: []string{"echo", "it's", "a b"}},
	}
	for _, tt := range tests {
		if got := shellWords(tt.script); !slices.Equal(got, tt.want) {
			t.Errorf("shellWords(%s) = %q, want %q", tt.script, got, tt.want)
		}
	}
}
