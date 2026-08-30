package report

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"strings"
	"time"

	"github.com/ironpark/toolz/cli/mohae/internal/fsutil"
	reportformat "github.com/ironpark/toolz/cli/mohae/internal/report/format"
	"github.com/ironpark/toolz/cli/mohae/internal/runner"
)

// ReportOptions are the presentation choices a report takes from the command
// line. They change how a result is shown and never what it says.
type ReportOptions struct {
	// Version is recorded in machine-readable reports.
	Version string
	// DetailedTokens breaks the usage down into input, output, cache read and
	// cache write instead of showing one total. The categories cost different
	// amounts, so a benchmark comparing agents often needs them apart.
	DetailedTokens bool
	// ShowDialogue includes the prompts and responses in the rendering. The
	// json format always carries them; the readable ones would otherwise be
	// pages of conversation where a verdict was wanted.
	ShowDialogue bool
}

// reportFormats connects each name from the format package to its renderer and,
// if it can be written to disk, its suffix. A consistency test keeps this
// executable registry aligned with the names accepted by config and flags.
//
// terminal has no extension on purpose: it is a rendering for a screen, and
// writing it to a file would produce something no other tool can read.
var reportFormats = map[string]struct {
	extension string
	render    func([]runner.TrialResult, ReportOptions) (string, error)
}{
	reportformat.Terminal: {"", func(r []runner.TrialResult, o ReportOptions) (string, error) { return renderTerminal(r, o), nil }},
	reportformat.JSON:     {".json", renderJSON},
	reportformat.Markdown: {".md", func(r []runner.TrialResult, o ReportOptions) (string, error) { return renderMarkdown(r, o), nil }},
	reportformat.HTML:     {".html", func(r []runner.TrialResult, o ReportOptions) (string, error) { return renderHTML(r, o), nil }},
}

// RenderReport renders a run's results in one format.
func RenderReport(format string, results []runner.TrialResult, options ReportOptions) (string, error) {
	entry, ok := reportFormats[format]
	if !ok {
		return "", fmt.Errorf("unknown report format %q", format)
	}
	return entry.render(results, options)
}

// WriteReports writes every file-backed format into dir and returns the paths
// written. The terminal format is skipped: it is printed as the run happens,
// and a file of it would only duplicate the screen in a form nothing can read.
//
// The file is named for the trial and for when it was written, so a benchmark's
// history accumulates instead of overwriting itself. Both parts are needed:
// several configurations share one report.dir by default, and a stamp alone
// collides between two trials of the same run that finish in the same second.
func WriteReports(dir string, name string, formats []string, results []runner.TrialResult, options ReportOptions) ([]string, error) {
	stem := "run"
	if name != "" {
		stem = sanitizeName(name)
	}
	stem += "-" + time.Now().Format("20060102-150405")
	written := []string{}
	seen := map[string]bool{}
	created := false
	for _, format := range formats {
		entry, ok := reportFormats[format]
		if !ok {
			return written, fmt.Errorf("unknown report format %q", format)
		}
		if entry.extension == "" || seen[format] {
			continue
		}
		seen[format] = true
		content, err := RenderReport(format, results, options)
		if err != nil {
			return written, err
		}
		if !created {
			// Once per call rather than once per format: the directory is the
			// same for all of them, and a run writes reports per trial.
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return written, err
			}
			created = true
		}
		path, err := fsutil.WriteFileUnique(dir, stem, entry.extension, []byte(content), 0o644)
		if err != nil {
			return written, err
		}
		written = append(written, path)
	}
	return written, nil
}

// summarize is a run's headline: how many trials passed and what they cost
// together. Every format shows it, so it is derived in one place — a screen
// report and the json file `compare` reads back must not be able to disagree.
func summarize(results []runner.TrialResult) (passed int, total runner.TokenUsage) {
	for _, result := range results {
		if result.Passed {
			passed++
		}
		total.Add(result.Usage)
	}
	return passed, total
}

