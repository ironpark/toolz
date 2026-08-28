package phase

import (
	"github.com/ironpark/toolz/cli/planr/internal/cliflag"
	ucli "github.com/urfave/cli/v3"
)

// startCommand marks a phase in-progress. It is the point where dependency
// order is enforced, so an out-of-order start fails unless --force is given.
func startCommand(complete ucli.ShellCompleteFunc) *ucli.Command {
	return &ucli.Command{
		Name:      "start",
		Usage:     "start a phase",
		ArgsUsage: "<plan-name> <phase-number>",
		Flags: []ucli.Flag{
			cliflag.Force("start despite unfinished dependencies"),
		},
		ShellComplete: complete,
		Action:        shortcut("in-progress"),
	}
}
