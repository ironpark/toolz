// Package vfs is planr's filesystem. Every document planr touches —
// configuration, plan directories, phase documents, drafts — is read and
// written through the afero.Fs held here, so a test can run a command against
// an in-memory tree instead of a temporary directory.
//
// Paths are ordinary host paths, the same ones planr resolves plan roots to
// and reports back to the user; afero takes them as-is, so nothing has to be
// translated on the way in or out. One difference is worth knowing: a relative
// path resolves against the process working directory on the machine's
// filesystem but against the root of an in-memory tree, so a test that swaps
// one in names its files absolutely.
//
// Two things stay on the os package because they are not file contents: the
// advisory plan locks in internal/planlock, which need a real descriptor to
// flock, and go-git's repository access. Both degrade explicitly rather than
// silently when a filesystem is swapped in — see planlock.AcquirePlan.
package vfs

import (
	"io/fs"
	"os"

	"github.com/spf13/afero"
)

// current is the filesystem the package-level helpers work through. The afero
// wrapper is what supplies ReadFile/WriteFile/ReadDir on top of the interface.
var current = &afero.Afero{Fs: afero.NewOsFs()}

// Use swaps in a filesystem and returns a function restoring the previous one.
// Tests use it; production code never calls it.
func Use(fsys afero.Fs) func() {
	previous := current
	current = &afero.Afero{Fs: fsys}
	return func() { current = previous }
}

// IsOS reports whether reads and writes reach the machine's filesystem. It is
// how the parts that cannot go through afero — advisory locking, git — decide
// whether they still have a real file to work with.
func IsOS() bool {
	_, ok := current.Fs.(*afero.OsFs)
	return ok
}

// ReadFile returns the contents of a file.
func ReadFile(path string) ([]byte, error) { return current.ReadFile(path) }

// ReadDir lists a directory, sorted by filename.
func ReadDir(path string) ([]fs.FileInfo, error) { return current.ReadDir(path) }

// Stat reports the file information for a path.
func Stat(path string) (fs.FileInfo, error) { return current.Stat(path) }

// WriteFile writes a file, creating it when it does not exist.
func WriteFile(path string, contents []byte, mode os.FileMode) error {
	return current.WriteFile(path, contents, mode)
}

// MkdirAll creates a directory and any missing parent.
func MkdirAll(path string, mode os.FileMode) error { return current.MkdirAll(path, mode) }

// MkdirTemp creates a uniquely named directory inside dir.
func MkdirTemp(dir, prefix string) (string, error) { return current.TempDir(dir, prefix) }

// CreateTemp creates a uniquely named file inside dir, open for writing.
func CreateTemp(dir, prefix string) (afero.File, error) { return current.TempFile(dir, prefix) }

// Rename moves a file or directory.
func Rename(oldPath, newPath string) error { return current.Rename(oldPath, newPath) }

// Chmod changes a file's mode.
func Chmod(path string, mode os.FileMode) error { return current.Chmod(path, mode) }

// Remove deletes a file or an empty directory.
func Remove(path string) error { return current.Remove(path) }

// RemoveAll deletes a path and anything beneath it.
func RemoveAll(path string) error { return current.RemoveAll(path) }
