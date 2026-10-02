//go:build unix

package process

import (
	"os/exec"
	"syscall"
	"time"
)

// stopGrace is how long a command stopped by stopGroup has to end on its own
// before it is killed.
const stopGrace = 5 * time.Second

// stopGroup starts cmd in a process group of its own, and makes the end of
// its context send SIGTERM to that whole group rather than kill cmd alone.
// That reaches the processes cmd started too, such as the server go run
// builds and runs, which would otherwise go on holding its port. cmd is
// killed if it is still running stopGrace later.
//
// Every command runs this way, as a watch may stop any of them: on a change
// with --restart or in diff mode, after --timeout, or when s is pressed. A
// command outside the terminal's process group cannot use the terminal, so
// one that opens a pager hangs; it leaves the keys to wtr.
func stopGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = stopGrace
}
