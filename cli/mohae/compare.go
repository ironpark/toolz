package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/urfave/cli/v3"
)

// CompareFields are the things an A/B trial can differ in. `auto` infers the
// field from what actually differs between the two sides, so the common case
// needs no flag at all.
var CompareFields = []string{"auto", "prompts", "agent-md", "agent", "mcp", "config"}

// CompareMetrics are the numbers a comparison can be decided on.
var CompareMetrics = []string{"success-rate", "tokens", "cost", "duration"}

func compareAction(_ context.Context, cmd *cli.Command) error {
	if !slices.Contains(CompareFields, cmd.String("target")) {
		return fmt.Errorf("unknown --target %q", cmd.String("target"))
	}
	if metric := cmd.String("metric"); metric != "" && !slices.Contains(CompareMetrics, metric) {
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
