package cmd

import "github.com/urfave/cli/v3"

func NewCompare(action cli.ActionFunc, defaultReportDir string) *cli.Command {
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
			&cli.StringFlag{Name: "report-dir", Value: defaultReportDir, Usage: "directory to write reports into"},
		},
		Action: action,
	}
}
