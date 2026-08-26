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
				ArgsUsage: "<plan-name>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "output", Usage: "draft file path"},
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
		},
	}

	if err := command.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
