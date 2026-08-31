package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSkill(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, Manifest), []byte("---\nname: x\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverTreatsATreeThatIsItselfASkillAsTheOnlyOne(t *testing.T) {
	root := filepath.Join(t.TempDir(), "commit")
	writeSkill(t, root)
	// Would be found by the directory search if the root were not checked first.
	writeSkill(t, filepath.Join(root, "skills", "other"))

	found, err := discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Name != "commit" {
		t.Fatalf("discover() = %+v", found)
	}
}

func TestDiscoverReturnsEverySkillARepositoryPublishes(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "skills", "beta"))
	writeSkill(t, filepath.Join(root, "skills", "alpha"))
	writeSkill(t, filepath.Join(root, ".claude", "skills", "gamma"))
	// No manifest, so not a skill.
	if err := os.MkdirAll(filepath.Join(root, "skills", "notaskill"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := discover(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range found {
		names = append(names, entry.Name)
	}
	// Sorted, so two machines install the same set in the same order.
	want := []string{"alpha", "beta", "gamma"}
	if len(names) != len(want) {
		t.Fatalf("discover() = %v, want %v", names, want)
	}
	for index := range want {
		if names[index] != want[index] {
			t.Fatalf("discover() = %v, want %v", names, want)
		}
	}
}

func TestDiscoverPrefersTheShallowestCopyOfADuplicatedSkill(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "skills", "commit"))
	writeSkill(t, filepath.Join(root, ".claude", "skills", "commit"))

	found, err := discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("discover() = %+v, want one entry", found)
	}
	if want := filepath.Join(root, "skills", "commit"); found[0].Dir != want {
		t.Errorf("dir = %q, want %q", found[0].Dir, want)
	}
}

func TestDiscoverSaysWhereItLookedWhenThereIsNothing(t *testing.T) {
	if _, err := discover(t.TempDir()); err == nil {
		t.Fatal("discover() = nil error, want one naming the searched directories")
	}
}
