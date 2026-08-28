package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/validation"
	"github.com/urfave/cli/v3"
)

func applyTestSettings() config.Config {
	return config.Config{
		PlansDirs: []string{"plans"},
		Language:  doc.English,
		Hooks:     hooks.Config{Timeout: hooks.DefaultTimeout},
		SkipHooks: true,
	}
}

func applyTestDraft(name string) draft {
	return draft{
		Name:         name,
		Description:  "a test plan",
		NextPhase:    0,
		NextText:     "Implement the first phase.",
		Goals:        "Ship the test plan.",
		Scope:        "The test scope.",
		Context:      "The test context.",
		Verification: "go test ./...",
		Ordering:     "The first phase comes first.",
		Phases: []draftPhase{
			{Title: "Foundation", Meta: phaseMeta{Phase: 0, Slug: "foundation", Status: "planned"}, Planned: "Build the foundation.", Completion: "Foundation tests pass."},
			{Title: "Follow-up", Meta: phaseMeta{Phase: 1, Slug: "follow-up", Status: "planned", DependsOn: []int{0}}, Planned: "Build the follow-up.", Completion: "Follow-up tests pass."},
		},
	}
}

func filledPhaseDraft(t *testing.T, language string) phaseDraftInput {
	t.Helper()
	raw, err := doc.RenderNewPhaseDraft(language, "checkout-v2", "Cache Warmup", "cache-warmup")
	if err != nil {
		t.Fatalf("doc.RenderNewPhaseDraft() unexpected error: %v", err)
	}
	raw = strings.Replace(raw, "perf_phase: false", "perf_phase: true", 1)
	raw = strings.Replace(raw, "depends_on: []", "depends_on: [1]", 1)
	raw = strings.Replace(raw, "status: planned", "status: conditional", 1)
	raw = strings.Replace(raw, "entry_condition: null", "entry_condition: only after the cache API is ready", 1)
	raw = strings.ReplaceAll(raw, draftPlaceholder, "filled")
	parsed, err := parsePhaseDraft([]byte(raw))
	if err != nil {
		t.Fatalf("parsePhaseDraft() unexpected error: %v", err)
	}
	return parsed
}

