package cmd

import "github.com/urfave/cli/v3"

func NewVerify(action cli.ActionFunc) *cli.Command {
	return &cli.Command{
		Name:      "verify",
		Usage:     "check configurations and their dependencies without running a trial",
		ArgsUsage: "[CONFIG_PATH...]",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "check-mcp", Usage: "ping the MCP servers and list their tools"},
			&cli.BoolFlag{Name: "check-scripts", Usage: "check the init and verify scripts for syntax errors and the executable bit"},
			&cli.BoolFlag{Name: "check-agent-md", Usage: "check AGENTS.md for the required sections"},
			&cli.BoolFlag{Name: "strict", Usage: "treat warnings as failures"},
		},
		Action: action,
	}
}
