package app

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
