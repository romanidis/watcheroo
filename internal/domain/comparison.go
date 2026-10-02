package domain

// Comparison is what diff mode compares each run's output with: the output
// the baseline picks, and the output of the last run shown, which Rebase
// can make the one compared with.
type Comparison struct {
	baseline Baseline
	context  int // the lines kept around each change, or below 0 for all of them
	ran      bool
	against  string // the output the next run is compared with
	latest   string // the output of the last run shown
}

// NewComparison compares each run with the run baseline picks. With context
// at 0 or more, a Diff keeps only the lines that changed and context lines
// around each; below 0, it keeps every line.
func NewComparison(baseline Baseline, context int) *Comparison {
	return &Comparison{baseline: baseline, context: context}
}

// Compare returns how output, of a run that ended with o, differs from the
// output it is compared with, and takes output as the one to compare the
// next run with when the baseline says so. A run that did not finish is not
// compared, and leaves the baseline as it was: what it printed is not all it
// would have. The first run is compared with itself, so all of it shows and
// nothing is marked.
func (c *Comparison) Compare(output string, o Outcome) (Diff, bool) {
	if !o.Finished() {
		return Diff{}, false
	}
	prev, ran := c.against, c.ran
	if !ran || c.baseline == BaselinePrevious {
		c.against, c.ran = output, true
	}
	c.latest = output
	if !ran {
		return Diff{First: true, Baseline: c.baseline, Lines: markLines(output, output, -1)}, true
	}
	return Diff{
		Unchanged: prev == output,
		Baseline:  c.baseline,
		Lines:     markLines(prev, output, c.context),
	}, true
}

// Rebase makes the next run compared with the last one shown, as
// BaselineFirst compares every run with the first. It reports false, and
// does nothing, before any run was shown.
func (c *Comparison) Rebase() bool {
	if !c.ran {
		return false
	}
	c.against = c.latest
	return true
}
