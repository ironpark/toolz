package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	configuration "github.com/ironpark/toolz/cli/mohae/internal/config"
	"github.com/ironpark/toolz/cli/mohae/internal/container"
	"github.com/ironpark/toolz/cli/mohae/internal/report"
	reportformat "github.com/ironpark/toolz/cli/mohae/internal/report/format"
	"github.com/ironpark/toolz/cli/mohae/internal/runner"
	"github.com/urfave/cli/v3"
)

func NewRun(version string) *cli.Command {
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
			&cli.StringFlag{Name: "container-image", Usage: "run the trial in this image instead of on the host"},
			&cli.StringFlag{Name: "container-scope", Usage: "what runs in the container: setup (the default) or full, which includes the agent"},
			&cli.StringSliceFlag{Name: "verify-command", Usage: "replace the commands that grade the finished workspace (repeatable)"},
			&cli.StringFlag{Name: "mcp-config", Aliases: []string{"m"}, Usage: "override the MCP server configuration"},
			&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Value: "terminal", Usage: "report format: terminal, json, markdown, html"},
			&cli.StringFlag{Name: "report-dir", Value: configuration.DefaultReportDir, Usage: "directory to write reports into"},
			&cli.BoolFlag{Name: "show-dialogue", Usage: "stream the conversation to the terminal while it runs"},
			&cli.BoolFlag{Name: "detailed-tokens", Usage: "break tokens down by input, output, cache read and cache write"},
			&cli.BoolFlag{Name: "web", Usage: "serve the dashboard alongside the run"},
			&cli.IntFlag{Name: "timeout", Aliases: []string{"t"}, Value: configuration.DefaultTimeoutSeconds, Usage: "seconds allowed for one trial"},
			&cli.BoolFlag{Name: "fail-fast", Usage: "stop at the first failed verification or command error"},
			&cli.IntFlag{Name: "concurrency", Aliases: []string{"c"}, Value: 1, Usage: "trials to run at the same time"},
		},
		Action: runAction(version),
	}
}

func runAction(version string) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		configs, err := loadConfigs(cmd.Args().Slice())
		if err != nil {
			return err
		}
		if err := applyRunOverrides(cmd, configs); err != nil {
			return err
		}
		output := cmd.String("output")
		if err := checkFlagValue("output", output, reportformat.All()); err != nil {
			return err
		}
		concurrency := cmd.Int("concurrency")
		if concurrency < 1 {
			return fmt.Errorf("concurrency must be at least 1")
		}
		if cmd.Bool("web") {
			// Failing here beats running the whole benchmark and only then
			// admitting the dashboard the caller asked for does not exist.
			return notImplemented("run --web")
		}

		reportOptions := report.ReportOptions{
			DetailedTokens: cmd.Bool("detailed-tokens"),
			ShowDialogue:   cmd.Bool("show-dialogue"),
			Version:        version,
		}
		trialOptions := runner.TrialOptions{
			ShowDialogue: cmd.Bool("show-dialogue"),
			Version:      version,
			// Serialized: with --concurrency the trials write at the same time, and
			// unsynchronised writes would tear each other's lines apart.
			Out: newLockedWriter(cmd.Writer),
		}

		// Before the run, so a machine that has been benchmarking for weeks does not
		// carry every failed trial's copy forever. Reported rather than silent: the
		// directories being reclaimed are debugging material.
		if pruned := runner.PruneStaleWorkspaces(runner.StaleWorkspaceAge); pruned > 0 {
			fmt.Fprintf(cmd.Writer, "pruned %d workspace(s) left by earlier runs\n", pruned)
		}
		// Containers are reclaimed on the same pass and for a stronger reason:
		// a left-behind directory costs disk, while a left-behind container
		// holds memory for as long as the machine is up. Only when this run
		// uses containers at all: the sweep costs a runtime probe and a daemon
		// round-trip, and a run made entirely of host trials cannot have
		// leaked one.
		if slices.ContainsFunc(configs, func(c *configuration.Config) bool { return c.Container.Enabled() }) {
			if pruned := container.PruneStale(); pruned > 0 {
				fmt.Fprintf(cmd.Writer, "removed %d container(s) left by earlier runs\n", pruned)
			}
		}

		results := runTrials(ctx, configs, concurrency, cmd.Bool("fail-fast"), trialOptions)

		rendered, err := report.RenderReport(output, results, reportOptions)
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.Writer, rendered)
		if err := writeRunReports(configs, results, output, reportOptions, cmd.Writer); err != nil {
			return err
		}

		failed := 0
		for _, result := range results {
			if !result.Passed {
				failed++
			}
		}
		if failed > 0 {
			// The exit status is what a CI job reads; a benchmark that exited 0 on
			// a failed trial would be a green build for work that did not happen.
			return fmt.Errorf("%d of %d trial(s) failed", failed, len(results))
		}
		return nil
	}
}

