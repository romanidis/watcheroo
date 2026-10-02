package cli_test

import (
	"reflect"
	"testing"

	"github.com/romanidis/watcheroo/internal/cli"
	"github.com/romanidis/watcheroo/internal/watch"
	"github.com/spf13/pflag"
)

func TestPatternListOrder(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []watch.PatternInput
	}{
		{
			name: "names after --watch are globs too",
			args: []string{"--watch", "report.awk", "b.csv", "--", "awk"},
			want: []watch.PatternInput{{Expr: "report.awk"}, {Expr: "b.csv"}},
		},
		{
			name: "names in among --watch and --regex where they were given",
			args: []string{"b.csv", "-w", "out.csv", "a.csv", "-r", "^report", "--", "echo"},
			want: []watch.PatternInput{{Expr: "b.csv"}, {Expr: "out.csv"}, {Expr: "a.csv"}, {Expr: "^report", Regex: true}},
		},
		{
			name: "flags before any name",
			args: []string{"-r", `\.log$`, "-w", "x.awk", "--", "echo"},
			want: []watch.PatternInput{{Expr: `\.log$`, Regex: true}, {Expr: "x.awk"}},
		},
		{
			name: "nothing given",
			args: []string{"--", "echo"},
			want: []watch.PatternInput{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := pflag.NewFlagSet("wtr", pflag.ContinueOnError)
			patterns := cli.NewPatternList(flags)
			flags.VarP(patterns.Value(false), "watch", "w", "")
			flags.VarP(patterns.Value(true), "regex", "r", "")
			if err := flags.Parse(tt.args); err != nil {
				t.Fatal(err)
			}
			names := flags.Args()[:flags.ArgsLenAtDash()]
			if got := patterns.Inputs(names); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("gave %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPatternListHelp(t *testing.T) {
	flags := pflag.NewFlagSet("wtr", pflag.ContinueOnError)
	patterns := cli.NewPatternList(flags)
	watchFlag := patterns.Value(false)
	flags.Var(watchFlag, "watch", "")
	flags.Var(patterns.Value(true), "regex", "")
	if watchFlag.String() != "" || watchFlag.Type() != "stringArray" {
		t.Errorf("before anything is given, --watch says %q of type %q, want no default and stringArray", watchFlag.String(), watchFlag.Type())
	}
	if err := flags.Parse([]string{"--watch", "a", "--regex", "b", "--watch", "c"}); err != nil {
		t.Fatal(err)
	}
	if got := watchFlag.String(); got != "a,c" {
		t.Errorf("--watch says %q, want only its own values, a,c", got)
	}
}
