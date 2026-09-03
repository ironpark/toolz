package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileUniqueDoesNotReplaceAnExistingFile(t *testing.T) {
	directory := t.TempDir()
	first, err := WriteFileUnique(directory, "report", ".json", []byte("first"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	second, err := WriteFileUnique(directory, "report", ".json", []byte("second"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(first) != "report.json" || filepath.Base(second) != "report-2.json" {
		t.Fatalf("created %q and %q", first, second)
	}
	data, err := os.ReadFile(first)
	if err != nil || string(data) != "first" {
		t.Fatalf("first file = %q, %v", data, err)
	}
}

func TestWriteFileUniqueRemovesAnIncompleteFile(t *testing.T) {
	directory := t.TempDir()
	wantErr := errors.New("write failed")
	path, err := writeFileUnique(directory, "report", ".json", 0o644, func(file *os.File) error {
		if _, err := file.WriteString("partial"); err != nil {
			return err
		}
		return wantErr
	})
	if path != "" || !errors.Is(err, wantErr) {
		t.Fatalf("writeFileUnique() = %q, %v", path, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("incomplete output remains: %v", entries)
	}
}

func TestMkdirUniqueKeepsTheSuffixAfterACollision(t *testing.T) {
	directory := t.TempDir()
	first, err := MkdirUnique(directory, "trial", ".artifacts", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MkdirUnique(directory, "trial", ".artifacts", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(first) != "trial.artifacts" || filepath.Base(second) != "trial-2.artifacts" {
		t.Fatalf("created %q and %q", first, second)
	}
}
