package app

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

func reportAction(_ context.Context, cmd *cli.Command) error {
	if err := checkFlagValue("output", cmd.String("output"), KnownFormats); err != nil {
		return err
	}
	if cmd.NArg() > 1 {
		return fmt.Errorf("report accepts at most one path")
	}
	return notImplemented("report")
}
