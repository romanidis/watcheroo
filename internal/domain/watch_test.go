package domain_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/romanidis/watcheroo/internal/domain"
)

// files returns a snapshot of *.txt that found each of paths stamped at s.
func files(s int, paths ...string) domain.Snapshot {
	var matches []domain.Match
	for _, path := range paths {
		matches = append(matches, domain.Match{Path: path, Pattern: 0, Stamp: stamp(s)})
	}
	return domain.NewSnapshot(watchlist("*.txt"), matches)
}

// at is second s of the watch.
func at(s float64) time.Time {
	return time.Unix(1000, 0).Add(time.Duration(s * float64(time.Second)))
}

func schedule(o domain.ScheduleOptions) domain.Schedule {
	if o.Mode == "" {
		o.Mode = domain.ModeClear
	}
	if o.Interval == 0 {
		o.Interval = time.Second
	}
	return must(domain.NewSchedule(o))
}

func TestNewScheduleErrors(t *testing.T) {
	tests := []struct {
		name string
		o    domain.ScheduleOptions
		want error
	}{
		{name: "a zero interval", o: domain.ScheduleOptions{Mode: domain.ModeClear}, want: domain.ErrInterval},
		{name: "a negative debounce", o: domain.ScheduleOptions{Mode: domain.ModeClear, Interval: time.Second, Debounce: -1}, want: domain.ErrDebounce},
		{name: "a negative timeout", o: domain.ScheduleOptions{Mode: domain.ModeClear, Interval: time.Second, Timeout: -1}, want: domain.ErrTimeout},
		{name: "restart in diff mode", o: domain.ScheduleOptions{Mode: domain.ModeDiff, Interval: time.Second, Restart: true}, want: domain.ErrRestartInDiff},
		{name: "a timeout with restart", o: domain.ScheduleOptions{Mode: domain.ModeClear, Interval: time.Second, Restart: true, Timeout: time.Second}, want: domain.ErrTimeoutWithRestart},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := domain.NewSchedule(tt.o); !errors.Is(err, tt.want) {
				t.Errorf("NewSchedule returned %v, want %v", err, tt.want)
			}
		})
	}
}

func TestWatchBegin(t *testing.T) {
	w := domain.NewWatch(schedule(domain.ScheduleOptions{}), files(1, "a.txt"), at(0))
	c, ok := w.Begin()
	if !ok || !slices.Equal(c.Files, []string{"a.txt"}) || c.Added != nil || c.Modified != nil {
		t.Errorf("began with %+v, %v, want a run for a.txt, with nothing changed", c, ok)
	}
	postponed := domain.NewWatch(schedule(domain.ScheduleOptions{Postpone: true}), files(1, "a.txt"), at(0))
	if _, ok := postponed.Begin(); ok {
		t.Error("began with a run, want it postponed to the first change")
	}
	if c, ok := postponed.Look(files(2, "a.txt"), at(1), false); !ok || !slices.Equal(c.Modified, []string{"a.txt"}) {
		t.Errorf("after the first change, ran with %+v, %v, want a.txt modified", c, ok)
	}
}

func TestWatchLook(t *testing.T) {
	type look struct {
		files   domain.Snapshot
		at      float64
		running bool
		want    []string // the files of the run due, or nil for none
	}
	tests := []struct {
		name  string
		o     domain.ScheduleOptions
		looks []look
	}{
		{
			name: "a change runs, and nothing more until the next",
			looks: []look{
				{files: files(1, "a.txt"), at: 1, want: nil},
				{files: files(2, "a.txt"), at: 2, want: []string{"a.txt"}},
				{files: files(2, "a.txt"), at: 3, want: nil},
				{files: files(2, "a.txt", "b.txt"), at: 4, want: []string{"a.txt", "b.txt"}},
			},
		},
		{
			name: "a change waits for the files to stay as they are for the debounce",
			o:    domain.ScheduleOptions{Debounce: time.Second},
			looks: []look{
				{files: files(2, "a.txt"), at: 1, want: nil},
				{files: files(3, "a.txt"), at: 1.5, want: nil},
				{files: files(3, "a.txt"), at: 2, want: nil},
				{files: files(3, "a.txt"), at: 2.5, want: []string{"a.txt"}},
			},
		},
		{
			name: "a change waits for the run going on to end",
			looks: []look{
				{files: files(2, "a.txt"), at: 1, running: true, want: nil},
				{files: files(2, "a.txt"), at: 2, running: false, want: []string{"a.txt"}},
			},
		},
		{
			name: "restart stops the run going on for a change",
			o:    domain.ScheduleOptions{Restart: true},
			looks: []look{
				{files: files(2, "a.txt"), at: 1, running: true, want: []string{"a.txt"}},
			},
		},
		{
			name: "diff mode stops the run going on for a change",
			o:    domain.ScheduleOptions{Mode: domain.ModeDiff},
			looks: []look{
				{files: files(2, "a.txt"), at: 1, running: true, want: []string{"a.txt"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := domain.NewWatch(schedule(tt.o), files(1, "a.txt"), at(0))
			for i, l := range tt.looks {
				c, ok := w.Look(l.files, at(l.at), l.running)
				switch {
				case ok != (l.want != nil):
					t.Errorf("look %d: a run due: %v, want %v", i+1, ok, l.want != nil)
				case ok && !slices.Equal(c.Files, l.want):
					t.Errorf("look %d: a run due for %v, want %v", i+1, c.Files, l.want)
				}
			}
		})
	}
}

func TestWatchPause(t *testing.T) {
	w := domain.NewWatch(schedule(domain.ScheduleOptions{}), files(1, "a.txt"), at(0))
	if !w.TogglePause() {
		t.Fatal("not paused after the first toggle")
	}
	if _, ok := w.Look(files(2, "a.txt"), at(1), false); ok {
		t.Error("a change ran while paused")
	}
	if c := w.RunNow(files(2, "a.txt")); !slices.Equal(c.Modified, []string{"a.txt"}) {
		t.Errorf("a run asked for while paused was made for %+v, want a.txt modified", c)
	}
	if w.TogglePause() {
		t.Fatal("still paused after the second toggle")
	}
	if c, ok := w.Look(files(3, "a.txt"), at(2), false); !ok || !slices.Equal(c.Modified, []string{"a.txt"}) {
		t.Errorf("after going on, a change ran with %+v, %v, want a.txt modified", c, ok)
	}
}

func TestWatchChangeWaitsWhilePaused(t *testing.T) {
	w := domain.NewWatch(schedule(domain.ScheduleOptions{}), files(1, "a.txt"), at(0))
	w.TogglePause()
	w.Look(files(1, "a.txt", "b.txt"), at(1), false)
	w.TogglePause()
	if c, ok := w.Look(files(1, "a.txt", "b.txt"), at(2), false); !ok || !slices.Equal(c.Added, []string{"b.txt"}) {
		t.Errorf("going on after a change while paused ran with %+v, %v, want b.txt added", c, ok)
	}
}
