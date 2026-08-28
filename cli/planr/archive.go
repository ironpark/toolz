package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/urfave/cli/v3"
)

// archiveCommand moves a completed plan to the last configured plans_dirs
// entry. The first plans directory is locked as a directory-level coordination
// point so apply and archive cannot race while plan numbering is discovered.
func archiveCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 1 {
		return fmt.Errorf("archive requires <plan-name>")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	settings, repoRoot, err := config.Load(cwd)
	if err != nil {
		return err
	}
	planDirectories := settings.PlanDirs(repoRoot)
	if len(planDirectories) < 2 {
		return fmt.Errorf("archive requires at least two plans_dirs entries")
	}
	sourceRoot, sourceDirectory, err := findPlanDirectory(planDirectories, cmd.Args().First())
	if err != nil {
		return err
	}
	destination := planDirectories[len(planDirectories)-1]
	if filepath.Clean(filepath.Dir(sourceRoot)) == filepath.Clean(destination) {
		return fmt.Errorf("plan %q is already in the archive directory %s", sourceDirectory, destination)
	}
	completed, err := planAlreadyDone(sourceRoot)
	if err != nil {
		return fmt.Errorf("validate %s: %w", sourceDirectory, err)
	}
	if !completed {
		return fmt.Errorf("plan %q is not done; only completed plans can be archived", sourceDirectory)
	}

	directoryLock, err := acquirePlansDirectoryLock(planDirectories[0])
	if err != nil {
		return err
	}
	defer directoryLock.close()
	// Re-resolve after waiting for the directory lock. Another archive may have
	// moved this plan while the initial validation was in progress.
	sourceRoot, sourceDirectory, err = findPlanDirectory(planDirectories, cmd.Args().First())
	if err != nil {
		return err
	}
	if filepath.Clean(filepath.Dir(sourceRoot)) == filepath.Clean(destination) {
		return fmt.Errorf("plan %q is already in the archive directory %s", sourceDirectory, destination)
	}
	completed, err = planAlreadyDone(sourceRoot)
	if err != nil {
		return fmt.Errorf("validate %s: %w", sourceDirectory, err)
	}
	if !completed {
		return fmt.Errorf("plan %q is not done; only completed plans can be archived", sourceDirectory)
	}
	planLock, err := acquirePlanLock(sourceRoot)
	if err != nil {
		return err
	}
	defer planLock.close()
	completed, err = planAlreadyDone(sourceRoot)
	if err != nil {
		return fmt.Errorf("validate %s: %w", sourceDirectory, err)
	}
	if !completed {
		return fmt.Errorf("plan %q is not done; only completed plans can be archived", sourceDirectory)
	}

	if err := os.MkdirAll(destination, 0755); err != nil {
		return fmt.Errorf("create archive directory %s: %w", destination, err)
	}
	target := filepath.Join(destination, sourceDirectory)
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("archive destination already contains plan %q", sourceDirectory)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect archive destination %s: %w", target, err)
	}
	if err := os.Rename(sourceRoot, target); err != nil {
		return fmt.Errorf("archive plan %s to %s: %w", sourceDirectory, target, err)
	}
	fmt.Printf("Archived %s to %s\n", sourceDirectory, target)
	return nil
}
