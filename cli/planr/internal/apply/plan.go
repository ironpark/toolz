package apply

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
)

// Plan registers a plan draft as a new numbered plan directory.
func Plan(d draft.Draft, settings config.Config, repoRoot string, dryRun, jsonOutput bool) (Operation, error) {
	planDirectories := settings.PlanDirs(repoRoot)
	if len(planDirectories) == 0 {
		return Operation{}, fmt.Errorf("no plans directory is configured")
	}
	if !dryRun {
		lock, err := plan.AcquireDirectoryLock(planDirectories[0])
		if err != nil {
			return Operation{}, err
		}
		defer lock.Close()
	}
	planDirectory, err := plan.NextDirectory(planDirectories, d.Name)
	if err != nil {
		return Operation{}, err
	}
	documents, err := plan.RenderDocuments(d, planDirectory, settings.Language, plan.CompletionTimestamp())
	if err != nil {
		return Operation{}, err
	}
	target := filepath.Join(planDirectories[0], planDirectory)
	op := makeOperation("register_plan", d.Name, dryRun, documentsWithRoot(target, documents), newDocumentDiffs(target, documents))
	if dryRun {
		return op, nil
	}
	if err := hooks.RunDocument(repoRoot, settings.Hooks, settings.SkipHooks, "before", hooks.EventAdd, planDirectory, -1, "registered", jsonOutput); err != nil {
		return Operation{}, err
	}
	temporary, err := os.MkdirTemp(planDirectories[0], ".planr-")
	if err != nil {
		return Operation{}, err
	}
	defer os.RemoveAll(temporary)
	if err := writeRenderedPlan(temporary, documents); err != nil {
		return Operation{}, err
	}
	if err := os.Rename(temporary, target); err != nil {
		return Operation{}, err
	}
	if !jsonOutput {
		fmt.Printf("Registered %s\n", planDirectory)
	}
	if err := hooks.RunDocument(repoRoot, settings.Hooks, settings.SkipHooks, "after", hooks.EventAdd, planDirectory, -1, "registered", jsonOutput); err != nil {
		return Operation{}, err
	}
	return op, nil
}

func writeRenderedPlan(root string, documents map[string]string) error {
	if err := os.MkdirAll(filepath.Join(root, "phases"), 0755); err != nil {
		return err
	}
	paths := make([]string, 0, len(documents))
	for path := range documents {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte(documents[path]), 0644); err != nil {
			return err
		}
	}
	return nil
}
