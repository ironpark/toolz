// Package phase implements the `planr phase` command group. Phase status is
// the only plan state a command mutates directly, so it lives apart from the
// document commands that go through `apply`.
package phase

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/cliflag"
	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/ironpark/toolz/cli/planr/internal/notes"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	ucli "github.com/urfave/cli/v3"
)

// Command builds the `phase` command group. complete is the shared plan-name
// completion, which the parent package owns because it reads the same
// configuration the other commands do.
func Command(complete ucli.ShellCompleteFunc) *ucli.Command {
	return &ucli.Command{
		Name:  "phase",
		Usage: "manage plan phases",
		Commands: []*ucli.Command{
			{
				Name:      "set",
				Aliases:   []string{"update"},
				Usage:     "set a phase status",
				ArgsUsage: "<plan-name> <phase-number>",
				Flags: []ucli.Flag{
					&ucli.StringFlag{Name: "status", Usage: "planned, conditional, in-progress, or done"},
					cliflag.Force("mark done despite uncommitted source changes"),
				},
				ShellComplete: complete,
				Action:        setCommand,
			},
			startCommand(complete),
			doneCommand(complete),
			{
				Name:          "reset",
				Usage:         "reset a phase to planned",
				ArgsUsage:     "<plan-name> <phase-number>",
				ShellComplete: complete,
				Action:        shortcut("planned"),
			},
			{
				Name:      "rm",
				Usage:     "remove a phase from an open plan",
				ArgsUsage: "<plan-name> <phase-number>",
				Flags: []ucli.Flag{
					cliflag.Force("remove a phase despite dependent phases"),
				},
				ShellComplete: complete,
				Action:        removeCommand,
			},
		},
	}
}

func setCommand(_ context.Context, cmd *ucli.Command) error {
	return run(cmd, strings.TrimSpace(cmd.String("status")))
}

// shortcut binds a fixed status to a subcommand that takes no --status flag.
func shortcut(status string) func(context.Context, *ucli.Command) error {
	return func(_ context.Context, cmd *ucli.Command) error {
		return run(cmd, status)
	}
}

func run(cmd *ucli.Command, status string) error {
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
	settings = settings.WithSkipHooks(cmd.Bool("no-hooks"))
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
	if err := hooks.Run(repoRoot, settings.Hooks, settings.SkipHooks, "before", event, planDirectory, phaseID, status, os.Stdout); err != nil {
		return err
	}
	if willComplete {
		if err := hooks.Run(repoRoot, settings.Hooks, settings.SkipHooks, "before", hooks.EventPlanDone, planDirectory, -1, "done", os.Stdout); err != nil {
			return err
		}
	}
	completed, err := plan.UpdatePhaseStatusLocked(planRoot, planDirectory, phaseID, status)
	if err != nil {
		return err
	}
	fmt.Printf("Updated %s phase %02d: %s\n", planDirectory, phaseID, status)
	// Link the completion to the commit it landed on, for `planr notes`.
	if status == "in-progress" {
		if err := notes.RecordCompletion(repoRoot, planDirectory, hooks.EventStart, phaseID); err != nil {
			notes.Warn("phase start", err)
		}
	}
	if status == "done" {
		if err := notes.RecordCompletion(repoRoot, planDirectory, hooks.EventDone, phaseID); err != nil {
			notes.Warn("completion", err)
		}
	}
	if completed {
		fmt.Printf("Plan %s marked done\n", planDirectory)
		if !planWasDone {
			if err := notes.RecordCompletion(repoRoot, planDirectory, hooks.EventPlanDone, -1); err != nil {
				notes.Warn("completion", err)
			}
		}
	}
	if err := hooks.Run(repoRoot, settings.Hooks, settings.SkipHooks, "after", event, planDirectory, phaseID, status, os.Stdout); err != nil {
		return err
	}
	if completed && status == "done" && !planWasDone {
		if err := hooks.Run(repoRoot, settings.Hooks, settings.SkipHooks, "after", hooks.EventPlanDone, planDirectory, -1, "done", os.Stdout); err != nil {
			return err
		}
	}
	return nil
}
