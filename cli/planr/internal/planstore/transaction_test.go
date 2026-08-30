package planstore

import (
	"errors"
	"testing"

	"github.com/ironpark/toolz/cli/planr/internal/vfs"
	"github.com/spf13/afero"
)

type failingFS struct {
	afero.Fs
	renameTarget string
	removeTarget string
}

func (f *failingFS) Rename(oldPath, newPath string) error {
	if newPath == f.renameTarget {
		return errors.New("injected rename failure")
	}
	return f.Fs.Rename(oldPath, newPath)
}

func (f *failingFS) Remove(path string) error {
	if path == f.removeTarget {
		return errors.New("injected remove failure")
	}
	return f.Fs.Remove(path)
}

func TestApplyRollsBackEarlierUpdate(t *testing.T) {
	fsys := afero.NewMemMapFs()
	writeFixture(t, fsys, "/plan/phase.md", "old phase")
	writeFixture(t, fsys, "/plan/PLAN.md", "old plan")
	restore := vfs.Use(&failingFS{Fs: fsys, renameTarget: "/plan/PLAN.md"})
	t.Cleanup(restore)

	err := Apply(
		Update("/plan/phase.md", "old phase", "new phase"),
		Update("/plan/PLAN.md", "old plan", "new plan"),
	)
	if err == nil {
		t.Fatal("Apply() succeeded, want injected failure")
	}
	assertContents(t, fsys, "/plan/phase.md", "old phase")
	assertContents(t, fsys, "/plan/PLAN.md", "old plan")
}

func TestApplyRemovesCreatedFileOnRollback(t *testing.T) {
	fsys := afero.NewMemMapFs()
	writeFixture(t, fsys, "/plan/PLAN.md", "old plan")
	restore := vfs.Use(&failingFS{Fs: fsys, renameTarget: "/plan/PLAN.md"})
	t.Cleanup(restore)

	err := Apply(
		Create("/plan/phase.md", "new phase"),
		Update("/plan/PLAN.md", "old plan", "new plan"),
	)
	if err == nil {
		t.Fatal("Apply() succeeded, want injected failure")
	}
	if exists, statErr := afero.Exists(fsys, "/plan/phase.md"); statErr != nil || exists {
		t.Fatalf("created phase exists after rollback=%v, err=%v", exists, statErr)
	}
}

func TestApplyRestoresUpdateWhenDeleteFails(t *testing.T) {
	fsys := afero.NewMemMapFs()
	writeFixture(t, fsys, "/plan/phase.md", "old phase")
	writeFixture(t, fsys, "/plan/PLAN.md", "old plan")
	restore := vfs.Use(&failingFS{Fs: fsys, removeTarget: "/plan/phase.md"})
	t.Cleanup(restore)

	err := Apply(
		Update("/plan/PLAN.md", "old plan", "new plan"),
		Delete("/plan/phase.md", "old phase"),
	)
	if err == nil {
		t.Fatal("Apply() succeeded, want injected failure")
	}
	assertContents(t, fsys, "/plan/PLAN.md", "old plan")
	assertContents(t, fsys, "/plan/phase.md", "old phase")
}

func writeFixture(t *testing.T, fsys afero.Fs, path, contents string) {
	t.Helper()
	if err := fsys.MkdirAll("/plan", 0755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fsys, path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}

func assertContents(t *testing.T, fsys afero.Fs, path, want string) {
	t.Helper()
	contents, err := afero.ReadFile(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != want {
		t.Fatalf("%s contents = %q, want %q", path, contents, want)
	}
}
