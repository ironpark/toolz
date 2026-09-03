package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/ironpark/toolz/cli/planr/internal/apply"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/jsonout"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/ironpark/toolz/cli/planr/internal/planlock"
	"github.com/ironpark/toolz/cli/planr/internal/plantest"
	"github.com/ironpark/toolz/cli/planr/internal/schema"
	ucli "github.com/urfave/cli/v3"
)

func TestRootCommandRemovedWriteAliasesAndAddsNewSurface(t *testing.T) {
	root := newRootCommand()
	commands := map[string]bool{}
	for _, command := range root.Commands {
		commands[command.Name] = true
	}
	for _, name := range []string{"new", "edit", "apply", "schema", "show"} {
		if !commands[name] {
			t.Fatalf("root command %q is missing", name)
		}
	}
	if commands["add"] {
		t.Fatal("removed root add command is still registered")
	}
	var phaseCommand *ucli.Command
	for _, command := range root.Commands {
		if command.Name == "phase" {
			phaseCommand = command
		}
	}
	if phaseCommand == nil {
		t.Fatal("phase command is missing")
	}
	for _, command := range phaseCommand.Commands {
		if command.Name == "add" {
			t.Fatal("removed phase command is still registered")
		}
	}
}

func TestJSONDocumentEnvelopeCanFlowIntoApplyStdin(t *testing.T) {
	template := "---\nplan_name: demo\n---\n# document\n"
	encoded, err := json.Marshal(jsonout.Template(apply.KindPlan, "demo", template))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(apply.UnwrapJSONDocument(encoded)); got != template {
		t.Fatalf("apply.UnwrapJSONDocument(template) = %q, want %q", got, template)
	}
	document := "---\nplanr_edit: demo#0\n---\n# phase\n"
	encoded, err = json.Marshal(jsonout.EditOutput{Document: document})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(apply.UnwrapJSONDocument(encoded)); got != document {
		t.Fatalf("apply.UnwrapJSONDocument(document) = %q, want %q", got, document)
	}
}

func TestStdinOnlyLifecycleUsesNewEditAndApply(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatalf("git.PlainInit() unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dir: plans\nlanguage: en\n"), 0644); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, root)

	plan, err := doc.RenderNewDraft(doc.English, "stdin-plan", nil, "a plan applied over stdin")
	if err != nil {
		t.Fatal(err)
	}
	plan = strings.ReplaceAll(plan, draft.Placeholder, "stdin lifecycle content")
	if output, err := runRootWithStdin(t, plan, "apply", "--stdin", "--no-hooks"); err != nil {
		t.Fatalf("apply plan over stdin: %v; output=%q", err, output)
	}

	phase, err := doc.RenderNewPhaseDraft(doc.English, "stdin-plan", "Second Phase", "second-phase")
	if err != nil {
		t.Fatal(err)
	}
	phase = strings.ReplaceAll(phase, draft.Placeholder, "stdin phase content")
	if output, err := runRootWithStdin(t, phase, "apply", "--stdin", "--no-hooks"); err != nil {
		t.Fatalf("apply phase over stdin: %v; output=%q", err, output)
	}

	checkoutOutput, err := captureOutput(t, func() error {
		return newRootCommand().Run(context.Background(), []string{"planr", "edit", "stdin-plan#1", "--json"})
	})
	if err != nil {
		t.Fatalf("edit phase as JSON: %v", err)
	}
	var checkout jsonout.EditOutput
	if err := json.Unmarshal([]byte(checkoutOutput), &checkout); err != nil {
		t.Fatalf("decode edit checkout: %v; output=%q", err, checkoutOutput)
	}
	checkout.Document = strings.Replace(checkout.Document, "stdin phase content", "edited over stdin", 1)
	encoded, err := json.Marshal(checkout)
	if err != nil {
		t.Fatal(err)
	}
	if output, err := runRootWithStdin(t, string(encoded), "apply", "--stdin", "--json", "--no-hooks"); err != nil {
		t.Fatalf("apply phase edit over stdin: %v; output=%q", err, output)
	}

	raw, err := os.ReadFile(filepath.Join(root, "plans", "00-stdin-plan", "phases", "01-second-phase.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "edited over stdin") {
		t.Fatalf("stdin edit was not applied:\n%s", raw)
	}
}

