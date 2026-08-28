package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/urfave/cli/v3"
)

func newReportCommand() *cli.Command {
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
		Action: reportAction,
	}
}

func reportAction(_ context.Context, cmd *cli.Command) error {
	if !slices.Contains(KnownFormats, cmd.String("output")) {
		return fmt.Errorf("unknown output format %q", cmd.String("output"))
	}
	if cmd.NArg() > 1 {
		return fmt.Errorf("report accepts at most one path")
	}
	return notImplemented("report")
}
