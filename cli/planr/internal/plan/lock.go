package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	LockFileName      = ".planr.lock"
	planLockPollDelay = 25 * time.Millisecond
)

var LockTimeout = 2 * time.Second

type Lock struct {
	file *os.File
	path string
	kind string
}

// AcquireLock opens the lock file inside an existing plan directory. The
// directory is intentionally not created here: if a plan was moved while a
// command was waiting, recreating its old path would be worse than reporting a
// clear failure.
func AcquireLock(planRoot string) (*Lock, error) {
	return acquireAdvisoryLock(planRoot, "plan", false)
}

// AcquireDirectoryLock serializes operations that add or move plan
// directories. Unlike a plan lock, its parent may not exist yet.
func AcquireDirectoryLock(plansRoot string) (*Lock, error) {
	return acquireAdvisoryLock(plansRoot, "plans directory", true)
}

func acquireAdvisoryLock(directory, kind string, createParent bool) (*Lock, error) {
	if createParent {
		if err := os.MkdirAll(directory, 0755); err != nil {
			return nil, fmt.Errorf("cannot prepare %s lock directory %s: %w", kind, directory, err)
		}
	}
	path := filepath.Join(directory, LockFileName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s lock %s: %w", kind, path, err)
	}
	lock := &Lock{file: file, path: path, kind: kind}
	deadline := time.Now().Add(LockTimeout)
	for {
		if err := tryAdvisoryLock(file); err == nil {
			return lock, nil
		} else if !advisoryLockBusy(err) {
			_ = file.Close()
			return nil, fmt.Errorf("cannot acquire %s lock %s: %w", kind, path, err)
		}
		if time.Now().After(deadline) {
			_ = file.Close()
			return nil, fmt.Errorf("cannot acquire %s lock %s within %s; another planr process may be changing this state", kind, path, LockTimeout)
		}
		time.Sleep(planLockPollDelay)
	}
}

func (lock *Lock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := releaseAdvisoryLock(lock.file)
	closeErr := lock.file.Close()
	if unlockErr != nil {
		return fmt.Errorf("release %s lock %s: %w", lock.kind, lock.path, unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s lock %s: %w", lock.kind, lock.path, closeErr)
	}
	return nil
}
