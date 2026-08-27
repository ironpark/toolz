//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

// isolateProcess puts the agent in its own process group so cancelling a turn
// can kill everything it started. An agent CLI is usually a shell script or a
// launcher that spawns the real process; signalling only the process mohae
// spawned would leave that child running — and holding the pipe mohae is
// waiting on — for the rest of the run.
func isolateProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		// A negative pid is the group: the agent and everything under it.
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
}
