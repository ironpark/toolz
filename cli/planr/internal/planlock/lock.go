package planlock

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ironpark/toolz/cli/planr/internal/vfs"
)

const (
	FileName = ".planr.lock"

	// DefaultTimeout bounds how long an acquire waits for a competing planr
	// process to release the lock before reporting a clear failure.
	DefaultTimeout = 2 * time.Second

	planLockPollDelay = 25 * time.Millisecond
)

type Lock struct {
	file *os.File
	path string
	kind string
}

// AcquirePlan opens the lock file inside an existing plan directory. The
// directory is intentionally not created here: if a plan was moved while a
// command was waiting, recreating its old path would be worse than reporting a
// clear failure.
func AcquirePlan(planRoot string) (*Lock, error) {
	return acquireAdvisoryLock(planRoot, "plan", false, DefaultTimeout)
}

// AcquireDirectory serializes operations that add or move plan
// directories. Unlike a plan lock, its parent may not exist yet.
func AcquireDirectory(plansRoot string) (*Lock, error) {
	return acquireAdvisoryLock(plansRoot, "plans directory", true, DefaultTimeout)
}

func acquireAdvisoryLock(directory, kind string, createParent bool, timeout time.Duration) (*Lock, error) {
	if createParent {
		if err := vfs.MkdirAll(directory, 0755); err != nil {
			return nil, fmt.Errorf("cannot prepare %s lock directory %s: %w", kind, directory, err)
		}
	}
	path := filepath.Join(directory, FileName)
	// An advisory lock is a flock on a real descriptor, which a filesystem
	// swapped in through vfs cannot provide. Such a filesystem belongs to a
	// single test process, where there is no competing planr to exclude, so the
	// lock is skipped rather than faked. Creating the directory above still
	// happens, because callers rely on it.
	if !vfs.IsOS() {
		return &Lock{path: path, kind: kind}, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s lock %s: %w", kind, path, err)
	}
	lock := &Lock{file: file, path: path, kind: kind}
	deadline := time.Now().Add(timeout)
	for {
		if err := tryAdvisoryLock(file); err == nil {
			return lock, nil
		} else if !advisoryLockBusy(err) {
			_ = file.Close()
			return nil, fmt.Errorf("cannot acquire %s lock %s: %w", kind, path, err)
		}
		if time.Now().After(deadline) {
			_ = file.Close()
			return nil, fmt.Errorf("cannot acquire %s lock %s within %s; another planr process may be changing this state", kind, path, timeout)
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