// renderTerminal is the rendering a run prints as it finishes: the verdict
// first, then what it cost, then what failed. Anything that passed needs no
// explanation, so only failures carry their detail.
func renderTerminal(results []runner.TrialResult, options ReportOptions) string {
	out := &strings.Builder{}
	for _, result := range results {
		fmt.Fprintf(out, "\n%-7s %s  agent=%s", strings.ToUpper(result.Verdict()), result.Name, result.Agent)
		if model := result.EffectiveModel(); model != "" {
			fmt.Fprintf(out, " model=%s", model)
		}
		fmt.Fprintf(out, "\n        turns=%d/%d  %s  %s\n", result.Sent(), len(result.Turns), usageText(result.Usage, options.DetailedTokens), durationText(result.DurationSeconds))
		if result.Error != "" {
			fmt.Fprintf(out, "        error: %s\n", firstLine(result.Error))
		}
		for _, probe := range result.MCP {
			if !probe.OK {
				// A server that never came up makes the whole trial a
				// measurement of an agent without its tools.
				fmt.Fprintf(out, "        mcp %s unreachable: %s\n", probe.Name, firstLine(probe.Error))
			}
		}
		for _, turn := range result.Turns {
			if options.ShowDialogue && turn.Sent {
				fmt.Fprintf(out, "        %d. %s\n", turn.Index, truncate(turn.Response, 72))
			}
			if turn.Skipped != "" {
				// The turn count alone says a prompt was dropped but not which
				// one or why, which is the first thing a false condition needs.
				fmt.Fprintf(out, "        turn %d skipped: %s\n", turn.Index, turn.Skipped)
				if options.ShowDialogue && turn.Prompt != "" {
					fmt.Fprintf(out, "          %s\n", truncate(firstLine(turn.Prompt), 72))
				}
			}
			if turn.Error != "" {
				fmt.Fprintf(out, "        turn %d failed: %s\n", turn.Index, firstLine(turn.Error))
			}
		}
		for _, hook := range result.Hooks {
			if hook.Passed {
				continue
			}
			fmt.Fprintf(out, "        hook after failed (%s, exit %d): %s\n", hook.Scope, hook.ExitCode, hook.Command)
			for _, line := range outputLines(hook.Output, 5) {
				fmt.Fprintf(out, "          %s\n", line)
			}
		}
		for _, check := range result.Verify {
			if check.Passed {
				continue
			}
			fmt.Fprintf(out, "        verify failed (exit %d): %s\n", check.ExitCode, check.Command)
			for _, line := range outputLines(check.Output, 5) {
				fmt.Fprintf(out, "          %s\n", line)
			}
		}
		if result.ArtifactDir != "" {
			fmt.Fprintf(out, "        artifacts: %s\n", result.ArtifactDir)
		}
		if result.ArtifactError != "" {
			fmt.Fprintf(out, "        artifact error: %s\n", firstLine(result.ArtifactError))
		}
		for _, artifact := range result.Artifacts {
			if len(artifact.Paths) == 0 {
				fmt.Fprintf(out, "        artifact matched nothing: %s\n", artifact.Pattern)
			}
		}
		if result.Workspace != "" {
			// The only way to see what the agent actually did.
			fmt.Fprintf(out, "        workspace: %s\n", result.Workspace)
		}
	}
	passed, total := summarize(results)
	fmt.Fprintf(out, "\n%d/%d passed  %s\n", passed, len(results), usageText(total, options.DetailedTokens))
	return out.String()
}

// reportDocument is the JSON file's shape. The results are wrapped rather than
// written as a bare array so the file can gain fields later without every
// reader having to change.
type reportDocument struct {
	GeneratedAt time.Time            `json:"generated_at"`
	Tool        string               `json:"tool"`
	Version     string               `json:"version"`
	Passed      int                  `json:"passed"`
	Total       int                  `json:"total"`
	Usage       runner.TokenUsage    `json:"usage"`
	Trials      []runner.TrialResult `json:"trials"`
}

