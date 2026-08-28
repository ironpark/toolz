package vfs

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestReadsGoThroughTheInjectedFilesystem(t *testing.T) {
	root := t.TempDir()
	name, err := Name(filepath.Join(root, "PLAN.md"))
	if err != nil {
		t.Fatalf("Name() unexpected error: %v", err)
	}
	defer Use(fstest.MapFS{name: {Data: []byte("in memory\n")}})()

	contents, err := ReadFile(filepath.Join(root, "PLAN.md"))
	if err != nil {
		t.Fatalf("ReadFile() unexpected error: %v", err)
	}
	if string(contents) != "in memory\n" {
		t.Fatalf("ReadFile() = %q, want the injected contents", contents)
	}
	entries, err := ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir() unexpected error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "PLAN.md" {
		t.Fatalf("ReadDir() = %#v, want one PLAN.md entry", entries)
	}
	if _, err := Stat(filepath.Join(root, "PLAN.md")); err != nil {
		t.Fatalf("Stat() unexpected error: %v", err)
	}
}

func TestHostFilesystemKeepsOSErrorSemantics(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "PLAN.md")
	if err := os.WriteFile(path, []byte("on disk\n"), 0644); err != nil {
		t.Fatal(err)
	}
	contents, err := ReadFile(path)
	if err != nil || string(contents) != "on disk\n" {
		t.Fatalf("ReadFile() = %q, %v", contents, err)
	}
	// Callers branch on os.IsNotExist to treat a missing plans directory as an
	// empty one, so the host filesystem has to keep reporting it that way.
	if _, err := ReadDir(filepath.Join(root, "absent")); !os.IsNotExist(err) {
		t.Fatalf("ReadDir(absent) error = %v, want a not-exist error", err)
	}
}

func TestNameAndPathRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plans", "00-checkout-v2")
	name, err := Name(path)
	if err != nil {
		t.Fatalf("Name() unexpected error: %v", err)
	}
	if Path(name) != path {
		t.Fatalf("Path(Name(%q)) = %q", path, Path(name))
	}
}
