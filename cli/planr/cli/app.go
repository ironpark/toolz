package cli

import (
	"context"
	"io"
	"os"

	"github.com/ironpark/toolz/cli/planr/internal/gitrepo"
	ucli "github.com/urfave/cli/v3"
)

// Run parses args and executes the selected command. main() is a thin wrapper
// around it so the whole CLI stays testable from within this package.
func Run(ctx context.Context, args []string) error {
	return newRootCommand().Run(ctx, args)
}

func newRootCommand() *ucli.Command {
	return &ucli.Command{
		Name:                  "planr",
		Usage:                 "register and track structured implementation plans",
		Version:               buildVersion(),
		EnableShellCompletion: true,
		Flags: []ucli.Flag{
			&ucli.BoolFlag{Name: "no-hooks", Usage: "skip all configured hooks for this invocation"},
		},
		// Every command reads or writes plan state inside a repository, so the
		// check runs once here instead of at each call site. The two diagnostic
		// commands are intentionally allowed to run outside a repository so they
		// can report that condition themselves.
		Before: func(ctx context.Context, cmd *ucli.Command) (context.Context, error) {
			// The root Before callback receives the root command, whose first
			// parsed argument is the selected subcommand.
			commandName := ""
			if cmd.Args() != nil {
				commandName = cmd.Args().First()
			}
			if commandName == "config" || commandName == "doctor" || commandName == "completion" || commandName == "schema" || commandName == "init" || isShellCompletionInvocation(os.Args) {
				// `config` can inspect defaults without git, and `doctor` needs
				// to turn a missing repository into a diagnostic rather than an
				// early generic failure. `init` often runs before `git init`,
				// so it writes the configuration and warns instead.
				return ctx, nil
			}
			cwd, err := os.Getwd()
			if err != nil {
				return ctx, err
			}
			return ctx, gitrepo.EnsureRepository(cwd)
		},
		Commands: []*ucli.Command{
			{
				Name:  "init",
				Usage: "create .planr.yaml and the plans directories for this repository",
				Flags: []ucli.Flag{
					&ucli.StringFlag{Name: "language", Usage: "plan document language (en, ko)"},
					&ucli.StringSliceFlag{Name: "plans-dir", Usage: "plans directory relative to the repository root (repeatable)"},
					forceFlag("overwrite an existing .planr.yaml"),
					jsonFlag(),
				},
				Action: initCommand,
			},
			{
				Name:  "config",
				Usage: "show the applied configuration",
				Flags: []ucli.Flag{
					jsonFlag(),
				},
				Action: configCommand,
			},
			{
				Name:  "doctor",
				Usage: "diagnose configuration, plans, and repository consistency",
				Flags: []ucli.Flag{
					&ucli.BoolFlag{Name: "fix", Usage: "repair PLAN.md checklists from phase files"},
					jsonFlag(),
				},
				Action: doctorCommand,
			},
			{
				Name:      "new",
				Usage:     "create a structured Markdown draft",
				ArgsUsage: "<plan-name>[#phase-name] [description]",
				Flags: []ucli.Flag{
					&ucli.StringFlag{Name: "output", Usage: "draft file path"},
					&ucli.StringSliceFlag{Name: "depends-on", Usage: "plan dependency (repeatable)"},
					&ucli.StringFlag{Name: "description", Usage: "short plan description (max 200 characters)"},
					jsonFlag(),
				},
				ShellComplete: planNameShellComplete,
				Action:        newCommand,
			},
			{
				Name:      "edit",
				Usage:     "check out an existing plan document for editing",
				ArgsUsage: "<plan-name>#<phase-number> or <plan-name>",
				Flags: []ucli.Flag{
					sectionFlag(),
					&ucli.StringFlag{Name: "output", Usage: "editable file path"},
					jsonFlag(),
				},
				ShellComplete: planNameShellComplete,
				Action:        editCommand,
			},
			{
				Name:      "apply",
				Usage:     "validate and write a plan document",
				ArgsUsage: "[document-file]",
				Flags: []ucli.Flag{
					&ucli.BoolFlag{Name: "stdin", Usage: "read the document from stdin"},
					&ucli.BoolFlag{Name: "dry-run", Usage: "report changes without writing"},
					jsonFlag(),
				},
				Action: applyCommand,
			},
			{
				Name:  "schema",
				Usage: "describe the plan document contract",
				Flags: []ucli.Flag{
					jsonFlag(),
				},
				Action: schemaCommand,
			},
			{
				Name:      "status",
				Usage:     "show plan progress",
				ArgsUsage: "[plan-name]",
				Flags: []ucli.Flag{
					jsonFlag(),
				},
				ShellComplete: planNameShellComplete,
				Action:        statusCommand,
			},
			{
				Name:      "show",
				Usage:     "show the current or selected phase document",
				ArgsUsage: "<plan-name> [phase-number]",
				Flags: []ucli.Flag{
					jsonFlag(),
					sectionFlag(),
					&ucli.BoolFlag{Name: "all", Usage: "show the entire plan"},
				},
				ShellComplete: planNameShellComplete,
				Action:        showCommand,
			},
			{
				Name:      "overview",
				Usage:     "show a concise overview of all plans",
				ArgsUsage: "[plan-name]",
				Flags: []ucli.Flag{
					jsonFlag(),
				},
				ShellComplete: planNameShellComplete,
				Action:        overviewCommand,
			},
			{
				Name:      "notes",
				Usage:     "list plan and phase completions linked to commits",
				ArgsUsage: "[plan-name]",
				Flags: []ucli.Flag{
					jsonFlag(),
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
			phaseCommand(),
		},
	}
}

// progressWriter is where a command sends its human-readable progress
// messages and its hook output. With --json, stdout carries a single machine
// readable document, so everything else is discarded.
func progressWriter(cmd *ucli.Command) io.Writer {
	if cmd.Bool("json") {
		return io.Discard
	}
	return os.Stdout
}