func renderJSON(results []runner.TrialResult, options ReportOptions) (string, error) {
	version := options.Version
	if version == "" {
		version = "unknown"
	}
	document := reportDocument{
		GeneratedAt: time.Now(),
		Tool:        "mohae",
		Version:     version,
		Total:       len(results),
		Trials:      results,
	}
	document.Passed, document.Usage = summarize(results)
	// Indented: a report is read by people at least as often as by programs.
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

func renderMarkdown(results []runner.TrialResult, options ReportOptions) string {
	out := &strings.Builder{}
	fmt.Fprintf(out, "# mohae report\n\n%s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(out, "| trial | verdict | agent | turns | tokens | duration |\n")
	fmt.Fprintf(out, "| ----- | ------- | ----- | ----- | ------ | -------- |\n")
	for _, result := range results {
		fmt.Fprintf(out, "| %s | %s | %s | %d/%d | %s | %s |\n",
			result.Name, result.Verdict(), result.Agent,
			result.Sent(), len(result.Turns),
			usageText(result.Usage, options.DetailedTokens), durationText(result.DurationSeconds))
	}
	for _, result := range results {
		fmt.Fprintf(out, "\n## %s — %s\n\n", result.Name, result.Verdict())
		if result.Description != "" {
			fmt.Fprintf(out, "%s\n\n", result.Description)
		}
		if result.Error != "" {
			fmt.Fprintf(out, "- error: `%s`\n", firstLine(result.Error))
		}
		for _, probe := range result.MCP {
			if probe.OK {
				fmt.Fprintf(out, "- mcp `%s`: %d tool(s)\n", probe.Name, len(probe.Tools))
				continue
			}
			fmt.Fprintf(out, "- mcp `%s`: unreachable — %s\n", probe.Name, firstLine(probe.Error))
		}
		for _, turn := range result.Turns {
			if !turn.Sent {
				fmt.Fprintf(out, "- turn %d skipped (%s)\n", turn.Index, turn.Skipped)
				continue
			}
			fmt.Fprintf(out, "- turn %d: %s, %s\n", turn.Index, usageText(turn.Usage, options.DetailedTokens), durationText(turn.DurationSeconds))
			if turn.Error != "" {
				fmt.Fprintf(out, "  - failed: `%s`\n", firstLine(turn.Error))
			}
			if options.ShowDialogue {
				fmt.Fprintf(out, "\n```\n> %s\n\n%s\n```\n\n", strings.TrimSpace(turn.Prompt), strings.TrimSpace(turn.Response))
			}
		}
		for _, hook := range result.Hooks {
			fmt.Fprintf(out, "- hook after %s (%s, exit %d): `%s`\n", verdictWord(hook.Passed), hook.Scope, hook.ExitCode, hook.Command)
			if !hook.Passed && hook.Output != "" {
				fmt.Fprintf(out, "\n```\n%s\n```\n\n", hook.Output)
			}
		}
		for _, check := range result.Verify {
			fmt.Fprintf(out, "- verify %s (exit %d): `%s`\n", verdictWord(check.Passed), check.ExitCode, check.Command)
			if !check.Passed && check.Output != "" {
				fmt.Fprintf(out, "\n```\n%s\n```\n\n", check.Output)
			}
		}
		if result.ArtifactDir != "" {
			fmt.Fprintf(out, "- artifacts: `%s`\n", result.ArtifactDir)
		}
		if result.ArtifactError != "" {
			fmt.Fprintf(out, "- artifact error: `%s`\n", firstLine(result.ArtifactError))
		}
		for _, artifact := range result.Artifacts {
			if len(artifact.Paths) == 0 {
				fmt.Fprintf(out, "- artifact `%s`: matched nothing\n", artifact.Pattern)
			}
		}
		if result.Workspace != "" {
			fmt.Fprintf(out, "- workspace: `%s`\n", result.Workspace)
		}
	}
	return out.String()
}

// renderHTML is a single self-contained document: a report is something people
// send to each other, and one that needed a stylesheet alongside it would
// arrive unreadable.
func renderHTML(results []runner.TrialResult, options ReportOptions) string {
	out := &strings.Builder{}
	out.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>mohae report</title>
<style>
body { font: 15px/1.5 ui-sans-serif, system-ui, sans-serif; margin: 2rem auto; max-width: 60rem; }
table { border-collapse: collapse; width: 100%; }
th, td { border-bottom: 1px solid #ddd; padding: .4rem .6rem; text-align: left; }
pre { background: #f6f6f6; padding: .6rem; overflow-x: auto; }
.pass { color: #157f3b; } .fail { color: #b3261e; } .timeout { color: #8a6d00; }
</style>
</head>
<body>
<h1>mohae report</h1>
`)
	fmt.Fprintf(out, "<p>%s</p>\n<table>\n<tr><th>trial</th><th>verdict</th><th>agent</th><th>turns</th><th>tokens</th><th>duration</th></tr>\n", html.EscapeString(time.Now().Format(time.RFC3339)))
	for _, result := range results {
		fmt.Fprintf(out, "<tr><td>%s</td><td class=\"%s\">%s</td><td>%s</td><td>%d/%d</td><td>%s</td><td>%s</td></tr>\n",
			html.EscapeString(result.Name), result.Verdict(), result.Verdict(), html.EscapeString(result.Agent),
			result.Sent(), len(result.Turns),
			html.EscapeString(usageText(result.Usage, options.DetailedTokens)), durationText(result.DurationSeconds))
	}
	out.WriteString("</table>\n")
	for _, result := range results {
		fmt.Fprintf(out, "<h2>%s — <span class=\"%s\">%s</span></h2>\n", html.EscapeString(result.Name), result.Verdict(), result.Verdict())
		if result.Error != "" {
			fmt.Fprintf(out, "<p>error: %s</p>\n", html.EscapeString(firstLine(result.Error)))
		}
		out.WriteString("<ul>\n")
		for _, turn := range result.Turns {
			if !turn.Sent {
				fmt.Fprintf(out, "<li>turn %d skipped (%s)</li>\n", turn.Index, html.EscapeString(turn.Skipped))
				continue
			}
			fmt.Fprintf(out, "<li>turn %d: %s, %s</li>\n", turn.Index, html.EscapeString(usageText(turn.Usage, options.DetailedTokens)), durationText(turn.DurationSeconds))
		}
		for _, hook := range result.Hooks {
			fmt.Fprintf(out, "<li class=\"%s\">hook after %s (%s, exit %d): <code>%s</code></li>\n",
				verdictWord(hook.Passed), verdictWord(hook.Passed), html.EscapeString(hook.Scope), hook.ExitCode, html.EscapeString(hook.Command))
		}
		for _, check := range result.Verify {
			fmt.Fprintf(out, "<li class=\"%s\">verify %s (exit %d): <code>%s</code></li>\n",
				verdictWord(check.Passed), verdictWord(check.Passed), check.ExitCode, html.EscapeString(check.Command))
		}
		if result.ArtifactDir != "" {
			fmt.Fprintf(out, "<li>artifacts: <code>%s</code></li>\n", html.EscapeString(result.ArtifactDir))
		}
		if result.ArtifactError != "" {
			fmt.Fprintf(out, "<li>artifact error: %s</li>\n", html.EscapeString(firstLine(result.ArtifactError)))
		}
		for _, artifact := range result.Artifacts {
			if len(artifact.Paths) == 0 {
				fmt.Fprintf(out, "<li>artifact <code>%s</code>: matched nothing</li>\n", html.EscapeString(artifact.Pattern))
			}
		}
		out.WriteString("</ul>\n")
		if options.ShowDialogue {
			for _, turn := range result.Turns {
				if !turn.Sent {
					continue
				}
				fmt.Fprintf(out, "<pre>&gt; %s\n\n%s</pre>\n", html.EscapeString(strings.TrimSpace(turn.Prompt)), html.EscapeString(strings.TrimSpace(turn.Response)))
			}
		}
		if result.Workspace != "" {
			fmt.Fprintf(out, "<p>workspace: <code>%s</code></p>\n", html.EscapeString(result.Workspace))
		}
	}
	out.WriteString("</body>\n</html>\n")
	return out.String()
}

// usageText renders a turn's or a run's token spend, in one number or broken
// down by category.
func usageText(usage runner.TokenUsage, detailed bool) string {
	if !detailed {
		text := fmt.Sprintf("%d tokens", usage.Total())
		if usage.CostUSD > 0 {
			text += fmt.Sprintf(" ($%.4f)", usage.CostUSD)
		}
		return text
	}
	text := fmt.Sprintf("in %d, out %d, cache read %d, cache write %d", usage.Input, usage.Output, usage.CacheRead, usage.CacheWrite)
	if usage.CostUSD > 0 {
		text += fmt.Sprintf(", $%.4f", usage.CostUSD)
	}
	return text
}

func durationText(seconds float64) string {
	return time.Duration(seconds * float64(time.Second)).Round(100 * time.Millisecond).String()
}

func firstLine(text string) string {
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return text[:index]
	}
	return text
}

// outputLines keeps a failing command's output to a readable size on screen.
// The whole of it is in the json and markdown reports, so nothing is lost.
func outputLines(output string, limit int) []string {
	if output == "" {
		return nil
	}
	lines := strings.Split(output, "\n")
	if len(lines) <= limit {
		return lines
	}
	return append(lines[:limit:limit], fmt.Sprintf("… %d more line(s)", len(lines)-limit))
}
