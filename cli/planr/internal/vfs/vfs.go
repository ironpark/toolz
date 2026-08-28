// Package vfs is the read side of planr's filesystem access. Every document
// planr inspects — configuration, plan directories, phase documents, drafts —
// is read through the io/fs interface held here, so a test can hand the
// commands an in-memory tree instead of building a temporary directory.
//
// Writes, locks and git access stay on the os package: io/fs is read-only, and
// pretending otherwise would only hide where planr mutates the repository.
//
// Callers keep passing host paths, because planr resolves plan roots as host
// paths and reports them back to the user. Name translates a host path into
// the io/fs name it has in the FS below, and Host reverses the translation.
package vfs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FS is the filesystem planr reads. The optional io/fs interfaces are required
// rather than optional so an injected filesystem cannot silently fall back to
// the slower generic implementations for the operations planr leans on.
type FS interface {
	fs.FS
	fs.ReadFileFS
	fs.ReadDirFS
	fs.StatFS
}

// current is the filesystem the package-level helpers read through.
var current FS = hostFS{}

// Use swaps in a filesystem and returns a function restoring the previous one.
// Tests use it; production code never calls it.
func Use(fsys FS) func() {
	previous := current
	current = fsys
	return func() { current = previous }
}

// Current returns the filesystem reads go through.
func Current() FS { return current }

// Name converts a host path into its io/fs name. Relative paths are resolved
// against the working directory first, so the name does not depend on where
// the process happens to be.
func Name(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	name := filepath.ToSlash(absolute)
	if volume := filepath.VolumeName(absolute); volume == "" {
		name = strings.TrimPrefix(name, "/")
	}
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		name = "."
	}
	if !fs.ValidPath(name) {
		return "", fmt.Errorf("%s: not a readable path", path)
	}
	return name, nil
}

// Path reverses Name, turning an io/fs name back into a host path.
func Path(name string) string {
	host := filepath.FromSlash(name)
	if filepath.VolumeName(host) != "" {
		return host
	}
	return string(filepath.Separator) + host
}

// ReadFile reads the file at a host path.
func ReadFile(path string) ([]byte, error) {
	name, err := Name(path)
	if err != nil {
		return nil, err
	}
	return fs.ReadFile(current, name)
}

// ReadDir lists the directory at a host path.
func ReadDir(path string) ([]fs.DirEntry, error) {
	name, err := Name(path)
	if err != nil {
		return nil, err
	}
	return fs.ReadDir(current, name)
}

// Stat reports the file information for a host path.
func Stat(path string) (fs.FileInfo, error) {
	name, err := Name(path)
	if err != nil {
		return nil, err
	}
	return fs.Stat(current, name)
}

// hostFS is the machine's filesystem seen through io/fs. It keeps the os
// package's errors, so os.IsNotExist and friends still hold for callers.
type hostFS struct{}

func (hostFS) Open(name string) (fs.File, error)          { return os.Open(Path(name)) }
func (hostFS) ReadFile(name string) ([]byte, error)       { return os.ReadFile(Path(name)) }
func (hostFS) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(Path(name)) }
func (hostFS) Stat(name string) (fs.FileInfo, error)      { return os.Stat(Path(name)) }
