//go:build !unix

package process

import "os/exec"

// Isolate is a no-op where process groups are not available: cancelling a turn
// kills only the process mohae spawned, so callers must bound their own waits
// to avoid hanging on a pipe a surviving child holds open.
func Isolate(command *exec.Cmd) {}
