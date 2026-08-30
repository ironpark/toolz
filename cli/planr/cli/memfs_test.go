package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/planr/internal/vfs"
	"github.com/spf13/afero"
)

// planr's documents all move through internal/vfs, so a command can be run
// against a filesystem that never touches the disk. This is the check that the
// seam is complete: an `apply` that writes and a `status` that reads it back
// have to agree, which they only do if every path in between goes through vfs.
func TestApplyAndStatusRunOnAnInMemoryFilesystem(t *testing.T) {
	memory := afero.NewMemMapFs()
	defer vfs.Use(memory)()

	// Two things stay on the disk because they are not documents: the working
	// directory, which the process cannot fake, and the git repository planr
	// insists on. Everything planr itself writes lives in memory.
	// planr resolves the repository through the working directory, which the
	// temporary directory reaches by symlink on macOS; the paths it writes are
	// the resolved ones, so the test names them the same way.
	root, err := filepath.EvalSymlinks(seedRepository(t))
	if err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, root)
	writeConfig(t, root, "language: en\nplans_dir: plans\n")
	draftPath := writeDraft(t, root, "checkout-v2", nil)

	// The draft is named absolutely: a relative path resolves against the
	// process working directory on the disk and against the root of an
	// in-memory tree, and only the disk knows where the test is standing.
	if output, err := runRoot(t, "apply", draftPath, "--no-hooks"); err != nil {
		t.Fatalf("apply: %v; output=%q", err, output)
	}
	status, err := runRoot(t, "status")
	if err != nil {
		t.Fatalf("status: %v; output=%q", err, status)
	}
	if !strings.Contains(status, "00-checkout-v2: in-progress (0/3 phases done)") {
		t.Fatalf("status does not report the registered plan:\n%s", status)
	}

	// The plan exists in memory and nowhere else.
	if _, err := vfs.Stat(filepath.Join(root, "plans", "00-checkout-v2", "PLAN.md")); err != nil {
		t.Fatalf("PLAN.md is missing from the memory filesystem: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "plans")); !os.IsNotExist(err) {
		t.Fatalf("apply wrote to the disk; stat error = %v", err)
	}
}
