package domain

import "time"

// Ending is how a run ended. The set is closed.
type Ending string

const (
	EndingOK       Ending = "ok"        // the command succeeded
	EndingExit     Ending = "exit"      // it ended with an exit status other than 0
	EndingError    Ending = "error"     // it ended without an exit status, or did not start
	EndingTimedOut Ending = "timed out" // it ran longer than the timeout, and was stopped
	EndingStopped  Ending = "stopped"   // the watch stopped it: a change, a key, or the end
)

// Outcome is how one run of the command ended, and how long it took.
type Outcome struct {
	ending  Ending
	code    int
	reason  string
	timeout time.Duration
	took    time.Duration
}

// OutcomeOK is a run that succeeded.
func OutcomeOK(took time.Duration) Outcome {
	return Outcome{ending: EndingOK, took: took}
}

// OutcomeExit is a run that ended with exit status code.
func OutcomeExit(code int, took time.Duration) Outcome {
	return Outcome{ending: EndingExit, code: code, took: took}
}

// OutcomeError is a run that ended without an exit status, or never
// started, for reason.
func OutcomeError(reason string, took time.Duration) Outcome {
	return Outcome{ending: EndingError, reason: reason, took: took}
}

// OutcomeTimedOut is a run stopped once it ran longer than timeout.
func OutcomeTimedOut(timeout, took time.Duration) Outcome {
	return Outcome{ending: EndingTimedOut, timeout: timeout, took: took}
}

// OutcomeStopped is a run the watch stopped.
func OutcomeStopped(took time.Duration) Outcome {
	return Outcome{ending: EndingStopped, took: took}
}

func (o Outcome) Ending() Ending         { return o.ending }
func (o Outcome) Code() int              { return o.code }
func (o Outcome) Reason() string         { return o.reason }
func (o Outcome) Timeout() time.Duration { return o.timeout }
func (o Outcome) Took() time.Duration    { return o.took }

// Failed reports whether the run went wrong. A run the watch stopped did not:
// it was not let finish.
func (o Outcome) Failed() bool {
	return o.ending == EndingExit || o.ending == EndingError || o.ending == EndingTimedOut
}

// Finished reports whether the run got to the end, so that its output is all
// it would print.
func (o Outcome) Finished() bool {
	return o.ending != EndingStopped && o.ending != EndingTimedOut
}
