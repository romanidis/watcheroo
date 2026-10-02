//go:build !unix

package process

import "os/exec"

// stopGroup leaves cmd as it is where there are no process groups: the end of
// its context kills cmd, and only cmd.
func stopGroup(cmd *exec.Cmd) {}