// runTrials runs every configuration, up to concurrency of them at a time, and
// returns the results in the order the configurations were given — a report
// whose order depended on which trial happened to finish first would be a
// different document on every run.
//
// With --fail-fast the first failure cancels the trials still running and stops
// the ones not yet started: the point of the flag is not to spend tokens on a
// run whose verdict is already known.
func runTrials(ctx context.Context, configs []*configuration.Config, concurrency int, failFast bool, options runner.TrialOptions) []runner.TrialResult {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]runner.TrialResult, len(configs))
	ran := make([]bool, len(configs))
	slots := make(chan struct{}, concurrency)
	var wait sync.WaitGroup
	var mutex sync.Mutex

	for index, config := range configs {
		// The cancelled context is the stop signal: fail-fast cancels, so a
		// second flag saying the same thing could only drift from it.
		if ctx.Err() != nil {
			break
		}
		slots <- struct{}{}
		wait.Add(1)
		go func() {
			defer wait.Done()
			defer func() { <-slots }()
			// Checked again here, not only before the slot was taken: waiting
			// for a slot is exactly when an earlier trial fails, and a
			// fail-fast run that started the next trial anyway would spend
			// tokens on a verdict already decided.
			if ctx.Err() != nil {
				return
			}
			result := runner.RunTrial(ctx, config, options)
			mutex.Lock()
			defer mutex.Unlock()
			results[index] = result
			ran[index] = true
			if failFast && !result.Passed {
				cancel()
			}
		}()
	}
	wait.Wait()

	// Only the trials that ran are reported. A configuration fail-fast never
	// reached did not pass and did not fail; recording it either way would be
	// a verdict nobody measured.
	reported := make([]runner.TrialResult, 0, len(results))
	for index, result := range results {
		if ran[index] {
			reported = append(reported, result)
		}
	}
	return reported
}

// writeRunReports writes each trial's report into the directory its own
// configuration named, in the formats that configuration asked for. Reports
// live beside the configuration that produced them, so a trial's history stays
// with the trial rather than in one directory shared by everything.
func writeRunReports(configs []*configuration.Config, results []runner.TrialResult, output string, options report.ReportOptions, out io.Writer) error {
	byPath := map[string]*configuration.Config{}
	for _, config := range configs {
		byPath[config.Path] = config
	}
	for _, result := range results {
		config, ok := byPath[result.ConfigPath]
		if !ok {
			continue
		}
		// --output is written too, so `-o markdown` leaves the document it
		// printed on disk rather than only on the terminal.
		formats := append(append([]string{}, config.Report.Formats...), output)
		written, err := report.WriteReports(config.Resolve(config.Report.Dir), result.Name, formats, []runner.TrialResult{result}, options)
		if err != nil {
			return err
		}
		for _, path := range written {
			fmt.Fprintf(out, "report %s\n", path)
		}
	}
	return nil
}

// lockedWriter serializes the writes of trials running at the same time. The
// dialogue of a parallel run is still interleaved trial by trial — nothing can
// unpick that — but no single line is torn in half by another trial's.
type lockedWriter struct {
	mutex  sync.Mutex
	writer io.Writer
}

func newLockedWriter(writer io.Writer) io.Writer {
	if writer == nil {
		writer = os.Stdout
	}
	return &lockedWriter{writer: writer}
}

func (w *lockedWriter) Write(data []byte) (int, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.writer.Write(data)
}

