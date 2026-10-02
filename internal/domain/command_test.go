package domain_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/romanidis/watcheroo/internal/domain"
)

func TestNewCommand(t *testing.T) {
	tests := []struct {
		name    string
		argv    []string
		shell   bool
		want    domain.Argv
		wantErr error
	}{
		{name: "nothing to run", argv: nil, wantErr: domain.ErrNoCommand},
		{name: "a shell command in two arguments", argv: []string{"awk", "-f"}, shell: true, wantErr: domain.ErrShellScript},
		{name: "a command as given", argv: []string{"awk", "-f", "x.awk", "{}"}, want: domain.Argv{"awk", "-f", "x.awk", "a.csv"}},
		{
			name:  "a shell command runs with sh -c, the files in $@",
			argv:  []string{`awk "$@" | sort`},
			shell: true,
			want:  domain.Argv{"sh", "-c", `awk "$@" | sort`, "sh", "a.csv"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := domain.NewCommand(tt.argv, tt.shell)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewCommand(%q, %v) returned %v, want %v", tt.argv, tt.shell, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got := c.For([]string{"a.csv"}); !slices.Equal(got, tt.want) {
				t.Errorf("for a.csv, it runs %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommandFor(t *testing.T) {
	tests := []struct {
		name  string
		argv  []string
		shell bool
		files []string
		want  domain.Argv
	}{
		{
			name:  "{} becomes one argument a file",
			argv:  []string{"cat", "{}"},
			files: []string{"a.csv", "b c.csv"},
			want:  domain.Argv{"cat", "a.csv", "b c.csv"},
		},
		{
			name:  "{} between other arguments",
			argv:  []string{"echo", "first", "{}", "last"},
			files: []string{"a.csv"},
			want:  domain.Argv{"echo", "first", "a.csv", "last"},
		},
		{
			name:  "{} when nothing matches",
			argv:  []string{"echo", "first", "{}", "last"},
			files: nil,
			want:  domain.Argv{"echo", "first", "last"},
		},
		{
			name:  "{} leaves out the files the command names itself",
			argv:  []string{"awk", "-f", "./report.awk", "{}"},
			files: []string{"data/a.csv", "report.awk"},
			want:  domain.Argv{"awk", "-f", "./report.awk", "data/a.csv"},
		},
		{
			name:  "{} keeps a file an argument names only as part of a longer word",
			argv:  []string{"echo", "FILENAME == a.csv", "{}"},
			files: []string{"a.csv"},
			want:  domain.Argv{"echo", "FILENAME == a.csv", "a.csv"},
		},
		{
			name:  "{} inside a longer argument is left alone",
			argv:  []string{"echo", "x{}"},
			files: []string{"a.csv"},
			want:  domain.Argv{"echo", "x{}"},
		},
		{
			name:  "a script given to sh -c names a file as a word",
			argv:  []string{"sh", "-c", `awk -f report.awk "$@" | sort`, "sh", "{}"},
			files: []string{"report.awk", "a.csv"},
			want:  domain.Argv{"sh", "-c", `awk -f report.awk "$@" | sort`, "sh", "a.csv"},
		},
		{
			name:  "a shell command names a file as a word",
			argv:  []string{`awk -f report.awk "$@" | sort`},
			shell: true,
			files: []string{"report.awk", "a.csv"},
			want:  domain.Argv{"sh", "-c", `awk -f report.awk "$@" | sort`, "sh", "a.csv"},
		},
		{
			name:  "a script names files after < and ;, and in quotes",
			argv:  []string{`cat <lookup.csv;echo "my file.csv"`},
			shell: true,
			files: []string{"lookup.csv", "my file.csv", "a.csv"},
			want:  domain.Argv{"sh", "-c", `cat <lookup.csv;echo "my file.csv"`, "sh", "a.csv"},
		},
		{
			name:  "a script names a file with an escaped space",
			argv:  []string{`echo it\'s a\ b`},
			shell: true,
			files: []string{"a b", "it's"},
			want:  domain.Argv{"sh", "-c", `echo it\'s a\ b`, "sh"},
		},
		{
			name:  "a string in an awk program in a script names no file",
			argv:  []string{`awk 'FILENAME == "a.csv" { n++ }' "$@"`},
			shell: true,
			files: []string{"a.csv"},
			want:  domain.Argv{"sh", "-c", `awk 'FILENAME == "a.csv" { n++ }' "$@"`, "sh", "a.csv"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := domain.NewCommand(tt.argv, tt.shell)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.For(tt.files); !slices.Equal(got, tt.want) {
				t.Errorf("for %q, it runs %q, want %q", tt.files, got, tt.want)
			}
		})
	}
}

func TestArgvString(t *testing.T) {
	tests := []struct {
		argv domain.Argv
		want string
	}{
		{argv: domain.Argv{"awk", "-f", "report.awk", "data/a.csv"}, want: "awk -f report.awk data/a.csv"},
		{argv: domain.Argv{"sh", "-c", `awk -f report.awk "$@" | sort`, "sh"}, want: `sh -c 'awk -f report.awk "$@" | sort' sh`},
		{argv: domain.Argv{"cat", "a b.csv", "it's", ""}, want: `cat 'a b.csv' 'it'\''s' ''`},
	}
	for _, tt := range tests {
		if got := tt.argv.String(); got != tt.want {
			t.Errorf("%q spelled %s, want %s", []string(tt.argv), got, tt.want)
		}
	}
}
