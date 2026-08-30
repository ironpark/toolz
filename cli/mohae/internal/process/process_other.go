//go:build !unix

package process

import "os/exec"

// isolateProcess leaves the default behaviour in place where process groups are
// not available: cancelling a turn kills the process mohae spawned, and
// WaitDelay stops the turn from hanging on a pipe a surviving child holds open.
func Isolate(command *exec.Cmd) {}
