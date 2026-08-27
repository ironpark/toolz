package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// CompareFields are the things an A/B trial can differ in. `auto` infers the
// field from what actually differs between the two sides, so the common case
// needs no flag at all.
var CompareFields = []string{"auto", "prompts", "agent-md", "agent", "mcp", "config"}

// CompareMetrics are the numbers a comparison can be decided on.
var CompareMetrics = []string{"success-rate", "tokens", "cost", "duration"}

func newCompareCommand() *cli.Command {
	return &cli.Command{
		Name:  "compare",
		Usage: "run two variants against each other and contrast success rate, tokens and duration",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "a", Usage: "baseline: a configuration path, or a value for the compared field", Required: true},
			&cli.StringFlag{Name: "b", Usage: "variant: a configuration path, or a value for the compared field", Required: true},
			&cli.StringFlag{Name: "target", Value: "auto", Usage: "field that differs: auto, prompts, agent-md, agent, mcp, config"},
			// Agent runs are not deterministic, so a single pair of runs cannot
			// separate a real difference from noise.
			&cli.IntFlag{Name: "repeat", Aliases: []string{"n"}, Value: 3, Usage: "repetitions per side"},
			&cli.StringFlag{Name: "metric", Usage: "headline metric: success-rate, tokens, cost, duration"},
			&cli.BoolFlag{Name: "web", Usage: "open the comparison in the dashboard's matrix view"},
			&cli.StringFlag{Name: "report-dir", Value: DefaultReportDir, Usage: "directory to write reports into"},
		},
		Action: compareAction,
	}
}

func compareAction(_ context.Context, cmd *cli.Command) error {
	if !contains(CompareFields, cmd.String("target")) {
		return fmt.Errorf("unknown --target %q", cmd.String("target"))
	}
	if metric := cmd.String("metric"); metric != "" && !contains(CompareMetrics, metric) {
		return fmt.Errorf("unknown --metric %q", metric)
	}
	if cmd.Int("repeat") < 1 {
		return fmt.Errorf("--repeat must be at least 1")
	}
	if cmd.String("a") == cmd.String("b") {
		return fmt.Errorf("--a and --b are identical; there is nothing to compare")
	}
	return notImplemented("compare")
}
