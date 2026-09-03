package cmd

import (
	"context"
	"fmt"

	"github.com/ironpark/toolz/cli/mohae/internal/config"
	"github.com/urfave/cli/v3"
)

func NewCompare() *cli.Command {
	return &cli.Command{
		Name:  "compare",
		Usage: "run two variants against each other and contrast success rate, tokens and duration",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "a", Usage: "baseline: a configuration path, or a value for the compared field", Required: true},
			&cli.StringFlag{Name: "b", Usage: "variant: a configuration path, or a value for the compared field", Required: true},
			&cli.StringFlag{Name: "target", Value: "auto", Usage: "field that differs: auto, prompts, agent-md, agent, mcp, config"},
			// Repetition is essential for nondeterministic agents: one sample can
			// make a weaker variant look better by chance.
			&cli.IntFlag{Name: "repeat", Aliases: []string{"n"}, Value: 3, Usage: "repetitions per side"},
			&cli.StringFlag{Name: "metric", Usage: "headline metric: success-rate, tokens, cost, duration"},
			&cli.BoolFlag{Name: "web", Usage: "open the comparison in the dashboard's matrix view"},
			&cli.StringFlag{Name: "report-dir", Value: config.DefaultReportDir, Usage: "directory to write reports into"},
		},
		Action: compareAction,
	}
}

// CompareFields are the things an A/B trial can differ in. `auto` infers the
// field from what actually differs between the two sides, so the common case
// needs no flag at all.
var CompareFields = []string{"auto", "prompts", "agent-md", "agent", "mcp", "config"}

// CompareMetrics are the numbers a comparison can be decided on.
var CompareMetrics = []string{"success-rate", "tokens", "cost", "duration"}

func compareAction(_ context.Context, cmd *cli.Command) error {
	if err := checkFlagValue("target", cmd.String("target"), CompareFields); err != nil {
		return err
	}
	if metric := cmd.String("metric"); metric != "" {
		if err := checkFlagValue("metric", metric, CompareMetrics); err != nil {
			return err
		}
	}
	if cmd.Int("repeat") < 1 {
		return fmt.Errorf("--repeat must be at least 1")
	}
	if cmd.String("a") == cmd.String("b") {
		return fmt.Errorf("--a and --b are identical; there is nothing to compare")
	}
	return notImplemented("compare")
}
