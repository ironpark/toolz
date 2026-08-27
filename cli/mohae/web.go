package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

func newWebCommand() *cli.Command {
	return &cli.Command{
		Name:  "web",
		Usage: "serve the dashboard: conversation viewer, token charts and the A/B studio",
		Flags: []cli.Flag{
			&cli.IntFlag{Name: "port", Aliases: []string{"p"}, Value: 3000, Usage: "port to bind"},
			// Loopback by default: a dashboard carries whole conversations and
			// whatever the fixtures contain, so exposing it takes a deliberate
			// flag rather than a default.
			&cli.StringFlag{Name: "host", Aliases: []string{"H"}, Value: "127.0.0.1", Usage: "address to bind"},
			&cli.BoolFlag{Name: "open", Value: true, Usage: "open a browser once the server is up"},
			&cli.StringFlag{Name: "report-dir", Aliases: []string{"d"}, Value: DefaultReportDir, Usage: "directory of reports to load"},
			&cli.StringFlag{Name: "config-dir", Aliases: []string{"c"}, Value: ".", Usage: "directory the UI browses for configurations"},
			&cli.BoolFlag{Name: "read-only", Usage: "serve as a viewer: no running or editing from the UI"},
		},
		Action: webAction,
	}
}

func webAction(_ context.Context, cmd *cli.Command) error {
	port := cmd.Int("port")
	if port < 1 || port > 65535 {
		return fmt.Errorf("--port must be between 1 and 65535")
	}
	return notImplemented("web")
}