func TestApplyPhaseDraftAddsPhaseAndPreservesPhaseFlags(t *testing.T) {
	root := t.TempDir()
	settings := applyTestSettings()
	planRoot := filepath.Join(root, "plans", "00-checkout-v2")
	if err := writePlan(planRoot, applyTestDraft("checkout-v2"), "00-checkout-v2", doc.English); err != nil {
		t.Fatalf("writePlan() unexpected error: %v", err)
	}

	draft := filledPhaseDraft(t, doc.English)
	if _, err := applyPhaseDraft(draft, settings, root, false, false); err != nil {
		t.Fatalf("applyPhaseDraft() unexpected error: %v", err)
	}
	phasePath := filepath.Join(planRoot, "phases", "02-cache-warmup.md")
	raw, err := os.ReadFile(phasePath)
	if err != nil {
		t.Fatalf("read applied phase: %v", err)
	}
	front, _, err := mdoc.Split(string(raw))
	if err != nil {
		t.Fatalf("parse applied phase: %v", err)
	}
	if got, want := front["status"], "conditional"; got != want {
		t.Fatalf("status = %v, want %v", got, want)
	}
	if got, want := front["perf_phase"], true; got != want {
		t.Fatalf("perf_phase = %v, want %v", got, want)
	}
	if !strings.Contains(string(raw), "00-checkout-v2#1") || !strings.Contains(string(raw), "only after the cache API is ready") {
		t.Fatalf("applied phase lost its metadata:\n%s", raw)
	}
	planRaw, err := os.ReadFile(filepath.Join(planRoot, "PLAN.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(planRaw), "[Phase 02: Cache Warmup](phases/02-cache-warmup.md)") {
		t.Fatalf("PLAN.md does not contain the new checklist entry:\n%s", planRaw)
	}
}

func TestApplyPhaseDraftRefusesCompletedPlan(t *testing.T) {
	root := t.TempDir()
	settings := applyTestSettings()
	planRoot := filepath.Join(root, "plans", "00-checkout-v2")
	if err := writePlan(planRoot, applyTestDraft("checkout-v2"), "00-checkout-v2", doc.English); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(planRoot, "PLAN.md")
	raw, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	front, body, err := mdoc.Split(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	front["plan_status"] = "done"
	if err := mdoc.WriteFile(planPath, front, body); err != nil {
		t.Fatal(err)
	}

	err = func() error {
		_, err := applyPhaseDraft(filledPhaseDraft(t, doc.English), settings, root, false, false)
		return err
	}()
	if err == nil || !strings.Contains(err.Error(), "already done") {
		t.Fatalf("applyPhaseDraft() error = %v, want already done", err)
	}
	records := validation.Records(err)
	if len(records) != 1 || records[0].Rule != "plan_done" {
		t.Fatalf("validation records = %#v, want plan_done", records)
	}
}

func TestApplyDryRunDoesNotCreatePlanFiles(t *testing.T) {
	root := t.TempDir()
	settings := applyTestSettings()
	draft := applyTestDraft("dry-run-plan")
	documents, err := renderPlanDocuments(draft, "00-dry-run-plan", settings.Language, "2026-08-27T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	var raw strings.Builder
	raw.WriteString("---\nplan_name: dry-run-plan\ndescription: a test plan\n---\n")
	raw.WriteString("# GOALS\n\nShip the test plan.\n# SCOPE\n\nThe test scope.\n# CONTEXT\n\nThe test context.\n# PHASES\n\n## PHASE — Foundation\n\n```yaml\nphase: 0\nslug: foundation\nstatus: planned\n```\n\n### Planned Work\n\nBuild the foundation.\n\n### Done When\n\nFoundation tests pass.\n# VERIFICATION\n\ngo test ./...\n# ORDERING\n\nThe first phase comes first.\n# NEXT\n\n```yaml\nnext_phase: 0\n```\n\nImplement the first phase.\n")
	parsed, err := parseDraft([]byte(raw.String()), "dry-run-plan.md")
	if err != nil {
		t.Fatalf("parse draft: %v", err)
	}
	if _, err := applyPlanDraft(parsed, settings, root, true, false); err != nil {
		t.Fatalf("applyPlanDraft(dry-run): %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "plans")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created plans directory; stat error = %v", err)
	}
	if len(documents) == 0 {
		t.Fatal("rendered dry-run documents are empty")
	}
}

func TestEditPhaseBaseHashAndStatusSafety(t *testing.T) {
	root := t.TempDir()
	settings := applyTestSettings()
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dir: plans\nlanguage: en\n"), 0644); err != nil {
		t.Fatal(err)
	}
	planRoot := filepath.Join(root, "plans", "00-checkout-v2")
	if err := writePlan(planRoot, applyTestDraft("checkout-v2"), "00-checkout-v2", doc.English); err != nil {
		t.Fatal(err)
	}
	phasePath := filepath.Join(planRoot, "phases", "00-foundation.md")
	checkout := editDocumentForTest(t, root, "checkout-v2#0", "phase.md", "")
	if !strings.Contains(checkout, "planr_base:") || !strings.Contains(checkout, "planr_target:") {
		t.Fatalf("checkout is missing safety metadata:\n%s", checkout)
	}
	updated := strings.Replace(checkout, "Build the foundation.", "Build the safer foundation.", 1)
	if _, err := applyEditDocument([]byte(updated), settings, root, false, false); err != nil {
		t.Fatalf("applyEditDocument() unexpected error: %v", err)
	}
	changed, err := os.ReadFile(phasePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(changed), "Build the safer foundation.") {
		t.Fatalf("phase edit was not applied:\n%s", changed)
	}

	stale := strings.Replace(updated, "Build the safer foundation.", "A stale edit.", 1)
	_, err = applyEditDocument([]byte(stale), settings, root, false, false)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("stale apply error = %v, want hash mismatch", err)
	}
	if records := validation.Records(err); len(records) != 1 || records[0].Rule != "base_mismatch" {
		t.Fatalf("stale validation records = %#v, want base_mismatch", records)
	}

	statusCheckout := editDocumentForTest(t, root, "checkout-v2#0", "status.md", "")
	statusEdited := strings.Replace(statusCheckout, "status: planned", "status: in-progress", 1)
	_, err = applyEditDocument([]byte(statusEdited), settings, root, false, false)
	if err == nil || !strings.Contains(err.Error(), "use `planr phase start`") {
		t.Fatalf("status edit error = %v, want phase start guidance", err)
	}
	if records := validation.Records(err); len(records) != 1 || records[0].Rule != "status_transition" {
		t.Fatalf("status validation records = %#v, want status_transition", records)
	}
}

func TestEditPlanSectionProtectsDerivedChecklist(t *testing.T) {
	root := t.TempDir()
	settings := applyTestSettings()
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dir: plans\nlanguage: en\n"), 0644); err != nil {
		t.Fatal(err)
	}
	planRoot := filepath.Join(root, "plans", "00-checkout-v2")
	if err := writePlan(planRoot, applyTestDraft("checkout-v2"), "00-checkout-v2", doc.English); err != nil {
		t.Fatal(err)
	}
	checkout := editDocumentForTest(t, root, "checkout-v2", "plan.md", "plan")
	bad := strings.Replace(checkout, planChecklistPlaceholder, "- [ ] a hand-written checklist", 1)
	if _, err := applyEditDocument([]byte(bad), settings, root, false, false); err == nil || !strings.Contains(err.Error(), "derived checklist") {
		t.Fatalf("derived-region apply error = %v", err)
	}
	if _, err := applyEditDocument([]byte(checkout), settings, root, false, false); err != nil {
		t.Fatalf("unchanged plan checkout apply: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".planr.lock")); err == nil {
		t.Fatal("unexpected repository-root lock")
	}
}

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
	var phaseCommand *cli.Command
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

func TestStructuredValidationIncludesPlaceholderLocationAndCycle(t *testing.T) {
	raw := renderPlaceholderDraftForTest(t)
	if err := checkDraftPlaceholders(raw); err == nil {
		t.Fatal("placeholder draft unexpectedly passed")
	} else {
		records := validation.Records(err)
		if len(records) != 3 || records[0].Rule != "placeholder" || records[0].Section != "PHASES" || records[0].Phase == nil || records[0].Line == 0 {
			t.Fatalf("placeholder records = %#v", records)
		}
		encoded := makeValidationJSON(records)
		if encoded[0].Rule != "placeholder" || encoded[0].Phase == nil || encoded[0].Line == 0 {
			t.Fatalf("placeholder JSON = %#v", encoded[0])
		}
	}

	err := validatePhaseDependencies([]draftPhase{phaseForTest(1, 3), phaseForTest(3, 1)})
	if err == nil {
		t.Fatal("cycle unexpectedly passed")
	}
	records := validation.Records(err)
	if len(records) != 1 || records[0].Rule != "dependency_cycle" || len(records[0].Phases) != 2 {
		t.Fatalf("cycle records = %#v", records)
	}
}

func TestJSONDocumentEnvelopeCanFlowIntoApplyStdin(t *testing.T) {
	template := "---\nplan_name: demo\n---\n# document\n"
	encoded, err := json.Marshal(makeTemplateJSON(applyKindPlan, "demo", template))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(unwrapJSONDocument(encoded)); got != template {
		t.Fatalf("unwrapJSONDocument(template) = %q, want %q", got, template)
	}
	document := "---\nplanr_edit: demo#0\n---\n# phase\n"
	encoded, err = json.Marshal(editJSONOutput{Document: document})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(unwrapJSONDocument(encoded)); got != document {
		t.Fatalf("unwrapJSONDocument(document) = %q, want %q", got, document)
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
	plan = strings.ReplaceAll(plan, draftPlaceholder, "stdin lifecycle content")
	if output, err := runRootWithStdin(t, plan, []string{"planr", "apply", "--stdin", "--no-hooks"}); err != nil {
		t.Fatalf("apply plan over stdin: %v; output=%q", err, output)
	}

	phase, err := doc.RenderNewPhaseDraft(doc.English, "stdin-plan", "Second Phase", "second-phase")
	if err != nil {
		t.Fatal(err)
	}
	phase = strings.ReplaceAll(phase, draftPlaceholder, "stdin phase content")
	if output, err := runRootWithStdin(t, phase, []string{"planr", "apply", "--stdin", "--no-hooks"}); err != nil {
		t.Fatalf("apply phase over stdin: %v; output=%q", err, output)
	}

	checkoutOutput, err := captureOutput(t, func() error {
		return newRootCommand().Run(context.Background(), []string{"planr", "edit", "stdin-plan#1", "--json"})
	})
	if err != nil {
		t.Fatalf("edit phase as JSON: %v", err)
	}
	var checkout editJSONOutput
	if err := json.Unmarshal([]byte(checkoutOutput), &checkout); err != nil {
		t.Fatalf("decode edit checkout: %v; output=%q", err, checkoutOutput)
	}
	checkout.Document = strings.Replace(checkout.Document, "stdin phase content", "edited over stdin", 1)
	encoded, err := json.Marshal(checkout)
	if err != nil {
		t.Fatal(err)
	}
	if output, err := runRootWithStdin(t, string(encoded), []string{"planr", "apply", "--stdin", "--json", "--no-hooks"}); err != nil {
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
	if err := writePlan(filepath.Join(root, "plans", "00-checkout-v2"), applyTestDraft("checkout-v2"), "00-checkout-v2", doc.Korean); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, root)

	output, err := captureOutput(t, func() error {
		return newNewTestCommand().Run(context.Background(), []string{"new", "new-plan", "--description", "a new plan", "--json"})
	})
	if err != nil {
		t.Fatalf("new plan --json: %v", err)
	}
	var planTemplate templateJSONOutput
	if err := json.Unmarshal([]byte(output), &planTemplate); err != nil {
		t.Fatalf("decode plan template: %v; output=%q", err, output)
	}
	if planTemplate.Kind != applyKindPlan || planTemplate.Selector != "new-plan" || !strings.Contains(planTemplate.Template, "plan_name: new-plan") {
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
	var phaseTemplate templateJSONOutput
	if err := json.Unmarshal([]byte(output), &phaseTemplate); err != nil {
		t.Fatalf("decode phase template: %v; output=%q", err, output)
	}
	if phaseTemplate.Kind != applyKindPhase || phaseTemplate.Selector != "checkout-v2#Second Phase" || !strings.Contains(phaseTemplate.Template, "planr_new: phase") {
		t.Fatalf("phase template = %+v", phaseTemplate)
	}
	if _, err := os.Stat(filepath.Join(root, "checkout-v2-second-phase.md")); !os.IsNotExist(err) {
		t.Fatalf("new phase --json wrote a draft file; stat error=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "plans", "00-checkout-v2", planLockFileName)); !os.IsNotExist(err) {
		t.Fatalf("new phase --json wrote a lock file; stat error=%v", err)
	}
}

func TestShowSectionsAllAndSchemaReturnMachineReadableDocuments(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dir: plans\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writePlan(filepath.Join(root, "plans", "00-checkout-v2"), applyTestDraft("checkout-v2"), "00-checkout-v2", doc.English); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, root)

	output, err := captureOutput(t, func() error {
		return newShowTestCommand().Run(context.Background(), []string{"show", "checkout-v2", "--section", "goals", "--json"})
	})
	if err != nil {
		t.Fatalf("show section: %v", err)
	}
	var section showSectionJSONOutput
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
	var all showAllJSONOutput
	if err := json.Unmarshal([]byte(output), &all); err != nil {
		t.Fatalf("decode show all: %v; output=%q", err, output)
	}
	if all.Plan != "checkout-v2" || len(all.Phases) != 2 || !strings.Contains(all.Documents["PLAN.md"], "Phase 00") {
		t.Fatalf("show all = %+v", all)
	}

	var schema schemaOutput
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

func editDocumentForTest(t *testing.T, root, selector, output, section string) string {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	args := []string{"edit", selector, "--output", output}
	if section != "" {
		args = append(args, "--section", section)
	}
	if err := newEditTestCommand().Run(context.Background(), args); err != nil {
		t.Fatalf("edit checkout: %v", err)
	}
	path := filepath.Join(root, output)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func newEditTestCommand() *cli.Command {
	return &cli.Command{
		Name: "edit",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "section"},
			&cli.StringFlag{Name: "output"},
			&cli.BoolFlag{Name: "json"},
			&cli.BoolFlag{Name: "no-hooks"},
		},
		Action: editCommand,
	}
}

func newNewTestCommand() *cli.Command {
	return &cli.Command{
		Name: "new",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "description"},
			&cli.StringSliceFlag{Name: "depends-on"},
			&cli.StringFlag{Name: "output"},
			&cli.BoolFlag{Name: "json"},
			&cli.BoolFlag{Name: "no-hooks"},
		},
		Action: newCommand,
	}
}

func newShowTestCommand() *cli.Command {
	return &cli.Command{
		Name: "show",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "section"},
			&cli.BoolFlag{Name: "all"},
			&cli.BoolFlag{Name: "json"},
		},
		Action: showCommand,
	}
}

func newSchemaTestCommand() *cli.Command {
	return &cli.Command{
		Name:   "schema",
		Flags:  []cli.Flag{&cli.BoolFlag{Name: "json"}},
		Action: schemaCommand,
	}
}

func renderPlaceholderDraftForTest(t *testing.T) string {
	t.Helper()
	raw, err := doc.RenderNewDraft(doc.English, "demo", nil, "a demo plan")
	if err != nil {
		t.Fatal(err)
	}
	return raw
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

func runRootWithStdin(t *testing.T, input string, args []string) (string, error) {
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
	return captureOutput(t, func() error {
		return newRootCommand().Run(context.Background(), args)
	})
}
