package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/urfave/cli/v3"
)

const (
	DefaultTimeoutSeconds = 300
	DefaultMaxTurns       = 30
)

func newRunCommand() *cli.Command {
	return &cli.Command{
		Name:      "run",
		Usage:     "run the trials described by one or more configuration files and report on them",
		ArgsUsage: "[CONFIG_PATH...]",
		Flags: []cli.Flag{
			// Overrides. Each one shadows a configuration field for this
			// invocation only, which is what makes a config reusable across
			// variants instead of edited between runs.
			&cli.StringFlag{Name: "agent", Aliases: []string{"a"}, Usage: "override the agent type (claude-code, codex, custom-cli)"},
			&cli.StringFlag{Name: "prompt", Aliases: []string{"p"}, Usage: "override the opening prompt inline"},
			&cli.StringFlag{Name: "prompt-file", Aliases: []string{"P"}, Usage: "override the opening prompt with a file"},
			&cli.StringFlag{Name: "agent-md", Usage: "override the AGENTS.md installed in the workspace"},
			&cli.StringFlag{Name: "init-script", Usage: "override the workspace setup script"},
			&cli.StringFlag{Name: "verify-script", Usage: "override the script that grades the finished workspace"},
			&cli.StringFlag{Name: "mcp-config", Aliases: []string{"m"}, Usage: "override the MCP server configuration"},
			&cli.StringFlag{Name: "target-cli", Usage: "override the CLI under test"},

			&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Value: "terminal", Usage: "report format: terminal, json, markdown, html"},
			&cli.StringFlag{Name: "report-dir", Value: DefaultReportDir, Usage: "directory to write reports into"},
			&cli.BoolFlag{Name: "show-dialogue", Usage: "stream the conversation to the terminal while it runs"},
			&cli.BoolFlag{Name: "detailed-tokens", Usage: "break tokens down by input, output, cache read and cache write"},
			&cli.BoolFlag{Name: "web", Usage: "serve the dashboard alongside the run"},

			&cli.IntFlag{Name: "timeout", Aliases: []string{"t"}, Value: DefaultTimeoutSeconds, Usage: "seconds allowed for one trial"},
			&cli.IntFlag{Name: "max-turns", Value: DefaultMaxTurns, Usage: "maximum conversation turns"},
			&cli.BoolFlag{Name: "fail-fast", Usage: "stop at the first failed verification or command error"},
			&cli.IntFlag{Name: "concurrency", Aliases: []string{"c"}, Value: 1, Usage: "trials to run at the same time"},
		},
		Action: runAction,
	}
}

func runAction(_ context.Context, cmd *cli.Command) error {
	configs, err := loadConfigs(cmd.Args().Slice())
	if err != nil {
		return err
	}
	if err := applyRunOverrides(cmd, configs); err != nil {
		return err
	}
	if !contains(KnownFormats, cmd.String("output")) {
		return fmt.Errorf("unknown output format %q", cmd.String("output"))
	}
	if cmd.Int("concurrency") < 1 {
		return fmt.Errorf("concurrency must be at least 1")
	}
	for _, config := range configs {
		fmt.Printf("%s  agent=%s workspace=%s\n", config.Name, config.Agent.Type, config.Workspace.Source)
	}
	return notImplemented("run")
}

// applyRunOverrides folds the command line into every selected configuration,
// so `--prompt` means the same thing whether one config was named or twenty.
func applyRunOverrides(cmd *cli.Command, configs []*Config) error {
	if cmd.IsSet("prompt") && cmd.IsSet("prompt-file") {
		return fmt.Errorf("--prompt and --prompt-file are mutually exclusive")
	}
	for _, config := range configs {
		if value := cmd.String("agent"); value != "" {
			config.Agent.Type = value
		}
		if cmd.IsSet("prompt") {
			config.Prompt = PromptConfig{Text: cmd.String("prompt")}
		}
		if cmd.IsSet("prompt-file") {
			config.Prompt = PromptConfig{File: absoluteOverride(cmd.String("prompt-file"))}
		}
		if value := cmd.String("agent-md"); value != "" {
			config.Workspace.AgentMD = absoluteOverride(value)
		}
		if value := cmd.String("init-script"); value != "" {
			config.Workspace.InitScript = absoluteOverride(value)
		}
		if value := cmd.String("verify-script"); value != "" {
			config.Verify.Script = absoluteOverride(value)
		}
		if value := cmd.String("mcp-config"); value != "" {
			config.MCP.Config = absoluteOverride(value)
		}
		if value := cmd.String("target-cli"); value != "" {
			config.TargetCLI.Command = value
		}
		if cmd.IsSet("timeout") {
			config.Limits.TimeoutSeconds = cmd.Int("timeout")
		}
		if cmd.IsSet("max-turns") {
			config.Limits.MaxTurns = cmd.Int("max-turns")
		}
		if cmd.IsSet("report-dir") {
			config.Report.Dir = cmd.String("report-dir")
		}
		if err := config.Validate(); err != nil {
			return fmt.Errorf("%s: %w", config.Path, err)
		}
	}
	return nil
}

// absoluteOverride pins a command-line path to the working directory before it
// reaches a config. Paths inside a config are read against the config's own
// directory, and an override that inherited that rule would resolve against a
// directory the caller never typed.
func absoluteOverride(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absolute
}

// loadConfigs resolves the positional arguments to configuration files. Globs
// are expanded here rather than left to the shell so a quoted pattern behaves
// the same on every platform.
func loadConfigs(arguments []string) ([]*Config, error) {
	if len(arguments) == 0 {
		arguments = []string{DefaultConfigName}
	}
	paths := []string{}
	seen := map[string]bool{}
	for _, argument := range arguments {
		matches, err := filepath.Glob(argument)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", argument, err)
		}
		if len(matches) == 0 {
			// Not a glob, or a glob that matched nothing: keep the literal so
			// the failure names the path the caller typed.
			matches = []string{argument}
		}
		sort.Strings(matches)
		for _, match := range matches {
			if !seen[match] {
				seen[match] = true
				paths = append(paths, match)
			}
		}
	}
	configs := make([]*Config, 0, len(paths))
	for _, path := range paths {
		config, err := LoadConfig(path)
		if err != nil {
			return nil, err
		}
		configs = append(configs, config)
	}
	return configs, nil
}
