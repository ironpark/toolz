package cmd

import "github.com/urfave/cli/v3"

func NewRun(action cli.ActionFunc, defaultReportDir string, defaultTimeoutSeconds int) *cli.Command {
	return &cli.Command{
		Name:      "run",
		Usage:     "run the trials described by one or more configuration files and report on them",
		ArgsUsage: "[CONFIG_PATH...]",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{Name: "profile", Usage: "apply a named profile from the config; repeatable, later ones win"},
			&cli.StringFlag{Name: "agent", Aliases: []string{"a"}, Usage: "override the agent type (claude-code, codex, custom-cli)"},
			&cli.StringSliceFlag{Name: "prompt", Aliases: []string{"p"}, Usage: "replace the conversation with these prompts, one turn each; a file://PATH value reads the turn from a file (repeatable)"},
			&cli.StringSliceFlag{Name: "prompt-when", Usage: "expr condition gating the prompt at the same position; use '' to leave one unconditional (repeatable)"},
			&cli.StringFlag{Name: "agent-md", Usage: "override the AGENTS.md installed in the workspace"},
			&cli.StringFlag{Name: "init-script", Usage: "override the workspace setup script"},
			&cli.StringSliceFlag{Name: "verify-command", Usage: "replace the commands that grade the finished workspace (repeatable)"},
			&cli.StringFlag{Name: "mcp-config", Aliases: []string{"m"}, Usage: "override the MCP server configuration"},
			&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Value: "terminal", Usage: "report format: terminal, json, markdown, html"},
			&cli.StringFlag{Name: "report-dir", Value: defaultReportDir, Usage: "directory to write reports into"},
			&cli.BoolFlag{Name: "show-dialogue", Usage: "stream the conversation to the terminal while it runs"},
			&cli.BoolFlag{Name: "detailed-tokens", Usage: "break tokens down by input, output, cache read and cache write"},
			&cli.BoolFlag{Name: "web", Usage: "serve the dashboard alongside the run"},
			&cli.IntFlag{Name: "timeout", Aliases: []string{"t"}, Value: defaultTimeoutSeconds, Usage: "seconds allowed for one trial"},
			&cli.BoolFlag{Name: "fail-fast", Usage: "stop at the first failed verification or command error"},
			&cli.IntFlag{Name: "concurrency", Aliases: []string{"c"}, Value: 1, Usage: "trials to run at the same time"},
		},
		Action: action,
	}
}
