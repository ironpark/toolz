package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/ironpark/toolz/cli/planr/internal/notes"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/urfave/cli/v3"
)

func phaseSetCommand(_ context.Context, cmd *cli.Command) error {
	return phaseCommand(cmd, strings.TrimSpace(cmd.String("status")))
}

func phaseShortcutCommand(status string) func(context.Context, *cli.Command) error {
	return func(_ context.Context, cmd *cli.Command) error {
		return phaseCommand(cmd, status)
	}
}

func phaseCommand(cmd *cli.Command, status string) error {
	if cmd.NArg() != 2 {
		return fmt.Errorf("phase command requires <plan-name> <phase-number>")
	}
	if !plan.StatusValues[status] {
		return fmt.Errorf("invalid phase status %q; use planned, conditional, in-progress, or done", status)
	}
	phaseID, err := strconv.Atoi(cmd.Args().Get(1))
	if err != nil || phaseID < 0 {
		return fmt.Errorf("phase number %q must be a non-negative integer", cmd.Args().Get(1))
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	settings, repoRoot, err := config.Load(cwd)
	if err != nil {
		return err
	}
	settings = commandSettings(settings, cmd)
	planDirectories := settings.PlanDirs(repoRoot)
	planRoot, planDirectory, err := plan.FindDirectory(planDirectories, cmd.Args().First())
	if err != nil {
		return err
	}
	planLock, err := plan.AcquireLock(planRoot)
	if err != nil {
		return err
	}
	defer planLock.Close()
	event := plan.HookEvent(status)
	willComplete := false
	planWasDone := false
	if status == "done" {
		planWasDone, err = plan.AlreadyDone(planRoot)
		if err != nil {
			return err
		}
	}
	if status == "done" && len(settings.Hooks.Commands("before", hooks.EventPlanDone)) > 0 {
		willComplete, err = plan.WillComplete(planRoot, phaseID)
		if err != nil {
			return err
		}
		willComplete = willComplete && !planWasDone
	}
	if !cmd.Bool("force") {
		// Starting or completing a phase out of order silently invalidates the
		// ordering the plan was validated against, so the same graph `apply`
		// checked is enforced here too.
		if err := plan.EnsureDependenciesMet(planDirectories, planRoot, planDirectory, phaseID, status); err != nil {
			return err
		}
		if status == "done" {
			if err := plan.EnsureCleanSource(repoRoot, planDirectories, settings.Ignore); err != nil {
				return err
			}
		}
	}
	if err := hooks.Run(repoRoot, settings.Hooks, settings.SkipHooks, "before", event, planDirectory, phaseID, status); err != nil {
		return err
	}
	if willComplete {
		if err := hooks.Run(repoRoot, settings.Hooks, settings.SkipHooks, "before", hooks.EventPlanDone, planDirectory, -1, "done"); err != nil {
			return err
		}
	}
	var completed bool
	planDirectory, completed, err = plan.UpdatePhaseStatusLocked(planRoot, planDirectory, phaseID, status)
	if err != nil {
		return err
	}
	fmt.Printf("Updated %s phase %02d: %s\n", planDirectory, phaseID, status)
	// Link the completion to the commit it landed on, for `planr notes`.
	if status == "in-progress" {
		if err := notes.RecordCompletion(repoRoot, planDirectory, hooks.EventStart, phaseID); err != nil {
			notes.WarnStartFailure(err)
		}
	}
	if status == "done" {
		if err := notes.RecordCompletion(repoRoot, planDirectory, hooks.EventDone, phaseID); err != nil {
			notes.WarnFailure(err)
		}
	}
	if completed {
		fmt.Printf("Plan %s marked done\n", planDirectory)
		if !planWasDone {
			if err := notes.RecordCompletion(repoRoot, planDirectory, hooks.EventPlanDone, -1); err != nil {
				notes.WarnFailure(err)
			}
		}
	}
	if err := hooks.Run(repoRoot, settings.Hooks, settings.SkipHooks, "after", event, planDirectory, phaseID, status); err != nil {
		return err
	}
	if completed && status == "done" && !planWasDone {
		if err := hooks.Run(repoRoot, settings.Hooks, settings.SkipHooks, "after", hooks.EventPlanDone, planDirectory, -1, "done"); err != nil {
			return err
		}
	}
	return nil
}
