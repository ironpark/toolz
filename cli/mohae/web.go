package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

func webAction(_ context.Context, cmd *cli.Command) error {
	port := cmd.Int("port")
	if port < 1 || port > 65535 {
		return fmt.Errorf("--port must be between 1 and 65535")
	}
	return notImplemented("web")
}
