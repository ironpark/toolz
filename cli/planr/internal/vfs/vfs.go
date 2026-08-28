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
// the io/fs name it has in the FS below, and path reverses the translation.
//
// A filesystem swapped in through Use has to report a missing file the way the
// os package does — as an error satisfying errors.Is(err, fs.ErrNotExist) —
// because callers treat a missing plans directory as an empty one.
package vfs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FS is the filesystem planr reads. Any fs.FS will do: the helpers below go
// through fs.ReadFile, fs.ReadDir and fs.Stat, which use the optional io/fs
// interfaces when the filesystem implements them. Keeping it to fs.FS is what
// lets a test hand in an embed.FS or an fs.Sub of one.
type FS = fs.FS

// current is the filesystem the package-level helpers read through.
var current FS = hostFS{}

// Use swaps in a filesystem and returns a function restoring the previous one.
// Tests use it; production code never calls it.
func Use(fsys FS) func() {
	previous := current
	current = fsys
	return func() { current = previous }
}

// Name converts a host path into its io/fs name. Relative paths are resolved
// against the working directory first, so the name does not depend on where
// the process happens to be.
func Name(hostPath string) (string, error) {
	absolute, err := filepath.Abs(hostPath)
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
		return "", fmt.Errorf("%s: not a readable path", hostPath)
	}
	return name, nil
}

// path reverses Name, turning an io/fs name back into a host path.
func path(name string) string {
	host := filepath.FromSlash(name)
	if filepath.VolumeName(host) != "" {
		return host
	}
	return string(filepath.Separator) + host
}

// ReadFile reads the file at a host path.
func ReadFile(hostPath string) ([]byte, error) {
	if _, ok := current.(hostFS); ok {
		return os.ReadFile(hostPath)
	}
	name, err := Name(hostPath)
	if err != nil {
		return nil, err
	}
	return fs.ReadFile(current, name)
}

// ReadDir lists the directory at a host path.
func ReadDir(hostPath string) ([]fs.DirEntry, error) {
	if _, ok := current.(hostFS); ok {
		return os.ReadDir(hostPath)
	}
	name, err := Name(hostPath)
	if err != nil {
		return nil, err
	}
	return fs.ReadDir(current, name)
}

// Stat reports the file information for a host path.
func Stat(hostPath string) (fs.FileInfo, error) {
	if _, ok := current.(hostFS); ok {
		return os.Stat(hostPath)
	}
	name, err := Name(hostPath)
	if err != nil {
		return nil, err
	}
	return fs.Stat(current, name)
}

// hostFS is the machine's filesystem seen through io/fs, and the default the
// helpers above short-circuit to: a host path needs no translation to reach
// the os package, which is also what keeps its errors — and so os.IsNotExist
// — intact for callers.
type hostFS struct{}

func (hostFS) Open(name string) (fs.File, error)          { return os.Open(path(name)) }
func (hostFS) ReadFile(name string) ([]byte, error)       { return os.ReadFile(path(name)) }
func (hostFS) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(path(name)) }
func (hostFS) Stat(name string) (fs.FileInfo, error)      { return os.Stat(path(name)) }
