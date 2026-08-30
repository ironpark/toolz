package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	command "github.com/ironpark/toolz/cli/mohae/cmd"
	"github.com/urfave/cli/v3"
)

// flagsOnly replaces a command's action so running it parses the flags without
// doing the work behind them. A test about what an override reaches has no
// business spending a whole trial to find out.
func flagsOnly(command *cli.Command) *cli.Command {
	command.Action = func(context.Context, *cli.Command) error { return nil }
	return command
}

func runCommandDefinition() *cli.Command {
	return command.NewRun(runAction("test"), DefaultReportDir, DefaultTimeoutSeconds)
}

// runnableProject writes a configuration whose agent is a shell stub, in the
// working directory, and returns that directory. Everything the run command
// needs is on disk and nothing needs the network.
func runnableProject(t *testing.T, name, agentScript, verifyCommand string) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	agent := stubAgent(t, agentScript)
	writeFile(t, filepath.Join(directory, "fixture", "README.md"), "hello\n", 0o644)
	config := "name: " + name + "\n" +
		"agent:\n  type: custom-cli\n  command: [\"" + agent + "\"]\n" +
		"workspace:\n  source: ./fixture\n" +
		"prompts:\n  - do the thing\n" +
		"verify:\n  commands:\n    - " + verifyCommand + "\n"
	writeFile(t, filepath.Join(directory, name+".config.yaml"), config, 0o644)
	return directory
}

// runCommand runs the CLI and captures what it printed, which is the only way
// to assert on a report meant for a terminal.
func runCommand(t *testing.T, arguments ...string) (string, error) {
	t.Helper()
	out := &bytes.Buffer{}
	command := NewCommand("test")
	command.Writer = out
	command.ErrWriter = out
	err := command.Run(context.Background(), append([]string{"mohae"}, arguments...))
	return out.String(), err
}

func TestRunReportsAPassingTrialAndExitsCleanly(t *testing.T) {
	chdir(t)
	runnableProject(t, "passing", "echo done\ntouch delivered.txt\n", `test -f "$MOHAE_WORKSPACE/delivered.txt"`)

	text, err := runCommand(t, "run", "passing.config.yaml")
	if err != nil {
		t.Fatalf("run = %v\n%s", err, text)
	}
	if !strings.Contains(text, "PASS") || !strings.Contains(text, "1/1 passed") {
		t.Errorf("report = %q", text)
	}
}

func TestRunFailsWhenATrialFails(t *testing.T) {
	chdir(t)
	runnableProject(t, "failing", "echo nothing to do\n", `test -f "$MOHAE_WORKSPACE/delivered.txt"`)

	text, err := runCommand(t, "run", "failing.config.yaml")
	// The exit status is what a CI job reads: a green build for a failed trial
	// would be worse than no benchmark at all.
	if err == nil {
		t.Fatalf("run exited cleanly on a failed trial\n%s", text)
	}
	if !strings.Contains(text, "FAIL") {
		t.Errorf("report = %q", text)
	}
	// The workspace is kept so the failure can be looked at.
	if index := strings.Index(text, "workspace: "); index >= 0 {
		path := strings.Fields(text[index+len("workspace: "):])[0]
		defer os.RemoveAll(filepath.Dir(path))
	}
}

