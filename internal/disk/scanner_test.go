package disk_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/romanidis/watcheroo/internal/disk"
	"github.com/romanidis/watcheroo/internal/domain"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// scan returns the files a look at list finds, in the order {} lists them.
func scan(t *testing.T, list domain.Watchlist) []string {
	t.Helper()
	matches, err := disk.Scanner{}.Scan(list)
	if err != nil {
		t.Fatal(err)
	}
	return domain.NewSnapshot(list, matches).Files()
}

// writeFiles creates each file under the working directory, with its directories.
func writeFiles(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScan(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFiles(t,
		"a.txt", "b.txt", "c.csv", ".env", ".a.txt.swp", "b.txt~",
		"data/jan.csv", "data/feb.csv", "data/notes.md", "data/.jan.csv.swp", "data/#jan.csv#", "data/old/dec.csv",
		".git/config", ".git/objects/ab/cdef",
	)

	tests := []struct {
		name    string
		globs   []string
		regexes []string
		hidden  bool
		exclude []string
		exRegex []string
		want    []string
	}{
		{
			name:  "a plain name",
			globs: []string{"a.txt"},
			want:  []string{"a.txt"},
		},
		{
			name:  "a name that does not exist yet",
			globs: []string{"later.txt"},
			want:  nil,
		},
		{
			name:  "a wildcard",
			globs: []string{"*.txt"},
			want:  []string{"a.txt", "b.txt"},
		},
		{
			name:  "a wildcard in a directory",
			globs: []string{"data/*.csv"},
			want:  []string{"data/feb.csv", "data/jan.csv"},
		},
		{
			name:  "a wildcard skips hidden files",
			globs: []string{"*", "data/*"},
			want: []string{
				"a.txt", "b.txt", "c.csv",
				"data/feb.csv", "data/jan.csv", "data/notes.md", "data/old/dec.csv",
			},
		},
		{
			name:  "** matches any number of directories",
			globs: []string{"data/**/*.csv"},
			want:  []string{"data/feb.csv", "data/jan.csv", "data/old/dec.csv"},
		},
		{
			name:  "** does not go into hidden directories",
			globs: []string{"**/config"},
			want:  nil,
		},
		{
			name:  "braces match either name",
			globs: []string{"*.{txt,csv}"},
			want:  []string{"a.txt", "b.txt", "c.csv"},
		},
		{
			name:    "editor backups are left out of what a regex matches",
			regexes: []string{`jan`},
			want:    []string{"data/jan.csv"},
		},
		{
			name:  "editor backups a glob names on purpose",
			globs: []string{"*~", "data/#*#"},
			want:  []string{"b.txt~", "data/#jan.csv#"},
		},
		{
			name:  "a hidden file named with its dot",
			globs: []string{".env", ".*.swp"},
			want:  []string{".env", ".a.txt.swp"},
		},
		{
			name:  "a directory stands for every file under it",
			globs: []string{"data"},
			want:  []string{"data/feb.csv", "data/jan.csv", "data/notes.md", "data/old/dec.csv"},
		},
		{
			name:  "a hidden directory named with its dot",
			globs: []string{".git"},
			want:  []string{".git/config", ".git/objects/ab/cdef"},
		},
		{
			name:    "a regex reaches into directories",
			regexes: []string{`\.csv$`},
			want:    []string{"c.csv", "data/feb.csv", "data/jan.csv", "data/old/dec.csv"},
		},
		{
			name:    "a regex matches files, not directories",
			regexes: []string{`^data`},
			want:    []string{"data/feb.csv", "data/jan.csv", "data/notes.md", "data/old/dec.csv"},
		},
		{
			name:    "a regex skips hidden files and directories",
			regexes: []string{`config|cdef|env|swp`},
			want:    nil,
		},
		{
			name:    "an anchored regex",
			regexes: []string{`^data/old/`},
			want:    []string{"data/old/dec.csv"},
		},
		{
			name:    "globs and regexes together, overlapping",
			globs:   []string{"a.txt", "c.csv"},
			regexes: []string{`\.md$`, `^c\.`},
			want:    []string{"a.txt", "c.csv", "data/notes.md"},
		},
		{
			name:   "--hidden lets a wildcard match hidden files",
			globs:  []string{"*"},
			hidden: true,
			want: []string{
				".a.txt.swp", ".env", ".git/config", ".git/objects/ab/cdef",
				"a.txt", "b.txt", "c.csv",
				"data/.jan.csv.swp", "data/feb.csv", "data/jan.csv", "data/notes.md", "data/old/dec.csv",
			},
		},
		{
			name:   "--hidden lets ** go into hidden directories",
			globs:  []string{"**/config"},
			hidden: true,
			want:   []string{".git/config"},
		},
		{
			name:    "--hidden lets a regex reach into hidden directories",
			regexes: []string{`config|env`},
			hidden:  true,
			want:    []string{".env", ".git/config"},
		},
		{
			name:    "an exclude without a slash names a directory wherever it is",
			globs:   []string{"data"},
			exclude: []string{"old"},
			want:    []string{"data/feb.csv", "data/jan.csv", "data/notes.md"},
		},
		{
			name:    "an exclude without a slash is a glob for names",
			regexes: []string{`\.csv$`},
			exclude: []string{"j*"},
			want:    []string{"c.csv", "data/feb.csv", "data/old/dec.csv"},
		},
		{
			name:    "an exclude with a slash is matched against the whole path",
			globs:   []string{"*.txt", "data"},
			exclude: []string{"data/*.md", "a.txt/"},
			want:    []string{"b.txt", "data/feb.csv", "data/jan.csv", "data/old/dec.csv"},
		},
		{
			name:    "an exclude leaves out what a glob matches under it",
			globs:   []string{"data/old/*.csv", "./data/*.csv"},
			exclude: []string{"./data/old", "feb.csv"},
			want:    []string{"data/jan.csv"},
		},
		{
			name:    "an exclude with **",
			globs:   []string{"data"},
			exclude: []string{"**/old"},
			want:    []string{"data/feb.csv", "data/jan.csv", "data/notes.md"},
		},
		{
			name:    "an exclude regex",
			regexes: []string{`\.csv$`},
			exRegex: []string{`^data/old/|^c`},
			want:    []string{"data/feb.csv", "data/jan.csv"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var patterns []domain.Pattern
			for _, glob := range tt.globs {
				patterns = append(patterns, must(domain.NewGlob(glob)))
			}
			for _, expr := range tt.regexes {
				patterns = append(patterns, must(domain.NewRegex(expr)))
			}
			var excludes []domain.Exclude
			for _, glob := range tt.exclude {
				excludes = append(excludes, must(domain.NewExcludeGlob(glob)))
			}
			for _, expr := range tt.exRegex {
				excludes = append(excludes, must(domain.NewExcludeRegex(expr)))
			}
			got := scan(t, must(domain.NewWatchlist(patterns, excludes, tt.hidden)))
			if !slices.Equal(got, tt.want) {
				t.Errorf("matched %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScanOrder(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFiles(t, "process.awk", "data.tsv", "a.csv", "b.csv", "logs/y.log", "logs/z.log")
	glob := func(s string) domain.Pattern { return must(domain.NewGlob(s)) }
	re := func(s string) domain.Pattern { return must(domain.NewRegex(s)) }

	tests := []struct {
		name     string
		patterns []domain.Pattern
		want     []string
	}{
		{
			name:     "files in the order of their patterns",
			patterns: []domain.Pattern{glob("process.awk"), glob("data.tsv")},
			want:     []string{"process.awk", "data.tsv"},
		},
		{
			name:     "the files one glob matches in path order",
			patterns: []domain.Pattern{glob("data.tsv"), glob("*.csv")},
			want:     []string{"data.tsv", "a.csv", "b.csv"},
		},
		{
			name:     "a file two globs match in the place of the first",
			patterns: []domain.Pattern{glob("b.csv"), glob("*.csv")},
			want:     []string{"b.csv", "a.csv"},
		},
		{
			name:     "a file a regex and a later glob match in the place of the regex",
			patterns: []domain.Pattern{re(`\.awk$`), glob("data.tsv"), glob("process.awk")},
			want:     []string{"process.awk", "data.tsv"},
		},
		{
			name:     "regexes walked from one directory",
			patterns: []domain.Pattern{re(`\.awk$`), re(`\.tsv$`)},
			want:     []string{"process.awk", "data.tsv"},
		},
		{
			name:     "regexes walked from different directories",
			patterns: []domain.Pattern{re(`^logs/`), glob("data.tsv"), re(`\.csv$`)},
			want:     []string{"logs/y.log", "logs/z.log", "data.tsv", "a.csv", "b.csv"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scan(t, must(domain.NewWatchlist(tt.patterns, nil, false)))
			if !slices.Equal(got, tt.want) {
				t.Errorf("listed %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScanStamps(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFiles(t, "a.txt")
	list := must(domain.NewWatchlist([]domain.Pattern{must(domain.NewGlob("*.txt"))}, nil, false))
	look := func() domain.Snapshot {
		t.Helper()
		matches, err := disk.Scanner{}.Scan(list)
		if err != nil {
			t.Fatal(err)
		}
		return domain.NewSnapshot(list, matches)
	}
	before := look()
	if !look().Same(before) {
		t.Fatal("two looks at files that did not change differ")
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes("a.txt", later, later); err != nil {
		t.Fatal(err)
	}
	if c := look().ChangeSince(before); !slices.Equal(c.Modified, []string{"a.txt"}) {
		t.Errorf("after a.txt was touched, the change is %+v, want a.txt modified", c)
	}
}

func TestScanNothingMatches(t *testing.T) {
	list := must(domain.NewWatchlist([]domain.Pattern{must(domain.NewGlob("a.txt"))}, nil, false))
	if _, err := (disk.Scanner{}).Scan(list); err != nil {
		t.Errorf("a glob that matches nothing failed the look: %v", err)
	}
}
