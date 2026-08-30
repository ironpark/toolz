package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/urfave/cli/v3"
)

func reportAction(_ context.Context, cmd *cli.Command) error {
	if !slices.Contains(KnownFormats, cmd.String("output")) {
		return fmt.Errorf("unknown output format %q", cmd.String("output"))
	}
	if cmd.NArg() > 1 {
		return fmt.Errorf("report accepts at most one path")
	}
	return notImplemented("report")
}
