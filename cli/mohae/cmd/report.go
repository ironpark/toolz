package cmd

import "github.com/urfave/cli/v3"

func NewReport(action cli.ActionFunc) *cli.Command {
	return &cli.Command{
		Name:      "report",
		Usage:     "re-render or aggregate reports from earlier runs",
		ArgsUsage: "[REPORT_PATH]",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Value: "terminal", Usage: "format: terminal, json, markdown, html"},
			&cli.StringFlag{Name: "export", Usage: "write the rendered report to this path"},
			&cli.BoolFlag{Name: "filter-failed", Usage: "keep only the failed cases"},
			&cli.BoolFlag{Name: "aggregate", Usage: "total the tokens, cost and success rate across every report found"},
		},
		Action: action,
	}
}
