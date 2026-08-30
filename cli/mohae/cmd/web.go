package cmd

import (
	"context"
	"fmt"

	"github.com/ironpark/toolz/cli/mohae/internal/config"
	"github.com/urfave/cli/v3"
)

func NewWeb() *cli.Command {
	return &cli.Command{
		Name:  "web",
		Usage: "serve the dashboard: conversation viewer, token charts and the A/B studio",
		Flags: []cli.Flag{
			&cli.IntFlag{Name: "port", Aliases: []string{"p"}, Value: 3000, Usage: "port to bind"},
			// Reports may contain conversations and source excerpts, so exposure
			// beyond the local machine must be deliberate.
			&cli.StringFlag{Name: "host", Aliases: []string{"H"}, Value: "127.0.0.1", Usage: "address to bind"},
			&cli.BoolFlag{Name: "open", Value: true, Usage: "open a browser once the server is up"},
			&cli.StringFlag{Name: "report-dir", Aliases: []string{"d"}, Value: config.DefaultReportDir, Usage: "directory of reports to load"},
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
