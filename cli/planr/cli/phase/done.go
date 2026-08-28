package phase

import ucli "github.com/urfave/cli/v3"

// doneCommand completes a phase. Completion is recorded as a git note against
// HEAD, so it also refuses to run with uncommitted source changes unless
// --force is given.
func doneCommand(complete ucli.ShellCompleteFunc) *ucli.Command {
	return &ucli.Command{
		Name:      "done",
		Usage:     "complete a phase",
		ArgsUsage: "<plan-name> <phase-number>",
		Flags: []ucli.Flag{
			forceFlag("complete despite unfinished dependencies or uncommitted source changes"),
		},
		ShellComplete: complete,
		Action:        shortcut("done"),
	}
}
