//go:build unix

package process

import (
	"os/exec"
	"syscall"
)

// Isolate puts a subprocess in its own process group so cancellation can kill
// everything it started. Agent CLIs and shell hooks often spawn the real work
// in children; signalling only the parent would leave those children running.
func Isolate(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		// A negative pid is the group: the agent and everything under it.
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
}
