package cmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

// executeWatcheroo runs the root command with args and returns everything it wrote.
// Its context is already cancelled, so a watch that starts stops after its first run.
func executeWatcheroo(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(ctx)
	return out.String(), err
}

func TestWatcheroo(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{
			name: "with --help",
			args: []string{"--help"},
			want: "--regex stringArray",
		},
		{
			name: "--version",
			args: []string{"--version"},
			want: "wtr version ",
		},
		{
			name: "bare, it prints the help",
			args: []string{}, // not nil, which would make cobra read os.Args
			want: "Usage:\n  wtr [--watch] GLOB...",
		},
		{
			name: "globs listed after --watch are watched, not run",
			args: []string{"--watch", "file1.txt", "file2.txt", "--", "echo", "hi"},
			want: "  echo hi",
		},
		{
			name: "a file named like a command is still a file to watch",
			args: []string{"--watch", "src", "docs", "help", "completion", "--", "echo", "hi"},
			want: "  echo hi",
		},
		{
			name:    "no -- before the command",
			args:    []string{"--watch", "file1.txt", "echo", "hi"},
			want:    "give the command to run after --",
			wantErr: true,
		},
		{
			name:    "nothing after --",
			args:    []string{"--watch", "file1.txt", "--"},
			want:    "give the command to run after --",
			wantErr: true,
		},
		{
			name:    "nothing to watch",
			args:    []string{"--", "echo", "hi"},
			want:    "nothing to watch",
			wantErr: true,
		},
		{
			name:    "a glob that does not parse",
			args:    []string{"--watch", "[", "--", "echo"},
			want:    `glob "["`,
			wantErr: true,
		},
		{
			name:    "a regex that does not parse",
			args:    []string{"--regex", "(", "--", "echo"},
			want:    "--regex: error parsing regexp",
			wantErr: true,
		},
		{
			name:    "an unknown --mode",
			args:    []string{"--watch", "file1.txt", "--mode", "fancy", "--", "echo"},
			want:    `--mode is one of [clear append diff], not "fancy"`,
			wantErr: true,
		},
		{
			name:    "a zero --interval",
			args:    []string{"--watch", "file1.txt", "--interval", "0s", "--", "echo"},
			want:    "--interval must be more than zero",
			wantErr: true,
		},
		{
			name:    "an unknown --baseline",
			args:    []string{"--watch", "file1.txt", "-m", "diff", "--baseline", "last", "--", "echo"},
			want:    `--baseline is one of [previous first], not "last"`,
			wantErr: true,
		},
		{
			name:    "--baseline without diff mode",
			args:    []string{"--watch", "file1.txt", "--baseline", "first", "--", "echo"},
			want:    "--baseline and --context work only with --mode diff",
			wantErr: true,
		},
		{
			name:    "--context without diff mode",
			args:    []string{"--watch", "file1.txt", "-m", "append", "--context", "2", "--", "echo"},
			want:    "--baseline and --context work only with --mode diff",
			wantErr: true,
		},
		{
			name:    "a negative --context",
			args:    []string{"--watch", "file1.txt", "-m", "diff", "--context", "-1", "--", "echo"},
			want:    "--context cannot be less than zero",
			wantErr: true,
		},
		{
			name:    "a negative --debounce",
			args:    []string{"--watch", "file1.txt", "--debounce", "-1s", "--", "echo"},
			want:    "--debounce cannot be less than zero",
			wantErr: true,
		},
		{
			name:    "--restart in diff mode",
			args:    []string{"--watch", "file1.txt", "-m", "diff", "--restart", "--", "echo"},
			want:    "--restart does not work with --mode diff",
			wantErr: true,
		},
		{
			name:    "--shell with the command in more than one argument",
			args:    []string{"--watch", "file1.txt", "--shell", "--", "awk", "-f", "x.awk"},
			want:    "with --shell, give the command as one argument after --",
			wantErr: true,
		},
		{
			name:    "a negative --timeout",
			args:    []string{"--watch", "file1.txt", "--timeout", "-1s", "--", "echo"},
			want:    "--timeout cannot be less than zero",
			wantErr: true,
		},
		{
			name:    "--timeout with --restart",
			args:    []string{"--watch", "file1.txt", "--restart", "--timeout", "1s", "--", "echo"},
			want:    "--timeout does not work with --restart",
			wantErr: true,
		},
		{
			name:    "an --exclude that does not parse",
			args:    []string{"--watch", "file1.txt", "--exclude", "[", "--", "echo"},
			want:    `--exclude "[": syntax error in pattern`,
			wantErr: true,
		},
		{
			name:    "an --exclude-regex that does not parse",
			args:    []string{"--watch", "file1.txt", "--exclude-regex", "(", "--", "echo"},
			want:    "--exclude-regex: error parsing regexp",
			wantErr: true,
		},
		{
			name: "--mode completes to its values",
			args: []string{"__complete", "--mode", ""},
			want: "clear\nappend\ndiff\n:4\n",
		},
		{
			name: "--baseline completes to its values",
			args: []string{"__complete", "-m", "diff", "--baseline", ""},
			want: "previous\nfirst\n:4\n",
		},
		{
			name:    "a command that is not on PATH",
			args:    []string{"--watch", "file1.txt", "--", "watcheroo-no-such-command"},
			want:    "executable file not found",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := executeWatcheroo(t, tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ran with %v: %v", tt.args, err)
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("printed:\n%s\nwant it to contain %q", out, tt.want)
			}
		})
	}
}

