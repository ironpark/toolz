package vfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

func TestReadsAndWritesGoThroughTheInjectedFilesystem(t *testing.T) {
	memory := afero.NewMemMapFs()
	defer Use(memory)()

	root := filepath.Join(string(filepath.Separator), "repo", "plans")
	if err := MkdirAll(root, 0755); err != nil {
		t.Fatalf("MkdirAll() unexpected error: %v", err)
	}
	path := filepath.Join(root, "PLAN.md")
	if err := WriteFile(path, []byte("in memory\n"), 0644); err != nil {
		t.Fatalf("WriteFile() unexpected error: %v", err)
	}
	contents, err := ReadFile(path)
	if err != nil || string(contents) != "in memory\n" {
		t.Fatalf("ReadFile() = %q, %v", contents, err)
	}
	entries, err := ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir() unexpected error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "PLAN.md" {
		t.Fatalf("ReadDir() = %#v, want one PLAN.md entry", entries)
	}
	if IsOS() {
		t.Fatal("IsOS() is true while a memory filesystem is installed")
	}
	// Nothing reached the machine's filesystem.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("os.Stat(%s) error = %v, want a not-exist error", path, err)
	}
}

func TestHostFilesystemKeepsOSErrorSemantics(t *testing.T) {
	if !IsOS() {
		t.Fatal("IsOS() is false by default")
	}
	root := t.TempDir()
	path := filepath.Join(root, "PLAN.md")
	if err := WriteFile(path, []byte("on disk\n"), 0644); err != nil {
		t.Fatalf("WriteFile() unexpected error: %v", err)
	}
	contents, err := ReadFile(path)
	if err != nil || string(contents) != "on disk\n" {
		t.Fatalf("ReadFile() = %q, %v", contents, err)
	}
	// Callers branch on os.IsNotExist to treat a missing plans directory as an
	// empty one, so both filesystems have to keep reporting it that way.
	if _, err := ReadDir(filepath.Join(root, "absent")); !os.IsNotExist(err) {
		t.Fatalf("ReadDir(absent) error = %v, want a not-exist error", err)
	}
	defer Use(afero.NewMemMapFs())()
	if _, err := ReadDir(filepath.Join(root, "absent")); !os.IsNotExist(err) {
		t.Fatalf("ReadDir(absent) on memory error = %v, want a not-exist error", err)
	}
}
