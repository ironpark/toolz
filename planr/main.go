package main

import (
	"context"
	"log"
	"os"

	"github.com/urfave/cli/v3"
)

func main() {
	command := &cli.Command{
		Name:  "planr",
		Usage: "register and track structured implementation plans",
		Commands: []*cli.Command{
			{
				Name:      "new",
				Usage:     "create a structured Markdown draft",
				ArgsUsage: "<plan-name> [description]",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "output", Usage: "draft file path"},
					&cli.StringSliceFlag{Name: "depends-on", Usage: "plan dependency (repeatable)"},
					&cli.StringFlag{Name: "description", Usage: "short plan description (max 200 characters)"},
				},
				Action: newCommand,
			},
			{
				Name:      "add",
				Usage:     "add a structured Markdown draft as a plan",
				ArgsUsage: "<draft-file>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "name", Usage: "plan name"},
				},
				Action: addCommand,
			},
			{
				Name:      "status",
				Usage:     "show plan progress",
				ArgsUsage: "[plan-name]",
				Action:    statusCommand,
			},
			{
				Name:      "overview",
				Usage:     "show a concise overview of all plans",
				ArgsUsage: "[plan-name]",
				Action:    overviewCommand,
			},
			{
				Name:  "phase",
				Usage: "manage plan phases",
				Commands: []*cli.Command{
					{
						Name:      "add",
						Usage:     "add a phase to an open plan",
						ArgsUsage: "<plan-name> <phase-title>",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "slug", Usage: "phase slug (derived from the title when omitted)"},
							&cli.StringSliceFlag{Name: "depends-on", Usage: "existing phase number (repeatable)"},
							&cli.StringFlag{Name: "status", Value: "planned", Usage: "planned or conditional"},
							&cli.StringFlag{Name: "entry-condition", Usage: "required when status is conditional"},
							&cli.BoolFlag{Name: "perf-phase", Usage: "mark this as a performance phase"},
							&cli.StringFlag{Name: "work", Usage: "planned work (required)"},
							&cli.StringFlag{Name: "done-when", Usage: "completion condition (required)"},
						},
						Action: phaseAddCommand,
					},
					{
						Name:      "set",
						Aliases:   []string{"update"},
						Usage:     "set a phase status",
						ArgsUsage: "<plan-name> <phase-number>",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "status", Usage: "planned, conditional, in-progress, or done"},
							&cli.BoolFlag{Name: "force", Usage: "mark done despite uncommitted source changes"},
						},
						Action: phaseSetCommand,
					},
					{
						Name:      "start",
						Usage:     "start a phase",
						ArgsUsage: "<plan-name> <phase-number>",
						Action:    phaseShortcutCommand("in-progress"),
					},
					{
						Name:      "done",
						Usage:     "complete a phase",
						ArgsUsage: "<plan-name> <phase-number>",
						Flags: []cli.Flag{
							&cli.BoolFlag{Name: "force", Usage: "complete despite uncommitted source changes"},
						},
						Action: phaseShortcutCommand("done"),
					},
					{
						Name:      "reset",
						Usage:     "reset a phase to planned",
						ArgsUsage: "<plan-name> <phase-number>",
						Action:    phaseShortcutCommand("planned"),
					},
				},
			},
		},
	}

	if err := command.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
