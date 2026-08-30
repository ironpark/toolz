package main

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
	"sync/atomic"

	"github.com/urfave/cli/v3"
)

const DefaultTimeoutSeconds = 300

func runAction(ctx context.Context, cmd *cli.Command) error {
	configs, err := loadConfigs(cmd.Args().Slice())
	if err != nil {
		return err
	}
	if err := applyRunOverrides(cmd, configs); err != nil {
		return err
	}
	output := cmd.String("output")
	if !slices.Contains(KnownFormats, output) {
		return fmt.Errorf("unknown output format %q", output)
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

	reportOptions := ReportOptions{
		DetailedTokens: cmd.Bool("detailed-tokens"),
		ShowDialogue:   cmd.Bool("show-dialogue"),
	}
	trialOptions := TrialOptions{
		ShowDialogue: cmd.Bool("show-dialogue"),
		// Serialized: with --concurrency the trials write at the same time, and
		// unsynchronised writes would tear each other's lines apart.
		Out: newLockedWriter(cmd.Writer),
	}

	// Before the run, so a machine that has been benchmarking for weeks does not
	// carry every failed trial's copy forever. Reported rather than silent: the
	// directories being reclaimed are debugging material.
	if pruned := PruneStaleWorkspaces(StaleWorkspaceAge); pruned > 0 {
		fmt.Fprintf(cmd.Writer, "pruned %d workspace(s) left by earlier runs\n", pruned)
	}

	results := runTrials(ctx, configs, concurrency, cmd.Bool("fail-fast"), trialOptions)

	rendered, err := RenderReport(output, results, reportOptions)
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

// runTrials runs every configuration, up to concurrency of them at a time, and
// returns the results in the order the configurations were given — a report
// whose order depended on which trial happened to finish first would be a
// different document on every run.
//
// With --fail-fast the first failure cancels the trials still running and stops
// the ones not yet started: the point of the flag is not to spend tokens on a
// run whose verdict is already known.
func runTrials(ctx context.Context, configs []*Config, concurrency int, failFast bool, options TrialOptions) []TrialResult {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]TrialResult, len(configs))
	ran := make([]bool, len(configs))
	slots := make(chan struct{}, concurrency)
	var wait sync.WaitGroup
	var mutex sync.Mutex
	// Its own flag rather than a field under mutex: the mutex guards the result
	// slices, and the two are read at different times by different goroutines.
	var stopped atomic.Bool

	for index, config := range configs {
		if stopped.Load() {
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
			if stopped.Load() {
				return
			}
			result := RunTrial(ctx, config, options)
			mutex.Lock()
			defer mutex.Unlock()
			results[index] = result
			ran[index] = true
			if failFast && !result.Passed {
				stopped.Store(true)
				cancel()
			}
		}()
	}
	wait.Wait()

	// Only the trials that ran are reported. A configuration fail-fast never
	// reached did not pass and did not fail; recording it either way would be
	// a verdict nobody measured.
	reported := make([]TrialResult, 0, len(results))
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
func writeRunReports(configs []*Config, results []TrialResult, output string, options ReportOptions, out io.Writer) error {
	byPath := map[string]*Config{}
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
		written, err := WriteReports(config.Resolve(config.Report.Dir), result.Name, formats, []TrialResult{result}, options)
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
func applyRunOverrides(cmd *cli.Command, configs []*Config) error {
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
		if values := cmd.StringSlice("verify-command"); len(values) > 0 {
			// Commands are shell text, not paths, so they pass through as
			// typed; anything path-like inside them is the caller's business.
			config.Verify.Commands = values
		}
		if value := cmd.String("mcp-config"); value != "" {
			// The override replaces the configured servers wholesale, and with
			// no agents filter, so what the flag names is what every agent gets.
			config.MCP = []MCPServerConfig{{Config: absoluteOverride(value)}}
		}
		if cmd.IsSet("timeout") {
			config.Limits.TimeoutSeconds = cmd.Int("timeout")
		}
		if cmd.IsSet("report-dir") {
			config.Report.Dir = absoluteOverride(cmd.String("report-dir"))
		}
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
func overridePrompts(cmd *cli.Command) ([]Prompt, error) {
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
	prompts := make([]Prompt, 0, len(values))
	for _, value := range values {
		path, isFile := strings.CutPrefix(value, promptFileScheme)
		if !isFile {
			prompts = append(prompts, Prompt{Text: value})
			continue
		}
		if path == "" {
			return nil, fmt.Errorf("--prompt %q names no file", value)
		}
		prompts = append(prompts, Prompt{File: absoluteOverride(path)})
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