func TestWatcherooList(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, name := range []string{"report.awk", "a.csv", "b.csv", "out.csv"} {
		if err := os.WriteFile(name, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "the files and the command they make",
			args: []string{"--list", "-w", "report.awk", "-w", "*.csv", "-x", "out.csv", "--", "awk", "-f", "report.awk", "{}"},
			want: "watching:\n  report.awk\n  a.csv\n  b.csv\nrunning:\n  awk -f report.awk a.csv b.csv\n",
		},
		{
			name: "{} keeps the order the files were given in",
			args: []string{"--list", "-w", "report.awk", "b.csv", "--", "awk", "-f", "{}"},
			want: "watching:\n  report.awk\n  b.csv\nrunning:\n  awk -f report.awk b.csv\n",
		},
		{
			name: "names before -- in among --watch and --regex where they were given",
			args: []string{"--list", "b.csv", "-w", "out.csv", "a.csv", "-r", "^report", "--", "echo", "{}"},
			want: "watching:\n  b.csv\n  out.csv\n  a.csv\n  report.awk\nrunning:\n  echo b.csv out.csv a.csv report.awk\n",
		},
		{
			name: "--shell runs the command with sh -c, the files in $@",
			args: []string{"--list", "-s", "-w", "report.awk", "a.csv", "--", `awk -f report.awk "$@" | sort`},
			want: "watching:\n  report.awk\n  a.csv\nrunning:\n  sh -c 'awk -f report.awk \"$@\" | sort' sh a.csv\n",
		},
		{
			name: "nothing matched",
			args: []string{"--list", "-w", "*.txt", "--", "echo", "{}"},
			want: "watching:\n  (nothing matches yet)\nmatching nothing yet:\n  *.txt\nrunning:\n  echo\n",
		},
		{
			name: "a pattern that matches nothing, among ones that do",
			args: []string{"--list", "-w", "report.awk", "reprot.awk", "-r", "^data/", "--", "echo", "{}"},
			want: "watching:\n  report.awk\nmatching nothing yet:\n  reprot.awk\n  ^data/\nrunning:\n  echo report.awk\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := executeWatcheroo(t, tt.args...)
			if err != nil {
				t.Fatalf("ran with %v: %v", tt.args, err)
			}
			if out != tt.want {
				t.Errorf("printed\n%s\nwant\n%s", out, tt.want)
			}
		})
	}
}

func TestWatcherooNotATerminal(t *testing.T) {
	out, err := executeWatcheroo(t, "--watch", "file1.txt", "-m", "diff", "--", "echo", "hi")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\033") {
		t.Errorf("printed %q, want no escape codes when the output is not a terminal", out)
	}
}
