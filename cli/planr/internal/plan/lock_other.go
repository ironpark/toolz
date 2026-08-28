//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package plan

import (
	"errors"
	"os"
)

func tryAdvisoryLock(_ *os.File) error {
	return errors.New("advisory locking is not supported on this platform")
}

func releaseAdvisoryLock(_ *os.File) error {
	return nil
}

func advisoryLockBusy(_ error) bool {
	return false
}