func TestRunWritesTheReportsIntoTheConfiguredDirectory(t *testing.T) {
	directory := chdir(t)
	runnableProject(t, "passing", "echo done\n", "true")

	text, err := runCommand(t, "run", "passing.config.yaml", "--output", "json", "--report-dir", "reports")
	if err != nil {
		t.Fatalf("run = %v\n%s", err, text)
	}
	entries, err := os.ReadDir(filepath.Join(directory, "reports"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no report was written")
	}
	// --output is also what is printed, so a run can be piped into another tool.
	document := reportDocument{}
	if err := json.Unmarshal([]byte(text[strings.Index(text, "{"):strings.LastIndex(text, "}")+1]), &document); err != nil {
		t.Fatalf("the printed json does not parse: %v\n%s", err, text)
	}
	if document.Total != 1 || document.Passed != 1 {
		t.Errorf("document = %+v", document)
	}
}

func TestWriteRunReportsResolvesTheDirectoryFromTheConfig(t *testing.T) {
	directory := t.TempDir()
	config := &Config{
		Path: filepath.Join(directory, "nested", "trial.config.yaml"),
		Name: "trial",
		Report: ReportConfig{
			Dir:     "reports",
			Formats: []string{"json"},
		},
	}
	result := TrialResult{Name: "trial", ConfigPath: config.Path, Passed: true}
	if err := writeRunReports([]*Config{config}, []TrialResult{result}, "terminal", ReportOptions{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(directory, "nested", "reports"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("wrote %d reports, want 1", len(entries))
	}
}

func TestRunFailFastStopsAtTheFirstFailure(t *testing.T) {
	chdir(t)
	// Named so the glob picks the failing one up first: a fail-fast run that
	// continued past it would spend tokens on a verdict already decided.
	runnableProject(t, "a-failing", "echo nothing\n", "false")
	runnableProject(t, "b-passing", "echo done\n", "true")

	text, err := runCommand(t, "run", "*.config.yaml", "--fail-fast")
	if err == nil {
		t.Fatalf("run exited cleanly\n%s", text)
	}
	if strings.Contains(text, "b-passing") {
		t.Errorf("fail-fast ran the trial after the failure:\n%s", text)
	}
	if index := strings.Index(text, "workspace: "); index >= 0 {
		defer os.RemoveAll(filepath.Dir(strings.Fields(text[index+len("workspace: "):])[0]))
	}
}

func TestRunWithoutFailFastRunsEveryTrial(t *testing.T) {
	chdir(t)
	runnableProject(t, "a-failing", "echo nothing\n", "false")
	runnableProject(t, "b-passing", "echo done\n", "true")

	text, err := runCommand(t, "run", "*.config.yaml")
	if err == nil {
		t.Fatalf("run exited cleanly on a failed trial\n%s", text)
	}
	for _, want := range []string{"a-failing", "b-passing", "1/2 passed"} {
		if !strings.Contains(text, want) {
			t.Errorf("report is missing %q:\n%s", want, text)
		}
	}
	if index := strings.Index(text, "workspace: "); index >= 0 {
		defer os.RemoveAll(filepath.Dir(strings.Fields(text[index+len("workspace: "):])[0]))
	}
}

func TestRunConcurrentlyKeepsTheReportInConfigurationOrder(t *testing.T) {
	chdir(t)
	// The slow trial is first, so a report ordered by completion would put them
	// the other way round and read differently on every run.
	runnableProject(t, "a-slow", "sleep 0.5\necho done\n", "true")
	runnableProject(t, "b-quick", "echo done\n", "true")

	text, err := runCommand(t, "run", "*.config.yaml", "--concurrency", "2")
	if err != nil {
		t.Fatalf("run = %v\n%s", err, text)
	}
	slow, quick := strings.Index(text, "a-slow"), strings.Index(text, "b-quick")
	if slow < 0 || quick < 0 || slow > quick {
		t.Errorf("report = %q, want the configuration order", text)
	}
	if !strings.Contains(text, "2/2 passed") {
		t.Errorf("report = %q", text)
	}
}

func TestRunShowDialogueStreamsTheConversation(t *testing.T) {
	chdir(t)
	runnableProject(t, "passing", "echo working on it\n", "true")

	text, err := runCommand(t, "run", "passing.config.yaml", "--show-dialogue")
	if err != nil {
		t.Fatalf("run = %v\n%s", err, text)
	}
	for _, want := range []string{"do the thing", "working on it"} {
		if !strings.Contains(text, want) {
			t.Errorf("dialogue is missing %q:\n%s", want, text)
		}
	}
}

func TestRunDetailedTokensChangesTheRendering(t *testing.T) {
	chdir(t)
	runnableProject(t, "passing", "echo done\n", "true")

	text, err := runCommand(t, "run", "passing.config.yaml", "--detailed-tokens")
	if err != nil {
		t.Fatalf("run = %v\n%s", err, text)
	}
	if !strings.Contains(text, "cache read") {
		t.Errorf("--detailed-tokens did not break the usage down:\n%s", text)
	}
}

func TestRunRejectsWhatItCannotDo(t *testing.T) {
	chdir(t)
	runnableProject(t, "passing", "echo done\n", "true")
	cases := [][]string{
		{"run", "passing.config.yaml", "--output", "carrier-pigeon"},
		{"run", "passing.config.yaml", "--concurrency", "0"},
		// Running the whole benchmark and only then admitting the dashboard
		// does not exist would waste every token it spent.
		{"run", "passing.config.yaml", "--web"},
	}
	for _, arguments := range cases {
		t.Run(strings.Join(arguments[2:], " "), func(t *testing.T) {
			if _, err := runCommand(t, arguments...); err == nil {
				t.Fatal("expected the run to be refused")
			}
		})
	}
}
