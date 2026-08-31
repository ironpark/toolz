package skill

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// tarball writes a .tar.gz whose entries are the given name/content pairs, so a
// test can describe an archive without carrying a binary fixture.
func tarball(t *testing.T, entries map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bundle.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	zipped := gzip.NewWriter(file)
	writer := tar.NewWriter(zipped)
	for name, content := range entries {
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zipped.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractRefusesAnEntryThatEscapesTheDestination(t *testing.T) {
	archive := tarball(t, map[string]string{"../escaped.txt": "no"})
	dir := t.TempDir()
	if err := extract(archive, "bundle.tar.gz", dir); err == nil {
		t.Fatal("extract() = nil error, want a refusal")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escaped.txt")); err == nil {
		t.Fatal("the entry was written outside the destination")
	}
}

func TestSafeJoinRejectsASiblingSharingThePrefix(t *testing.T) {
	// "skills-evil" starts with "skills" but is not inside it; a prefix check
	// without the separator would let it through.
	if _, err := safeJoin(filepath.Join("/tmp", "skills"), "../skills-evil/x"); err == nil {
		t.Fatal("safeJoin() = nil error, want a refusal")
	}
}

func TestSafeJoinRejectsAnAbsoluteEntry(t *testing.T) {
	if _, err := safeJoin(t.TempDir(), "/etc/passwd"); err == nil {
		t.Fatal("safeJoin() = nil error, want a refusal")
	}
}

func TestUnpackStripsTheWrappingDirectoryHostsAdd(t *testing.T) {
	archive := tarball(t, map[string]string{
		"repo-abc123/" + Manifest:            "---\nname: x\n---\n",
		"repo-abc123/skills/one/" + Manifest: "---\nname: one\n---\n",
	})
	dir := t.TempDir()
	if err := unpack(archive, "bundle.tar.gz", dir); err != nil {
		t.Fatal(err)
	}
	// The manifest is at the root, not one level down under repo-abc123.
	if _, err := os.Stat(filepath.Join(dir, Manifest)); err != nil {
		t.Fatalf("wrapping directory was not stripped: %v", err)
	}
}
