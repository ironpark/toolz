package cmd

import "github.com/urfave/cli/v3"

func NewInit(action cli.ActionFunc) *cli.Command {
	return &cli.Command{
		Name:      "init",
		Usage:     "write a configuration template, optionally with its scripts and AGENTS.md",
		ArgsUsage: "[PATH]",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "template", Aliases: []string{"t"}, Value: "basic", Usage: "preset: basic, mcp-server, cli-skill, multi-agent"},
			&cli.BoolFlag{Name: "with-scripts", Usage: "also write init.sh and verify.sh"},
			&cli.BoolFlag{Name: "with-agent-md", Usage: "also write an AGENTS.md template"},
			&cli.BoolFlag{Name: "with-prompt", Usage: "also write the PROMPT.md the config sends as its first turn"},
			&cli.BoolFlag{Name: "with-fixture", Usage: "also write the fixture workspace the trial is run against"},
			&cli.BoolFlag{Name: "with-mcp", Usage: "also write the MCP server configuration (mcp-server template)"},
			// --all makes the generated project immediately verifiable instead of
			// leaving referenced support files absent.
			&cli.BoolFlag{Name: "all", Usage: "write every file the chosen template's configuration references"},
			&cli.BoolFlag{Name: "force", Aliases: []string{"f"}, Usage: "overwrite existing files"},
		},
		Action: action,
	}
}
