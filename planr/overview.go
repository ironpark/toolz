package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"
)

func overviewCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() > 1 {
		return fmt.Errorf("overview accepts at most one plan name")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	planDirectories, err := planPaths(cwd)
	if err != nil {
		return err
	}
	// Collect every plan before resolving dependencies. When a single plan is
	// requested, its dependencies may live outside the filtered result and
	// still need to be classified as done, in-progress, or missing correctly.
	summaries, foundDirectory, err := collectPlanSummaries(planDirectories, "")
	if err != nil {
		return err
	}
	if !foundDirectory {
		return fmt.Errorf("no plans directories found: %s", strings.Join(planDirectories, ", "))
	}
	annotatePlanWaits(summaries)
	if filter := cmd.Args().First(); filter != "" {
		matched := summaries[:0]
		for _, summary := range summaries {
			if summary.name == filter || filepath.Base(summary.label) == filter {
				matched = append(matched, summary)
			}
		}
		summaries = matched
	}
	if len(summaries) == 0 {
		fmt.Println("No plans found")
		return nil
	}
	printPlanGroups(summaries, func(name string, summary planSummary) {
		status := summary.status
		if status == "" {
			status = "unknown"
		}
		done, total, next := summary.progress()
		fmt.Printf("  %s: %s (%d/%d phases)", name, status, done, total)
		if next != "" {
			fmt.Printf("; next: %s", next)
		}
		fmt.Println()
		printPlanList("wait", summary.wait)
	})
	return nil
}
