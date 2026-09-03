package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/gitrepo"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/ironpark/toolz/cli/planr/internal/notes"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/ironpark/toolz/cli/planr/internal/planlock"
	ucli "github.com/urfave/cli/v3"
)

func newPhaseCommand() *ucli.Command {
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
					forceFlag("mark done despite uncommitted source changes"),
				},
				ShellComplete: planNameShellComplete,
				Action:        runSetPhase,
			},
			newPhaseStartCommand(),
			newPhaseDoneCommand(),
			{
				Name:          "reset",
				Usage:         "reset a phase to planned",
				ArgsUsage:     "<plan-name> <phase-number>",
				ShellComplete: planNameShellComplete,
				Action:        phaseShortcut(draft.StatusPlanned),
			},
			{
				Name:      "rm",
				Usage:     "remove a phase from an open plan",
				ArgsUsage: "<plan-name> <phase-number>",
				Flags: []ucli.Flag{
					forceFlag("remove a phase despite dependent phases"),
				},
				ShellComplete: planNameShellComplete,
				Action:        runRemovePhase,
			},
		},
	}
}

func newPhaseStartCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "start",
		Usage:     "start a phase",
		ArgsUsage: "<plan-name> <phase-number>",
		Flags: []ucli.Flag{
			forceFlag("start despite unfinished dependencies"),
		},
		ShellComplete: planNameShellComplete,
		Action:        phaseShortcut(draft.StatusInProgress),
	}
}

func newPhaseDoneCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "done",
		Usage:     "complete a phase",
		ArgsUsage: "<plan-name> <phase-number>",
		Flags: []ucli.Flag{
			forceFlag("complete despite unfinished dependencies or uncommitted source changes"),
		},
		ShellComplete: planNameShellComplete,
		Action:        phaseShortcut(draft.StatusDone),
	}
}

func runSetPhase(_ context.Context, cmd *ucli.Command) error {
	return runPhaseCommand(cmd, strings.TrimSpace(cmd.String("status")))
}

func phaseShortcut(status string) func(context.Context, *ucli.Command) error {
	return func(_ context.Context, cmd *ucli.Command) error {
		return runPhaseCommand(cmd, status)
	}
}

func runPhaseCommand(cmd *ucli.Command, status string) error {
	if cmd.NArg() != 2 {
		return fmt.Errorf("phase command requires <plan-name> <phase-number>")
	}
	if !draft.ValidStatus(status) {
		return fmt.Errorf("invalid phase status %q; use %s", status, strings.Join(draft.Statuses(), ", "))
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
	planLock, err := planlock.AcquirePlan(planRoot)
	if err != nil {
		return err
	}
	defer planLock.Close()
	event := phaseHookEvent(status)
	willComplete := false
	planWasDone := false
	if status == draft.StatusDone {
		planWasDone, err = plan.AlreadyDone(planRoot)
		if err != nil {
			return err
		}
	}
	if status == draft.StatusDone && len(settings.Hooks.Commands("before", hooks.EventPlanDone)) > 0 {
		willComplete, err = plan.WillComplete(planRoot, phaseID)
		if err != nil {
			return err
		}
		willComplete = willComplete && !planWasDone
	}
	if !cmd.Bool("force") {
		if err := plan.EnsureDependenciesMet(planDirectories, planRoot, planDirectory, phaseID, status); err != nil {
			return err
		}
		if status == draft.StatusDone {
			if err := gitrepo.EnsureCleanSource(repoRoot, planDirectories, settings.Ignore); err != nil {
				return err
			}
		}
	}
	if err := hooks.Run(repoRoot, settings.Hooks, settings.SkipHooks, "before", event, planDirectory, phaseID, status, os.Stdout); err != nil {
		return err
	}
	if willComplete {
		if err := hooks.Run(repoRoot, settings.Hooks, settings.SkipHooks, "before", hooks.EventPlanDone, planDirectory, -1, draft.StatusDone, os.Stdout); err != nil {
			return err
		}
	}
	completed, err := plan.UpdatePhaseStatusLocked(planRoot, planDirectory, phaseID, status)
	if err != nil {
		return err
	}
	fmt.Printf("Updated %s phase %02d: %s\n", planDirectory, phaseID, status)
	if status == draft.StatusInProgress {
		if err := notes.RecordCompletion(repoRoot, planDirectory, hooks.EventStart, phaseID); err != nil {
			notes.Warn("phase start", err)
		}
	}
	if status == draft.StatusDone {
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
	if completed && status == draft.StatusDone && !planWasDone {
		if err := hooks.Run(repoRoot, settings.Hooks, settings.SkipHooks, "after", hooks.EventPlanDone, planDirectory, -1, draft.StatusDone, os.Stdout); err != nil {
			return err
		}
	}
	return nil
}

func phaseHookEvent(status string) string {
	switch status {
	case draft.StatusPlanned:
		return hooks.EventReset
	case draft.StatusConditional:
		return hooks.EventConditional
	case draft.StatusInProgress:
		return hooks.EventStart
	case draft.StatusDone:
		return hooks.EventDone
	default:
		return ""
	}
}
