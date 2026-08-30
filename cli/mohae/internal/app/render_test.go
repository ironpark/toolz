package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sampleResults are one passing and one failing trial, which is enough to
// exercise everything a renderer decides: the verdict, the totals, and the fact
// that only failures carry their detail.
func sampleResults() []TrialResult {
	return []TrialResult{
		{
			Name: "passing", Agent: "custom-cli", StartedAt: time.Now(), DurationSeconds: 1.5,
			Turns: []TurnResult{
				{Index: 1, Sent: true, Prompt: "do the thing", Response: "did it", Model: "stub-1",
					Usage: TokenUsage{Input: 100, Output: 20, CacheRead: 5, CacheWrite: 1}, DurationSeconds: 1.2},
				{Index: 2, Skipped: "when: false"},
			},
			Verify:      []VerifyResult{{Command: "true", Passed: true}},
			ArtifactDir: "/tmp/mohae-reports/passing.artifacts",
			Artifacts:   []ArtifactResult{{Pattern: "plans/**", Paths: []string{"plans"}}},
			Usage:       TokenUsage{Input: 100, Output: 20, CacheRead: 5, CacheWrite: 1, CostUSD: 0.02},
			Passed:      true,
		},
		{
			Name: "failing", Agent: "custom-cli", DurationSeconds: 3,
			Turns:     []TurnResult{{Index: 1, Sent: true, Prompt: "do the thing", Response: "gave up"}},
			Hooks:     []HookResult{{Command: "./finalize.sh", Scope: HookScopeOutside, ExitCode: 3, Output: "finalize failed"}},
			Verify:    []VerifyResult{{Command: "test -f out.txt", ExitCode: 1, Output: "out.txt is missing"}},
			MCP:       []MCPProbe{{Name: "files", Error: "connection refused"}},
			Workspace: "/tmp/mohae-failing/workspace",
		},
	}
}

func TestTerminalReportLeadsWithTheVerdictAndExplainsOnlyFailures(t *testing.T) {
	text, err := RenderReport("terminal", sampleResults(), ReportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"PASS", "FAIL",
		"1/2 passed",
		"verify failed (exit 1): test -f out.txt",
		"hook after failed (outside, exit 3): ./finalize.sh",
		"finalize failed",
		"out.txt is missing",
		// A failed trial's workspace is the only record of what the agent did.
		"/tmp/mohae-failing/workspace",
		// A server that never came up explains a failure that would otherwise
		// read as the agent's.
		"mcp files unreachable: connection refused",
		"/tmp/mohae-reports/passing.artifacts",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("terminal report is missing %q:\n%s", want, text)
		}
	}
	// A passing trial needs no explanation, so its transcript stays off screen.
	if strings.Contains(text, "did it") {
		t.Errorf("the passing trial's dialogue was printed without --show-dialogue:\n%s", text)
	}
}

func TestDetailedTokensBreaksTheUsageDown(t *testing.T) {
	plain, _ := RenderReport("terminal", sampleResults(), ReportOptions{})
	detailed, _ := RenderReport("terminal", sampleResults(), ReportOptions{DetailedTokens: true})
	if !strings.Contains(plain, "126 tokens") {
		t.Errorf("the default report should total the tokens:\n%s", plain)
	}
	// The categories cost different amounts, which is the whole point of asking.
	for _, want := range []string{"in 100", "out 20", "cache read 5", "cache write 1"} {
		if !strings.Contains(detailed, want) {
			t.Errorf("--detailed-tokens report is missing %q:\n%s", want, detailed)
		}
	}
}