// applyRunOverrides folds the command line into every selected configuration,
// so `--prompt` means the same thing whether one config was named or twenty.
func applyRunOverrides(cmd *cli.Command, configs []*configuration.Config) error {
	prompts, err := overridePrompts(cmd)
	if err != nil {
		return err
	}
	for _, config := range configs {
		for _, name := range cmd.StringSlice("profile") {
			if err := config.ApplyProfile(name); err != nil {
				return err
			}
		}
		if value := cmd.String("agent"); value != "" {
			config.Agent.Type = value
		}
		if prompts != nil {
			config.Prompts = prompts
		}
		if value := cmd.String("agent-md"); value != "" {
			config.Workspace.AgentMD = absoluteOverride(value)
		}
		if value := cmd.String("init-script"); value != "" {
			config.Workspace.InitScript = absoluteOverride(value)
		}
		if value := cmd.String("container-image"); value != "" {
			// The image replaces whatever the configuration said, build
			// included: a run cannot be asked for both.
			config.Container.Image = value
			config.Container.Build = ""
		}
		if value := cmd.String("container-scope"); value != "" {
			if !config.Container.Enabled() {
				return fmt.Errorf("--container-scope needs a container: set one in the config or pass --container-image")
			}
			config.Container.Scope = value
		}
		if values := cmd.StringSlice("verify-command"); len(values) > 0 {
			// Commands are shell text, not paths, so they pass through as
			// typed; anything path-like inside them is the caller's business.
			config.Verify.Commands = values
		}
		if value := cmd.String("mcp-config"); value != "" {
			// The override replaces the configured servers wholesale, and with
			// no agents filter, so what the flag names is what every agent gets.
			config.MCP = []configuration.MCPServerConfig{{Config: absoluteOverride(value)}}
		}
		if cmd.IsSet("timeout") {
			config.Limits.TimeoutSeconds = cmd.Int("timeout")
		}
		if cmd.IsSet("report-dir") {
			config.Report.Dir = absoluteOverride(cmd.String("report-dir"))
		}
		// A profile or a flag may have replaced a section with one whose own
		// fields are empty, and Validate reads them as written.
		config.ApplyDefaults()
		if err := config.Validate(); err != nil {
			return fmt.Errorf("%s: %w", config.Path, err)
		}
	}
	return nil
}

// promptFileScheme marks a --prompt value that names a file rather than the
// text of a turn.
const promptFileScheme = "file://"

// overridePrompts builds the conversation named on the command line, or nil if
// none was. A single repeatable flag carries both kinds of turn so that the
// order the flags were typed in is the order the turns are sent; two flags
// would leave the interleaving undefined.
func overridePrompts(cmd *cli.Command) ([]configuration.Prompt, error) {
	values := cmd.StringSlice("prompt")
	conditions := cmd.StringSlice("prompt-when")
	if len(values) == 0 {
		if len(conditions) > 0 {
			return nil, fmt.Errorf("--prompt-when needs --prompt to attach to")
		}
		return nil, nil
	}
	if len(conditions) > len(values) {
		return nil, fmt.Errorf("%d --prompt-when values for %d prompt(s)", len(conditions), len(values))
	}
	prompts := make([]configuration.Prompt, 0, len(values))
	for _, value := range values {
		path, isFile := strings.CutPrefix(value, promptFileScheme)
		if !isFile {
			prompts = append(prompts, configuration.Prompt{Text: value})
			continue
		}
		if path == "" {
			return nil, fmt.Errorf("--prompt %q names no file", value)
		}
		prompts = append(prompts, configuration.Prompt{File: absoluteOverride(path)})
	}
	for index, condition := range conditions {
		prompts[index].When = condition
	}
	return prompts, nil
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
func loadConfigs(arguments []string) ([]*configuration.Config, error) {
	if len(arguments) == 0 {
		arguments = []string{configuration.DefaultConfigName}
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
	configs := make([]*configuration.Config, 0, len(paths))
	for _, path := range paths {
		config, err := configuration.LoadConfig(path)
		if err != nil {
			return nil, err
		}
		configs = append(configs, config)
	}
	return configs, nil
}
