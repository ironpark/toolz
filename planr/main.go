package main

import (
	"context"
	"log"
	"os"
	"runtime/debug"

	"github.com/urfave/cli/v3"
)

// version is overridable at link time (-ldflags "-X main.version=v1.2.3") for
// release builds. Installs made with `go install ...@latest` leave it unset and
// fall back to the module version the binary was built from.
var version = ""

func buildVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "unknown"
	}
	return info.Main.Version
}

func main() {
	command := newRootCommand()

	if err := command.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func newRootCommand() *cli.Command {
	return &cli.Command{
		Name:                  "planr",
		Usage:                 "register and track structured implementation plans",
		Version:               buildVersion(),
		EnableShellCompletion: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "no-hooks", Usage: "skip all configured hooks for this invocation"},
		},
		// Every command reads or writes plan state inside a repository, so the
		// check runs once here instead of at each call site. The two diagnostic
		// commands are intentionally allowed to run outside a repository so they
		// can report that condition themselves.
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			// The root Before callback receives the root command, whose first
			// parsed argument is the selected subcommand.
			commandName := ""
			if cmd.Args() != nil {
				commandName = cmd.Args().First()
			}
			if commandName == "config" || commandName == "doctor" || commandName == "completion" {
				// `config` can inspect defaults without git, and `doctor` needs
				// to turn a missing repository into a diagnostic rather than an
				// early generic failure.
				return ctx, nil
			}
			cwd, err := os.Getwd()
			if err != nil {
				return ctx, err
			}
			return ctx, ensureGitRepository(cwd)
		},
		Commands: []*cli.Command{
			{
				Name:  "config",
				Usage: "show the applied configuration",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "json", Usage: "write machine-readable JSON"},
				},
				Action: configCommand,
			},
			{
				Name:  "doctor",
				Usage: "diagnose configuration, plans, and repository consistency",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "fix", Usage: "repair PLAN.md checklists from phase files"},
					&cli.BoolFlag{Name: "json", Usage: "write machine-readable JSON"},
				},
				Action: doctorCommand,
			},
			{
				Name:      "new",
				Usage:     "create a structured Markdown draft",
				ArgsUsage: "<plan-name> [description]",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "output", Usage: "draft file path"},
					&cli.StringSliceFlag{Name: "depends-on", Usage: "plan dependency (repeatable)"},
					&cli.StringFlag{Name: "description", Usage: "short plan description (max 200 characters)"},
				},
				Action: newCommand,
			},
			{
				Name:      "add",
				Usage:     "add a structured Markdown draft as a plan",
				ArgsUsage: "<draft-file>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "name", Usage: "plan name"},
				},
				Action: addCommand,
			},
			{
				Name:      "status",
				Usage:     "show plan progress",
				ArgsUsage: "[plan-name]",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "json", Usage: "write machine-readable JSON"},
				},
				ShellComplete: planNameShellComplete,
				Action:        statusCommand,
			},
			{
				Name:      "show",
				Usage:     "show the current or selected phase document",
				ArgsUsage: "<plan-name> [phase-number]",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "json", Usage: "write machine-readable JSON"},
				},
				ShellComplete: planNameShellComplete,
				Action:        showCommand,
			},
			{
				Name:      "overview",
				Usage:     "show a concise overview of all plans",
				ArgsUsage: "[plan-name]",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "json", Usage: "write machine-readable JSON"},
				},
				ShellComplete: planNameShellComplete,
				Action:        overviewCommand,
			},
			{
				Name:      "notes",
				Usage:     "list plan and phase completions linked to commits",
				ArgsUsage: "[plan-name]",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "json", Usage: "write machine-readable JSON"},
				},
				ShellComplete: planNameShellComplete,
				Action:        notesCommand,
			},
			{
				Name:          "archive",
				Usage:         "move a completed plan to the archive directory",
				ArgsUsage:     "<plan-name>",
				ShellComplete: planNameShellComplete,
				Action:        archiveCommand,
			},
			{
				Name:  "phase",
				Usage: "manage plan phases",
				Commands: []*cli.Command{
					{
						Name:      "add",
						Usage:     "add a phase to an open plan",
						ArgsUsage: "<plan-name> <phase-title>",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "slug", Usage: "phase slug (derived from the title when omitted)"},
							&cli.StringSliceFlag{Name: "depends-on", Usage: "existing phase number or slug (repeatable)"},
							&cli.StringFlag{Name: "status", Value: "planned", Usage: "planned or conditional"},
							&cli.StringFlag{Name: "entry-condition", Usage: "required when status is conditional"},
							&cli.BoolFlag{Name: "perf-phase", Usage: "mark this as a performance phase"},
							&cli.StringFlag{Name: "work", Usage: "planned work (required)"},
							&cli.StringFlag{Name: "done-when", Usage: "completion condition (required)"},
						},
						ShellComplete: planNameShellComplete,
						Action:        phaseAddCommand,
					},
					{
						Name:      "set",
						Aliases:   []string{"update"},
						Usage:     "set a phase status",
						ArgsUsage: "<plan-name> <phase-number>",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "status", Usage: "planned, conditional, in-progress, or done"},
							&cli.BoolFlag{Name: "force", Usage: "mark done despite uncommitted source changes"},
						},
						ShellComplete: planNameShellComplete,
						Action:        phaseSetCommand,
					},
					{
						Name:      "start",
						Usage:     "start a phase",
						ArgsUsage: "<plan-name> <phase-number>",
						Flags: []cli.Flag{
							&cli.BoolFlag{Name: "force", Usage: "start despite unfinished dependencies"},
						},
						ShellComplete: planNameShellComplete,
						Action:        phaseShortcutCommand("in-progress"),
					},
					{
						Name:      "done",
						Usage:     "complete a phase",
						ArgsUsage: "<plan-name> <phase-number>",
						Flags: []cli.Flag{
							&cli.BoolFlag{Name: "force", Usage: "complete despite unfinished dependencies or uncommitted source changes"},
						},
						ShellComplete: planNameShellComplete,
						Action:        phaseShortcutCommand("done"),
					},
					{
						Name:          "reset",
						Usage:         "reset a phase to planned",
						ArgsUsage:     "<plan-name> <phase-number>",
						ShellComplete: planNameShellComplete,
						Action:        phaseShortcutCommand("planned"),
					},
					{
						Name:      "rm",
						Usage:     "remove a phase from an open plan",
						ArgsUsage: "<plan-name> <phase-number>",
						Flags: []cli.Flag{
							&cli.BoolFlag{Name: "force", Usage: "remove a phase despite dependent phases"},
						},
						ShellComplete: planNameShellComplete,
						Action:        phaseRemoveCommand,
					},
				},
			},
		},
	}
}
