package cmd

import (
	"context"
	"fmt"

	reportformat "github.com/ironpark/toolz/cli/mohae/internal/report/format"
	"github.com/urfave/cli/v3"
)

func NewReport() *cli.Command {
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
	if err := checkFlagValue("output", cmd.String("output"), reportformat.All()); err != nil {
		return err
	}
	if cmd.NArg() > 1 {
		return fmt.Errorf("report accepts at most one path")
	}
	return notImplemented("report")
}
