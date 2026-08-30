package cmd

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
			newInitCommand(),
			newConfigCommand(),
			newDoctorCommand(),
			newDraftCommand(),
			newEditCommand(),
			newApplyCommand(),
			newSchemaCommand(),
			newStatusCommand(),
			newShowCommand(),
			newOverviewCommand(),
			newNotesCommand(),
			newArchiveCommand(),
			newPhaseCommand(),
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
