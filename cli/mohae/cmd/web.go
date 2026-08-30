package cmd

import "github.com/urfave/cli/v3"

func NewWeb(action cli.ActionFunc, defaultReportDir string) *cli.Command {
	return &cli.Command{
		Name:  "web",
		Usage: "serve the dashboard: conversation viewer, token charts and the A/B studio",
		Flags: []cli.Flag{
			&cli.IntFlag{Name: "port", Aliases: []string{"p"}, Value: 3000, Usage: "port to bind"},
			// Loopback is the safe default because reports can contain prompts,
			// dialogue, paths, and source excerpts.
			&cli.StringFlag{Name: "host", Aliases: []string{"H"}, Value: "127.0.0.1", Usage: "address to bind"},
			&cli.BoolFlag{Name: "open", Value: true, Usage: "open a browser once the server is up"},
			&cli.StringFlag{Name: "report-dir", Aliases: []string{"d"}, Value: defaultReportDir, Usage: "directory of reports to load"},
			&cli.StringFlag{Name: "config-dir", Aliases: []string{"c"}, Value: ".", Usage: "directory the UI browses for configurations"},
			&cli.BoolFlag{Name: "read-only", Usage: "serve as a viewer: no running or editing from the UI"},
		},
		Action: action,
	}
}
