//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package planlock

import (
	"errors"
	"os"
	"syscall"
)

func tryAdvisoryLock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func releaseAdvisoryLock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}

func advisoryLockBusy(err error) bool {
	return errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK)
}