func TestShowDialogueIncludesTheConversation(t *testing.T) {
	text, err := RenderReport("markdown", sampleResults(), ReportOptions{ShowDialogue: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "did it") || !strings.Contains(text, "do the thing") {
		t.Errorf("markdown report is missing the conversation:\n%s", text)
	}
}

func TestJSONReportRoundTripsEveryResult(t *testing.T) {
	text, err := RenderReport("json", sampleResults(), ReportOptions{Version: "v-test"})
	if err != nil {
		t.Fatal(err)
	}
	document := reportDocument{}
	if err := json.Unmarshal([]byte(text), &document); err != nil {
		t.Fatalf("the json report does not parse: %v\n%s", err, text)
	}
	if document.Total != 2 || document.Passed != 1 {
		t.Errorf("totals = %d/%d, want 1/2", document.Passed, document.Total)
	}
	if document.Version != "v-test" {
		t.Errorf("version = %q, want v-test", document.Version)
	}
	// The json format is what `compare` and the dashboard will read, so it
	// carries the whole trial rather than a summary of it.
	if got := document.Trials[0].Turns[0].Response; got != "did it" {
		t.Errorf("response = %q", got)
	}
	if document.Trials[1].Verify[0].Output != "out.txt is missing" {
		t.Errorf("verify output did not survive: %+v", document.Trials[1].Verify)
	}
	// The run's totals are the trials' usage summed; the failing trial spent
	// nothing the driver reported.
	if document.Usage.Total() != 126 {
		t.Errorf("aggregate usage = %d", document.Usage.Total())
	}
}

func TestMarkdownAndHTMLReportsCarryEveryTrial(t *testing.T) {
	for _, format := range []string{"markdown", "html"} {
		t.Run(format, func(t *testing.T) {
			text, err := RenderReport(format, sampleResults(), ReportOptions{})
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"passing", "failing", "test -f out.txt"} {
				if !strings.Contains(text, want) {
					t.Errorf("%s report is missing %q:\n%s", format, want, text)
				}
			}
		})
	}
}

func TestHTMLReportEscapesWhatTheAgentWrote(t *testing.T) {
	// A report is something people open in a browser, and an agent's reply is
	// untrusted text that must not become markup in it.
	results := sampleResults()
	results[0].Turns[0].Response = "<script>alert('x')</script>"
	text, err := RenderReport("html", results, ReportOptions{ShowDialogue: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "<script>alert") {
		t.Errorf("the agent's reply was not escaped:\n%s", text)
	}
	if !strings.Contains(text, "&lt;script&gt;") {
		t.Errorf("the reply is missing from the report entirely:\n%s", text)
	}
}

func TestRenderReportRejectsAnUnknownFormat(t *testing.T) {
	if _, err := RenderReport("carrier-pigeon", sampleResults(), ReportOptions{}); err == nil {
		t.Fatal("expected an unknown format to be refused")
	}
}

func TestWriteReportsWritesOneFilePerFormat(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "reports")
	paths, err := WriteReports(directory, "", []string{"terminal", "json", "markdown", "html", "json"}, sampleResults(), ReportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// terminal is a rendering for a screen, and the repeated json is one report.
	if len(paths) != 3 {
		t.Fatalf("wrote %v, want one file each for json, markdown and html", paths)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) == 0 {
			t.Errorf("%s is empty", path)
		}
		// Named for when the run started, so a benchmark's history accumulates
		// instead of overwriting itself.
		if !strings.HasPrefix(filepath.Base(path), "run-") {
			t.Errorf("%s is not named for its run", path)
		}
	}
}

// TestWriteReportsDoesNotOverwriteAnotherTrialsReport pins the naming: several
// configurations share one report.dir by default, so two trials finishing in
// the same second must not resolve to the same file.
func TestWriteReportsDoesNotOverwriteAnotherTrialsReport(t *testing.T) {
	directory := t.TempDir()
	paths := map[string]bool{}
	for _, name := range []string{"first", "second", "first"} {
		written, err := WriteReports(directory, name, []string{"json"}, sampleResults(), ReportOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(written) != 1 {
			t.Fatalf("wrote %d files, want 1", len(written))
		}
		if paths[written[0]] {
			t.Fatalf("%s was written twice, overwriting the earlier report", written[0])
		}
		paths[written[0]] = true
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Errorf("%d report files on disk, want 3", len(entries))
	}
}
