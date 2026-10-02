package domain

import "time"

// Watch is one watch going on. It decides when the command runs, from the
// files at each look, what the person watching asks for, and whether a run
// is still going. It holds no timer and no process: the watch use case
// looks, runs and stops, and asks Watch what is due.
type Watch struct {
	schedule Schedule
	ran      Snapshot  // the files of the last run, or at the start
	seen     Snapshot  // the files at the last look
	changed  time.Time // when seen last changed
	paused   bool
}

// NewWatch starts a watch on schedule, its files at the start found at now.
func NewWatch(schedule Schedule, start Snapshot, now time.Time) *Watch {
	return &Watch{schedule: schedule, ran: start, seen: start, changed: now}
}

// Begin returns the change the first run is made for, unless the schedule
// postpones it to the first change.
func (w *Watch) Begin() (Change, bool) {
	if w.schedule.postpone {
		return Change{}, false
	}
	return w.ran.ChangeSince(w.ran), true
}

// Look takes in cur, the files at a look made at now, while a run goes on or
// not, and returns the change a run is due for, if one is: the files differ
// from those of the last run, have stayed as they are for the debounce, the
// watch is not paused, and no run is going on, or one is but the schedule
// stops a run a change makes stale. A change not run now stays due, and is
// run at a later look.
func (w *Watch) Look(cur Snapshot, now time.Time, running bool) (Change, bool) {
	if !cur.Same(w.seen) {
		w.seen, w.changed = cur, now
	}
	if w.paused || cur.Same(w.ran) || now.Sub(w.changed) < w.schedule.debounce || running && !w.schedule.stopStale {
		return Change{}, false
	}
	return w.runFor(cur), true
}

// RunNow returns the change a run asked for is made for, cur being the files
// now. It is due whatever the schedule says, and even while paused: the
// person watching asked for it.
func (w *Watch) RunNow(cur Snapshot) Change {
	return w.runFor(cur)
}

// TogglePause pauses the watch, or goes on with it, and reports whether it
// is paused now. While paused, changes wait, so several files can be edited
// before one run sees them all.
func (w *Watch) TogglePause() bool {
	w.paused = !w.paused
	return w.paused
}

// runFor records a run for cur, and returns the change it is made for.
func (w *Watch) runFor(cur Snapshot) Change {
	c := cur.ChangeSince(w.ran)
	w.ran = cur
	return c
}