func TestNewJSONProducesPlanAndPhaseTemplatesWithoutFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dir: plans\nlanguage: ko\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := plan.Write(filepath.Join(root, "plans", "00-checkout-v2"), plantest.ApplyDraft("checkout-v2"), "00-checkout-v2", doc.Korean); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, root)

	output, err := captureOutput(t, func() error {
		return newNewTestCommand().Run(context.Background(), []string{"new", "new-plan", "--description", "a new plan", "--json"})
	})
	if err != nil {
		t.Fatalf("new plan --json: %v", err)
	}
	var planTemplate jsonout.TemplateOutput
	if err := json.Unmarshal([]byte(output), &planTemplate); err != nil {
		t.Fatalf("decode plan template: %v; output=%q", err, output)
	}
	if planTemplate.Kind != apply.KindPlan || planTemplate.Selector != "new-plan" || !strings.Contains(planTemplate.Template, "plan_name: new-plan") {
		t.Fatalf("plan template = %+v", planTemplate)
	}
	if _, err := os.Stat(filepath.Join(root, "new-plan.md")); !os.IsNotExist(err) {
		t.Fatalf("new --json wrote a draft file; stat error=%v", err)
	}

	output, err = captureOutput(t, func() error {
		return newNewTestCommand().Run(context.Background(), []string{"new", "checkout-v2#Second Phase", "--json"})
	})
	if err != nil {
		t.Fatalf("new phase --json: %v", err)
	}
	var phaseTemplate jsonout.TemplateOutput
	if err := json.Unmarshal([]byte(output), &phaseTemplate); err != nil {
		t.Fatalf("decode phase template: %v; output=%q", err, output)
	}
	if phaseTemplate.Kind != apply.KindPhase || phaseTemplate.Selector != "checkout-v2#Second Phase" || !strings.Contains(phaseTemplate.Template, "planr_new: phase") {
		t.Fatalf("phase template = %+v", phaseTemplate)
	}
	if _, err := os.Stat(filepath.Join(root, "checkout-v2-second-phase.md")); !os.IsNotExist(err) {
		t.Fatalf("new phase --json wrote a draft file; stat error=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "plans", "00-checkout-v2", planlock.FileName)); !os.IsNotExist(err) {
		t.Fatalf("new phase --json wrote a lock file; stat error=%v", err)
	}
}

func TestShowSectionsAllAndSchemaReturnMachineReadableDocuments(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dir: plans\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := plan.Write(filepath.Join(root, "plans", "00-checkout-v2"), plantest.ApplyDraft("checkout-v2"), "00-checkout-v2", doc.English); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, root)

	output, err := captureOutput(t, func() error {
		return newShowTestCommand().Run(context.Background(), []string{"show", "checkout-v2", "--section", "goals", "--json"})
	})
	if err != nil {
		t.Fatalf("show section: %v", err)
	}
	var section jsonout.ShowSectionOutput
	if err := json.Unmarshal([]byte(output), &section); err != nil {
		t.Fatalf("decode section: %v; output=%q", err, output)
	}
	if section.Section != "goals" || !strings.Contains(section.Content, "Ship the test plan.") || section.File == "" {
		t.Fatalf("section = %+v", section)
	}

	output, err = captureOutput(t, func() error {
		return newShowTestCommand().Run(context.Background(), []string{"show", "checkout-v2", "--all", "--json"})
	})
	if err != nil {
		t.Fatalf("show all: %v", err)
	}
	var all jsonout.ShowAllOutput
	if err := json.Unmarshal([]byte(output), &all); err != nil {
		t.Fatalf("decode show all: %v; output=%q", err, output)
	}
	if all.Plan != "checkout-v2" || len(all.Phases) != 2 || !strings.Contains(all.Documents["PLAN.md"], "Phase 00") {
		t.Fatalf("show all = %+v", all)
	}

	var schema schema.Output
	output, err = captureOutput(t, func() error {
		return newSchemaTestCommand().Run(context.Background(), []string{"schema", "--json"})
	})
	if err != nil {
		t.Fatalf("schema --json: %v", err)
	}
	if err := json.Unmarshal([]byte(output), &schema); err != nil {
		t.Fatalf("decode schema: %v; output=%q", err, output)
	}
	if schema.Name == "" || len(schema.RequiredPlanSections) == 0 || len(schema.PhaseStatuses) != 4 || len(schema.ApplyKinds) != 3 {
		t.Fatalf("schema = %+v", schema)
	}
}

func newNewTestCommand() *ucli.Command {
	return &ucli.Command{
		Name: "new",
		Flags: []ucli.Flag{
			&ucli.StringFlag{Name: "description"},
			&ucli.StringSliceFlag{Name: "depends-on"},
			&ucli.StringFlag{Name: "output"},
			&ucli.BoolFlag{Name: "json"},
			&ucli.BoolFlag{Name: "no-hooks"},
		},
		Action: runNew,
	}
}

func newShowTestCommand() *ucli.Command {
	return &ucli.Command{
		Name: "show",
		Flags: []ucli.Flag{
			&ucli.StringFlag{Name: "section"},
			&ucli.BoolFlag{Name: "all"},
			&ucli.BoolFlag{Name: "json"},
		},
		Action: runShow,
	}
}

func newSchemaTestCommand() *ucli.Command {
	return &ucli.Command{
		Name:   "schema",
		Flags:  []ucli.Flag{&ucli.BoolFlag{Name: "json"}},
		Action: runSchema,
	}
}

func captureOutput(t *testing.T, function func() error) (string, error) {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = write
	callErr := function()
	if closeErr := write.Close(); callErr == nil {
		callErr = closeErr
	}
	os.Stdout = original
	output, readErr := io.ReadAll(read)
	_ = read.Close()
	if callErr == nil {
		callErr = readErr
	}
	return string(output), callErr
}

func runRootWithStdin(t *testing.T, input string, args ...string) (string, error) {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := write.WriteString(input); err != nil {
		_ = read.Close()
		_ = write.Close()
		t.Fatal(err)
	}
	if err := write.Close(); err != nil {
		_ = read.Close()
		t.Fatal(err)
	}
	original := os.Stdin
	os.Stdin = read
	defer func() {
		os.Stdin = original
		_ = read.Close()
	}()
	return runRoot(t, args...)
}
