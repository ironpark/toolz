package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/doctor"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/ironpark/toolz/cli/planr/internal/validation"
	ucli "github.com/urfave/cli/v3"
)

const (
	applyKindPlan  = "plan"
	applyKindPhase = "phase"
	applyKindEdit  = "edit"

	planChecklistPlaceholder = "<!-- planr: phase checklist is derived; do not edit -->"
)

// editEnvelopeKeys are the frontmatter keys that belong to planr's document
// envelopes (edit checkouts and new-phase drafts), not to the stored document.
var editEnvelopeKeys = []string{
	"planr_edit",
	"planr_target",
	"planr_base",
	"planr_phase",
	"planr_slug",
	"planr_section",
	"planr_new",
	"planr_plan",
	"phase_title",
	"phase",
	"slug",
}

type phaseDraftInput struct {
	Plan  string
	Title string
	draft.Phase
}

type applyOperation struct {
	action    string
	selector  string
	dryRun    bool
	changed   bool
	documents map[string]string
	diffs     []applyDiffJSON
}

func newPhaseCommand(cmd *ucli.Command, selector string) error {
	planArg, title, ok := strings.Cut(selector, "#")
	if !ok || planArg == "" || title == "" || strings.Contains(title, "#") {
		return fmt.Errorf("new phase requires <plan-name>#<phase-name>")
	}
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("phase title must not be empty")
	}
	if strings.ContainsAny(title, "\r\n") {
		return fmt.Errorf("phase title must be a single line")
	}
	if len(cmd.StringSlice("depends-on")) > 0 || strings.TrimSpace(cmd.String("description")) != "" {
		return fmt.Errorf("phase draft fields belong in the draft; do not pass plan description or dependency flags")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	settings, repoRoot, err := config.Load(cwd)
	if err != nil {
		return err
	}
	settings = settings.WithSkipHooks(cmd.Bool("no-hooks"))
	planDirectories := settings.PlanDirs(repoRoot)
	planRoot, planDirectory, err := plan.FindDirectory(planDirectories, planArg)
	if err != nil {
		return err
	}
	done, err := plan.AlreadyDone(planRoot)
	if err != nil {
		return err
	}
	if done {
		return fmt.Errorf("plan %q is already done; new phase drafts are only allowed for open plans", planDirectory)
	}

	slug := slugifyPhaseTitle(strings.TrimSpace(title))
	if slug == "" {
		// The draft remains editable, and the author can replace this placeholder
		// with a valid ASCII slug before applying a non-ASCII title.
		slug = "phase"
	}
	output := cmd.String("output")
	if output == "" {
		output = draft.Name(planDirectory) + "-" + slug + ".md"
	}
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if !cmd.Bool("json") {
		if _, err := os.Stat(absOutput); err == nil {
			return fmt.Errorf("draft file already exists: %s", absOutput)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	rendered, err := doc.RenderNewPhaseDraft(settings.Language, draft.Name(planDirectory), strings.TrimSpace(title), slug)
	if err != nil {
		return err
	}
	if err := runDocumentHooks(repoRoot, settings, "before", hooks.EventNew, planDirectory, -1, "draft", cmd.Bool("json")); err != nil {
		return err
	}
	if cmd.Bool("json") {
		if err := writeJSON(makeTemplateJSON(applyKindPhase, draft.Name(planDirectory)+"#"+strings.TrimSpace(title), rendered)); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(absOutput, []byte(rendered), 0644); err != nil {
			return err
		}
		fmt.Printf("Created %s\n", absOutput)
	}
	return runDocumentHooks(repoRoot, settings, "after", hooks.EventNew, planDirectory, -1, "draft", cmd.Bool("json"))
}

func runDocumentHooks(repoRoot string, settings config.Config, when, event, planDirectory string, phaseID int, status string, jsonOutput bool) error {
	if jsonOutput {
		return hooks.RunTo(repoRoot, settings.Hooks, settings.SkipHooks, when, event, planDirectory, phaseID, status, io.Discard)
	}
	return hooks.Run(repoRoot, settings.Hooks, settings.SkipHooks, when, event, planDirectory, phaseID, status)
}

func applyCommand(_ context.Context, cmd *ucli.Command) error {
	if cmd.NArg() > 1 {
		return applyCommandError(cmd, fmt.Errorf("apply accepts one document file or --stdin"))
	}
	if cmd.Bool("stdin") && cmd.NArg() != 0 {
		return applyCommandError(cmd, fmt.Errorf("apply accepts either a document file or --stdin, not both"))
	}
	if !cmd.Bool("stdin") && cmd.NArg() == 0 {
		return applyCommandError(cmd, fmt.Errorf("apply requires <document-file> or --stdin"))
	}

	var (
		raw      []byte
		fallback string
		err      error
	)
	if cmd.Bool("stdin") {
		raw, err = io.ReadAll(os.Stdin)
		raw = unwrapJSONDocument(raw)
	} else {
		fallback = cmd.Args().First()
		raw, err = os.ReadFile(fallback)
	}
	if err != nil {
		return applyCommandError(cmd, err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return applyCommandError(cmd, err)
	}
	settings, repoRoot, err := config.Load(cwd)
	if err != nil {
		return applyCommandError(cmd, err)
	}
	settings = settings.WithSkipHooks(cmd.Bool("no-hooks"))

	kind, document, err := detectApplyDocument(raw, fallback)
	if err != nil {
		return applyCommandError(cmd, err)
	}
	var operation applyOperation
	switch kind {
	case applyKindPlan:
		operation, err = applyPlanDraft(document.(draft.Draft), settings, repoRoot, cmd.Bool("dry-run"), cmd.Bool("json"))
	case applyKindPhase:
		operation, err = applyPhaseDraft(document.(phaseDraftInput), settings, repoRoot, cmd.Bool("dry-run"), cmd.Bool("json"))
	case applyKindEdit:
		operation, err = applyEditDocument(raw, settings, repoRoot, cmd.Bool("dry-run"), cmd.Bool("json"))
	default:
		err = fmt.Errorf("unsupported document kind %q", kind)
	}
	if err != nil {
		return applyCommandError(cmd, err)
	}
	if cmd.Bool("json") {
		return writeJSON(makeApplyJSON(operation))
	}
	if operation.dryRun {
		printApplyDryRun(operation)
	}
	return nil
}

// unwrapJSONDocument lets an agent pass the JSON result of `new --json` or
// `edit --json` directly to `apply --stdin`. Raw Markdown remains the primary
// stdin format; this compatibility envelope only makes the stdout-to-stdin
// path composable without a temporary file.
func unwrapJSONDocument(raw []byte) []byte {
	var envelope struct {
		Template string `json:"template"`
		Document string `json:"document"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return raw
	}
	if envelope.Document != "" {
		return []byte(envelope.Document)
	}
	if envelope.Template != "" {
		return []byte(envelope.Template)
	}
	return raw
}

func printApplyDryRun(operation applyOperation) {
	if !operation.changed {
		fmt.Printf("Would leave %s unchanged\n", operation.selector)
		return
	}
	for _, diff := range operation.diffs {
		fmt.Printf("Would update %s\n", diff.Path)
	}
}

func applyCommandError(cmd *ucli.Command, err error) error {
	if !cmd.Bool("json") {
		return err
	}
	records := validation.Records(err)
	if len(records) == 0 {
		records = []validation.Record{{Rule: "document", Detail: err.Error()}}
	}
	if writeErr := writeJSON(applyFailureJSON{Ok: false, Errors: makeValidationJSON(records)}); writeErr != nil {
		return writeErr
	}
	return err
}

func detectApplyDocument(raw []byte, fallback string) (string, any, error) {
	front, _, err := mdoc.Split(string(raw))
	if err != nil {
		return "", nil, validation.Wrap(err, "frontmatter", "frontmatter")
	}
	if value, ok := front["planr_edit"].(string); ok && strings.TrimSpace(value) != "" {
		return applyKindEdit, raw, nil
	}
	if value, ok := front["planr_new"].(string); ok {
		if value != applyKindPhase {
			return "", nil, fmt.Errorf("unknown planr_new document kind %q", value)
		}
		phase, err := parsePhaseDraft(raw)
		if err != nil {
			return "", nil, err
		}
		return applyKindPhase, phase, nil
	}
	parsed, err := draft.Parse(raw, fallback)
	if err != nil {
		return "", nil, err
	}
	if err := draft.ValidateDependencies(&parsed); err != nil {
		return "", nil, err
	}
	return applyKindPlan, parsed, nil
}

func parsePhaseDraft(raw []byte) (phaseDraftInput, error) {
	front, body, err := mdoc.Split(string(raw))
	if err != nil {
		return phaseDraftInput{}, validation.Wrap(err, "frontmatter", "frontmatter")
	}
	if err := draft.CheckPlaceholders(string(raw)); err != nil {
		return phaseDraftInput{}, err
	}
	planName, ok := front["planr_plan"].(string)
	if !ok || strings.TrimSpace(planName) == "" {
		return phaseDraftInput{}, phaseDraftValidationError("frontmatter", "phase draft requires planr_plan")
	}
	planName = strings.TrimSpace(planName)
	title, ok := front["phase_title"].(string)
	if !ok || strings.TrimSpace(title) == "" {
		return phaseDraftInput{}, phaseDraftValidationError("frontmatter", "phase draft requires phase_title")
	}
	title = strings.TrimSpace(title)
	if strings.ContainsAny(title, "\r\n") {
		return phaseDraftInput{}, phaseDraftValidationError("frontmatter", "phase title must be a single line")
	}
	if _, found := front["phase"]; found {
		return phaseDraftInput{}, phaseDraftValidationError("frontmatter", "phase number is assigned by planr apply; remove phase from the draft")
	}
	slug, ok := front["slug"].(string)
	if !ok || !draft.KebabPattern.MatchString(strings.TrimSpace(slug)) {
		return phaseDraftInput{}, phaseDraftValidationError("frontmatter", fmt.Sprintf("phase slug %q must be lowercase kebab-case", slug))
	}
	slug = strings.TrimSpace(slug)

	var meta draft.Meta
	frontYAML, err := yaml.Marshal(front)
	if err != nil {
		return phaseDraftInput{}, phaseDraftValidationError("frontmatter", fmt.Sprintf("parse phase metadata: %v", err))
	}
	if err := yaml.Unmarshal(frontYAML, &meta); err != nil {
		return phaseDraftInput{}, phaseDraftValidationError("frontmatter", fmt.Sprintf("parse phase metadata: %v", err))
	}
	meta.Phase = -1
	meta.Slug = slug
	if meta.Status != "planned" && meta.Status != "conditional" {
		return phaseDraftInput{}, phaseDraftValidationError("frontmatter", fmt.Sprintf("invalid new phase status %q; use planned or conditional", meta.Status))
	}
	if meta.Status == "conditional" && (meta.EntryCondition == nil || strings.TrimSpace(*meta.EntryCondition) == "") {
		return phaseDraftInput{}, phaseDraftValidationError("frontmatter", "conditional phase requires entry_condition")
	}
	if meta.Status == "planned" && meta.EntryCondition != nil {
		return phaseDraftInput{}, phaseDraftValidationError("frontmatter", "planned phase cannot set entry_condition")
	}
	if mdoc.Title(body) != title {
		return phaseDraftInput{}, phaseDraftValidationError("phase", fmt.Sprintf("phase title %q in the body does not match phase_title %q", mdoc.Title(body), title))
	}
	planned, completion, err := draft.SplitPhaseDocumentSections(title, body)
	if err != nil {
		return phaseDraftInput{}, phaseDraftValidationError("phase", err.Error())
	}
	if planned == "" || completion == "" {
		return phaseDraftInput{}, phaseDraftValidationError("phase", "phase work and completion must not be empty")
	}
	return phaseDraftInput{Plan: planName, Title: title, Phase: draft.Phase{Title: title, Meta: meta, Planned: planned, Completion: completion}}, nil
}

func phaseDraftValidationError(section, detail string) error {
	return validation.NewFailure(validation.Record{Rule: "phase_document", Section: section, Detail: detail}, detail)
}

func applyPlanDraft(d draft.Draft, settings config.Config, repoRoot string, dryRun, jsonOutput bool) (applyOperation, error) {
	planDirectories := settings.PlanDirs(repoRoot)
	if len(planDirectories) == 0 {
		return applyOperation{}, fmt.Errorf("no plans directory is configured")
	}
	if !dryRun {
		lock, err := plan.AcquireDirectoryLock(planDirectories[0])
		if err != nil {
			return applyOperation{}, err
		}
		defer lock.Close()
	}
	planDirectory, err := plan.NextDirectory(planDirectories, d.Name)
	if err != nil {
		return applyOperation{}, err
	}
	documents, err := plan.RenderDocuments(d, planDirectory, settings.Language, plan.CompletionTimestamp())
	if err != nil {
		return applyOperation{}, err
	}
	target := filepath.Join(planDirectories[0], planDirectory)
	op := makeOperation("register_plan", d.Name, dryRun, documentsWithRoot(target, documents), newDocumentDiffs(target, documents))
	if dryRun {
		return op, nil
	}
	if err := runDocumentHooks(repoRoot, settings, "before", hooks.EventAdd, planDirectory, -1, "registered", jsonOutput); err != nil {
		return applyOperation{}, err
	}
	temporary, err := os.MkdirTemp(planDirectories[0], ".planr-")
	if err != nil {
		return applyOperation{}, err
	}
	defer os.RemoveAll(temporary)
	if err := writeRenderedPlan(temporary, documents); err != nil {
		return applyOperation{}, err
	}
	if err := os.Rename(temporary, target); err != nil {
		return applyOperation{}, err
	}
	if !jsonOutput {
		fmt.Printf("Registered %s\n", planDirectory)
	}
	if err := runDocumentHooks(repoRoot, settings, "after", hooks.EventAdd, planDirectory, -1, "registered", jsonOutput); err != nil {
		return applyOperation{}, err
	}
	return op, nil
}

func writeRenderedPlan(root string, documents map[string]string) error {
	if err := os.MkdirAll(filepath.Join(root, "phases"), 0755); err != nil {
		return err
	}
	paths := make([]string, 0, len(documents))
	for path := range documents {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte(documents[path]), 0644); err != nil {
			return err
		}
	}
	return nil
}

func applyPhaseDraft(d phaseDraftInput, settings config.Config, repoRoot string, dryRun, jsonOutput bool) (applyOperation, error) {
	planDirectories := settings.PlanDirs(repoRoot)
	planRoot, planDirectory, err := plan.FindDirectory(planDirectories, d.Plan)
	if err != nil {
		return applyOperation{}, err
	}
	if !dryRun {
		lock, err := plan.AcquireLock(planRoot)
		if err != nil {
			return applyOperation{}, err
		}
		defer lock.Close()
	}

	planPath := filepath.Join(planRoot, "PLAN.md")
	planRaw, err := os.ReadFile(planPath)
	if err != nil {
		return applyOperation{}, err
	}
	planFront, planBody, err := mdoc.Split(string(planRaw))
	if err != nil {
		return applyOperation{}, fmt.Errorf("parse %s/PLAN.md: %w", planDirectory, err)
	}
	if status, _ := planFront["plan_status"].(string); status == "done" {
		detail := fmt.Sprintf("plan %q is already done; new phases can only be applied to open plans", planDirectory)
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "plan_done", Section: "frontmatter", Detail: detail}, detail)
	}
	phases, err := plan.ReadPhases(planRoot)
	if err != nil {
		return applyOperation{}, err
	}
	phaseID := nextPhaseID(phases)
	for _, phase := range phases {
		if phase.Slug == d.Meta.Slug {
			detail := fmt.Sprintf("phase slug %q already exists in plan %q", d.Meta.Slug, planDirectory)
			return applyOperation{}, validation.NewFailure(validation.Record{Rule: "phase_slug_duplicate", Section: "frontmatter", Detail: detail}, detail)
		}
	}
	dependencies, err := resolvePhaseDraftDependencies(d.Meta.DependsOnRefs, phases)
	if err != nil {
		return applyOperation{}, err
	}
	meta := d.Meta
	meta.Phase = phaseID
	meta.DependsOn = dependencies
	if err := validateNewPhaseDependencies(planDirectory, meta, d.Title, phases); err != nil {
		return applyOperation{}, err
	}
	updatedPlanBody, err := appendPhaseChecklist(planBody, phaseID, d.Title, meta.Slug)
	if err != nil {
		return applyOperation{}, fmt.Errorf("update %s/PLAN.md: %w", planDirectory, err)
	}
	updatedPlanFront := mdoc.CopyFront(planFront)
	updatedPlanFront["plan_status"] = "in-progress"
	delete(updatedPlanFront, "completed_at")
	phasePath := filepath.Join(planRoot, plan.PhaseDocumentPath(phaseID, meta.Slug))
	phaseContents, err := mdoc.Render(plan.PhaseFrontmatter(planDirectory, meta), plan.PhaseDocumentBody(settings.Language, d.Title, d.Planned, d.Completion))
	if err != nil {
		return applyOperation{}, err
	}
	updatedPlanContents, err := mdoc.Render(updatedPlanFront, updatedPlanBody)
	if err != nil {
		return applyOperation{}, err
	}
	documents := map[string]string{phasePath: phaseContents, planPath: updatedPlanContents}
	diffs := []applyDiffJSON{{Path: absolutePath(phasePath), After: phaseContents}, {Path: absolutePath(planPath), Before: string(planRaw), After: updatedPlanContents}}
	op := makeOperation("add_phase", draft.Name(planDirectory)+"#"+d.Title, dryRun, documents, diffs)
	if dryRun {
		return op, nil
	}
	if err := runDocumentHooks(repoRoot, settings, "before", hooks.EventPhaseAdd, planDirectory, phaseID, meta.Status, jsonOutput); err != nil {
		return applyOperation{}, err
	}
	if err := os.MkdirAll(filepath.Join(planRoot, "phases"), 0755); err != nil {
		return applyOperation{}, err
	}
	if err := mdoc.WriteAtomically(phasePath, phaseContents); err != nil {
		return applyOperation{}, err
	}
	if err := mdoc.WriteAtomically(planPath, updatedPlanContents); err != nil {
		_ = os.Remove(phasePath)
		return applyOperation{}, err
	}
	if !jsonOutput {
		fmt.Printf("Added %s phase %02d: %s\n", planDirectory, phaseID, phasePath)
	}
	if err := runDocumentHooks(repoRoot, settings, "after", hooks.EventPhaseAdd, planDirectory, phaseID, meta.Status, jsonOutput); err != nil {
		return applyOperation{}, err
	}
	return op, nil
}

func resolvePhaseDraftDependencies(refs []draft.Ref, existing []plan.StoredPhase) ([]int, error) {
	bySlug := map[string]int{}
	known := make([]string, 0, len(existing))
	for _, phase := range existing {
		bySlug[phase.Slug] = phase.ID
		known = append(known, phase.Slug)
	}
	sort.Strings(known)
	dependencies := []int{}
	seen := map[int]bool{}
	for _, ref := range refs {
		id := -1
		if ref.Number != nil {
			id = *ref.Number
		} else {
			var found bool
			id, found = bySlug[ref.Slug]
			if !found {
				detail := fmt.Sprintf("phase dependency %q is neither a phase number nor a slug of an existing phase; available slugs: %s", ref.Slug, strings.Join(known, ", "))
				return nil, validation.NewFailure(validation.Record{Rule: "dependency_reference", Section: "frontmatter", Detail: detail}, detail)
			}
		}
		if id < 0 {
			detail := fmt.Sprintf("phase dependency %d must be a non-negative phase number", id)
			return nil, validation.NewFailure(validation.Record{Rule: "dependency_reference", Section: "frontmatter", Detail: detail}, detail)
		}
		if seen[id] {
			detail := fmt.Sprintf("phase dependency %d is listed more than once", id)
			return nil, validation.NewFailure(validation.Record{Rule: "dependency_duplicate", Section: "frontmatter", Detail: detail}, detail)
		}
		seen[id] = true
		dependencies = append(dependencies, id)
	}
	sort.Ints(dependencies)
	return dependencies, nil
}

// slugifyPhaseTitle collapses each run of non-alphanumeric characters into a
// single dash and trims the dashes at both ends.
func slugifyPhaseTitle(title string) string {
	var builder strings.Builder
	pendingDash := false
	for _, value := range strings.ToLower(title) {
		if (value >= 'a' && value <= 'z') || (value >= '0' && value <= '9') {
			if pendingDash {
				builder.WriteByte('-')
				pendingDash = false
			}
			builder.WriteRune(value)
			continue
		}
		pendingDash = builder.Len() > 0
	}
	return builder.String()
}

func nextPhaseID(phases []plan.StoredPhase) int {
	next := 0
	for _, phase := range phases {
		if phase.ID >= next {
			next = phase.ID + 1
		}
	}
	return next
}

// storedPhaseToDraft converts one stored phase into the draft shape used by
// dependency validation, resolving its internal depends_on references.
func storedPhaseToDraft(planName string, phase plan.StoredPhase) (draft.Phase, error) {
	meta := draft.Meta{Phase: phase.ID, Slug: phase.Slug, Status: phase.Status}
	for _, raw := range phase.Dependencies {
		dependency, err := draft.ParseDependency(raw)
		if err != nil || dependency.Phase == nil || dependency.Plan != planName {
			return draft.Phase{}, fmt.Errorf("phase %d has invalid internal dependency %q", phase.ID, raw)
		}
		meta.DependsOn = append(meta.DependsOn, *dependency.Phase)
	}
	return draft.Phase{Title: phase.Title, Meta: meta}, nil
}

// storedPhasesToDraft converts a plan's stored phases into draft phases so the
// shared dependency-graph validation can run against them.
func storedPhasesToDraft(planDirectory string, phases []plan.StoredPhase) ([]draft.Phase, error) {
	planName := draft.Name(planDirectory)
	all := make([]draft.Phase, 0, len(phases)+1)
	for _, phase := range phases {
		converted, err := storedPhaseToDraft(planName, phase)
		if err != nil {
			return nil, err
		}
		all = append(all, converted)
	}
	return all, nil
}

func validateNewPhaseDependencies(planDirectory string, newPhase draft.Meta, title string, existing []plan.StoredPhase) error {
	all, err := storedPhasesToDraft(planDirectory, existing)
	if err != nil {
		return err
	}
	all = append(all, draft.Phase{Title: title, Meta: newPhase})
	if err := draft.ValidatePhaseDependencies(all); err != nil {
		message := fmt.Sprintf("invalid dependencies for new phase %d: %v", newPhase.Phase, err)
		records := validation.Records(err)
		if len(records) == 0 {
			records = []validation.Record{{Rule: "dependency", Section: "frontmatter", Phase: validation.IntPointer(newPhase.Phase), Detail: err.Error()}}
		}
		return validation.NewFailures(records, message)
	}
	return nil
}

func appendPhaseChecklist(body string, phaseID int, title, slug string) (string, error) {
	marker := fmt.Sprintf("[Phase %02d:", phaseID)
	if strings.Contains(body, marker) {
		return "", fmt.Errorf("checklist already contains phase %02d", phaseID)
	}
	lines := strings.SplitAfter(body, "\n")
	offset := 0
	phasesHeadingEnd := -1
	insertion := len(body)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "# Phases" {
			phasesHeadingEnd = offset + len(line)
		} else if phasesHeadingEnd >= 0 && strings.HasPrefix(trimmed, "# ") {
			insertion = offset
			break
		}
		offset += len(line)
	}
	if phasesHeadingEnd < 0 {
		return "", fmt.Errorf("PLAN.md does not contain a # Phases section")
	}
	entry := plan.ChecklistEntry(phaseID, title, slug, false)
	before := strings.TrimRight(body[:insertion], "\n")
	after := strings.TrimLeft(body[insertion:], "\n")
	if after == "" {
		return before + "\n\n" + entry + "\n", nil
	}
	return before + "\n" + entry + "\n\n" + after, nil
}

func makeOperation(action, selector string, dryRun bool, documents map[string]string, diffs []applyDiffJSON) applyOperation {
	changed := false
	for _, diff := range diffs {
		if diff.Before != diff.After {
			changed = true
			break
		}
	}
	return applyOperation{action: action, selector: selector, dryRun: dryRun, changed: changed, documents: documents, diffs: diffs}
}

func documentsWithRoot(root string, documents map[string]string) map[string]string {
	result := make(map[string]string, len(documents))
	for path, contents := range documents {
		result[absolutePath(filepath.Join(root, filepath.FromSlash(path)))] = contents
	}
	return result
}

func newDocumentDiffs(root string, documents map[string]string) []applyDiffJSON {
	paths := make([]string, 0, len(documents))
	for path := range documents {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]applyDiffJSON, 0, len(paths))
	for _, path := range paths {
		result = append(result, applyDiffJSON{Path: absolutePath(filepath.Join(root, filepath.FromSlash(path))), After: documents[path]})
	}
	return result
}

func absolutePath(path string) string {
	value, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return value
}

func applyEditDocument(raw []byte, settings config.Config, repoRoot string, dryRun, jsonOutput bool) (applyOperation, error) {
	front, _, err := mdoc.Split(string(raw))
	if err != nil {
		return applyOperation{}, validation.Wrap(err, "frontmatter", "frontmatter")
	}
	selector, ok := front["planr_edit"].(string)
	if !ok || strings.TrimSpace(selector) == "" {
		detail := "edit document requires planr_edit in frontmatter"
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "edit_identity", Section: "frontmatter", Detail: detail}, detail)
	}
	targetValue, ok := front["planr_target"].(string)
	if !ok || strings.TrimSpace(targetValue) == "" {
		detail := "edit document requires planr_target in frontmatter"
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "target_required", Section: "frontmatter", Detail: detail}, detail)
	}
	base, ok := front["planr_base"].(string)
	if !ok || strings.TrimSpace(base) == "" {
		detail := "edit document requires mandatory planr_base in frontmatter; run planr edit again"
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "base_required", Detail: detail}, detail)
	}
	if !strings.HasPrefix(base, "sha256:") {
		detail := "planr_base must be a sha256 hash; run planr edit again"
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "base_invalid", Detail: detail}, detail)
	}
	decodedBase, decodeErr := hex.DecodeString(strings.TrimPrefix(base, "sha256:"))
	if decodeErr != nil || len(decodedBase) != sha256.Size {
		detail := "planr_base must be a sha256 hash; run planr edit again"
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "base_invalid", Detail: detail}, detail)
	}
	planArg, targetKind, phaseID, section, err := parseEditDocumentSelector(selector, front)
	if err != nil {
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "edit_selector", Section: "frontmatter", Detail: err.Error()}, err.Error())
	}
	planDirectories := settings.PlanDirs(repoRoot)
	planRoot, planDirectory, err := plan.FindDirectory(planDirectories, planArg)
	if err != nil {
		return applyOperation{}, err
	}
	if !dryRun {
		lock, err := plan.AcquireLock(planRoot)
		if err != nil {
			return applyOperation{}, err
		}
		defer lock.Close()
	}
	var target string
	if targetKind == "phase" {
		target, err = plan.FindPhaseFile(planRoot, phaseID)
	} else {
		target = filepath.Join(planRoot, sectionFile(section))
		_, err = os.Stat(target)
	}
	if err != nil {
		return applyOperation{}, fmt.Errorf("%s: %w", planDirectory, err)
	}
	expectedTarget, err := relativeTargetPath(repoRoot, targetValue)
	if err != nil {
		return applyOperation{}, err
	}
	actualTarget, err := relativeTargetPath(repoRoot, target)
	if err != nil {
		return applyOperation{}, err
	}
	if expectedTarget != actualTarget {
		return applyOperation{}, fmt.Errorf("planr_target %q does not identify %s", targetValue, actualTarget)
	}
	currentRaw, err := os.ReadFile(target)
	if err != nil {
		return applyOperation{}, err
	}
	currentHash := mdoc.Hash(currentRaw)
	if currentHash != base {
		detail := fmt.Sprintf("cannot apply edit for %s: planr_base %s does not match the current on-disk document hash %s; run planr edit again", selector, base, currentHash)
		record := validation.Record{Rule: "base_mismatch", Detail: detail}
		if targetKind == "phase" {
			record.Phase = validation.IntPointer(phaseID)
		}
		return applyOperation{}, validation.NewFailure(record, detail)
	}
	if targetKind == "phase" {
		return applyPhaseEdit(raw, front, currentRaw, target, planRoot, planDirectory, phaseID, dryRun, jsonOutput)
	}
	return applySectionEdit(raw, currentRaw, target, planDirectory, section, dryRun, jsonOutput)
}

func parseEditSelector(selector string) (string, string, int, string, error) {
	if strings.Count(selector, "#") != 1 {
		return "", "", 0, "", fmt.Errorf("planr_edit must use <plan-name>#<phase-number>")
	}
	planName, suffix, _ := strings.Cut(selector, "#")
	planName = strings.TrimSpace(planName)
	suffix = strings.TrimSpace(suffix)
	if planName == "" || suffix == "" {
		return "", "", 0, "", fmt.Errorf("planr_edit has an empty plan or document selector")
	}
	phaseID, err := strconv.Atoi(suffix)
	if err != nil || phaseID < 0 {
		return "", "", 0, "", fmt.Errorf("edit phase number %q must be a non-negative integer", suffix)
	}
	return planName, "phase", phaseID, "", nil
}

func parseEditDocumentSelector(selector string, front map[string]any) (string, string, int, string, error) {
	if value, found := front["planr_section"]; found {
		section, ok := value.(string)
		if !ok || strings.TrimSpace(section) == "" {
			return "", "", 0, "", fmt.Errorf("planr_section must be goals, context, or plan")
		}
		section = strings.TrimSpace(section)
		if !validSection(section) {
			return "", "", 0, "", fmt.Errorf("invalid edit section %q; use goals, context, or plan", section)
		}
		if strings.Contains(selector, "#") {
			return "", "", 0, "", fmt.Errorf("section edit selector must contain only the plan name")
		}
		if strings.TrimSpace(selector) == "" {
			return "", "", 0, "", fmt.Errorf("planr_edit must contain a plan name")
		}
		return strings.TrimSpace(selector), "section", -1, section, nil
	}
	return parseEditSelector(selector)
}

// sectionFiles maps the editable/showable plan section names to the documents
// they live in; it also serves as the section-name validation set.
var sectionFiles = map[string]string{
	"goals":   "GOALS.md",
	"context": "CONTEXT.md",
	"plan":    "PLAN.md",
}

func validSection(section string) bool {
	_, ok := sectionFiles[section]
	return ok
}

func sectionFile(section string) string {
	if file, ok := sectionFiles[section]; ok {
		return file
	}
	return "PLAN.md"
}

func relativeTargetPath(repoRoot, value string) (string, error) {
	path := filepath.FromSlash(strings.TrimSpace(value))
	if filepath.IsAbs(path) {
		path = filepath.Clean(path)
	} else {
		path = filepath.Join(repoRoot, path)
	}
	relative, err := filepath.Rel(repoRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("planr_target %q must identify a document inside the repository", value)
	}
	return filepath.ToSlash(filepath.Clean(relative)), nil
}

func applyPhaseEdit(raw []byte, incomingFront map[string]any, currentRaw []byte, target, planRoot, planDirectory string, phaseID int, dryRun, jsonOutput bool) (applyOperation, error) {
	currentFront, currentBody, err := mdoc.Split(string(currentRaw))
	if err != nil {
		return applyOperation{}, fmt.Errorf("parse %s: %w", filepath.Base(target), err)
	}
	currentStatus, _ := currentFront["status"].(string)
	incomingStatus, _ := incomingFront["status"].(string)
	if incomingStatus != currentStatus {
		detail := fmt.Sprintf("cannot apply phase edit for %s phase %02d: status changed from %q to %q; use `planr phase %s` to change phase status", planDirectory, phaseID, currentStatus, incomingStatus, phaseStatusCommand(incomingStatus))
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "status_transition", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
	}
	if !plan.StatusValues[currentStatus] {
		detail := fmt.Sprintf("%s phase %02d has invalid status %q", planDirectory, phaseID, currentStatus)
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "status", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
	}
	if err := plan.ValidateStatusChange(incomingFront, currentStatus); err != nil {
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "status_metadata", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: err.Error()}, err.Error())
	}
	if value, found := incomingFront["planr_phase"]; found && fmt.Sprint(value) != strconv.Itoa(phaseID) {
		detail := fmt.Sprintf("edit document identifies phase %v, but target is phase %02d", value, phaseID)
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "phase_identity", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
	}
	phases, err := plan.ReadPhases(planRoot)
	if err != nil {
		return applyOperation{}, err
	}
	meta, normalizedFront, err := editablePhaseMeta(incomingFront, planDirectory, phaseID, phases)
	if err != nil {
		if len(validation.Records(err)) > 0 {
			return applyOperation{}, err
		}
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "phase_metadata", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: err.Error()}, err.Error())
	}
	if value, found := incomingFront["planr_slug"]; found && fmt.Sprint(value) != meta.Slug {
		detail := fmt.Sprintf("edit document identifies slug %q, but target is %q", value, meta.Slug)
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "phase_identity", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
	}
	if value, found := incomingFront["slug"]; found {
		if fmt.Sprint(value) != meta.Slug {
			detail := fmt.Sprintf("edit document cannot change phase slug from %q to %q", meta.Slug, value)
			return applyOperation{}, validation.NewFailure(validation.Record{Rule: "phase_identity", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
		}
	}
	if meta.Slug != phaseSlugForID(phases, phaseID) {
		detail := "phase edit cannot change the phase slug"
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "phase_identity", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
	}
	title := mdoc.Title(mdoc.Body(raw))
	if title == "unnamed phase" {
		detail := "phase document must contain a Markdown title"
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "phase_document", Section: "phase", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
	}
	planned, completion, err := draft.SplitPhaseDocumentSections(title, mdoc.Body(raw))
	if err != nil {
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "phase_document", Section: "phase", Phase: validation.IntPointer(phaseID), Detail: err.Error()}, err.Error())
	}
	if planned == "" || completion == "" {
		detail := fmt.Sprintf("phase %q work and completion must not be empty", title)
		return applyOperation{}, validation.NewFailure(validation.Record{Rule: "phase_document", Section: "phase", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
	}
	for _, key := range editEnvelopeKeys {
		delete(normalizedFront, key)
	}
	normalizedFront["status"] = currentStatus
	if completedAt, found := currentFront["completed_at"]; found {
		normalizedFront["completed_at"] = completedAt
	} else {
		delete(normalizedFront, "completed_at")
	}
	newPhase, err := mdoc.Render(normalizedFront, mdoc.Body(raw))
	if err != nil {
		return applyOperation{}, err
	}
	documents := map[string]string{target: newPhase}
	diffs := []applyDiffJSON{{Path: absolutePath(target), Before: string(currentRaw), After: newPhase}}
	if title != mdoc.Title(currentBody) {
		planPath := filepath.Join(planRoot, "PLAN.md")
		planRaw, readErr := os.ReadFile(planPath)
		if readErr != nil {
			return applyOperation{}, readErr
		}
		_, planBody, frontErr := mdoc.Split(string(planRaw))
		if frontErr != nil {
			return applyOperation{}, frontErr
		}
		updatedBody, updateErr := replacePhaseChecklistEntry(planBody, phaseID, title, meta.Slug, currentStatus == "done")
		if updateErr != nil {
			return applyOperation{}, updateErr
		}
		updatedPlan, renderErr := mdoc.WithBody(string(planRaw), updatedBody)
		if renderErr != nil {
			return applyOperation{}, renderErr
		}
		documents[planPath] = updatedPlan
		diffs = append(diffs, applyDiffJSON{Path: absolutePath(planPath), Before: string(planRaw), After: updatedPlan})
	}
	op := makeOperation("edit_phase", draft.Name(planDirectory)+"#"+strconv.Itoa(phaseID), dryRun, documents, diffs)
	if dryRun || !op.changed {
		return op, nil
	}
	if err := mdoc.WriteAtomically(target, newPhase); err != nil {
		return applyOperation{}, err
	}
	if planPath, ok := changedPlanPath(documents, target); ok {
		if err := mdoc.WriteAtomically(planPath, documents[planPath]); err != nil {
			return applyOperation{}, err
		}
	}
	if !jsonOutput {
		fmt.Printf("Updated %s\n", target)
	}
	return op, nil
}

func changedPlanPath(documents map[string]string, phasePath string) (string, bool) {
	planPath := filepath.Join(filepath.Dir(filepath.Dir(phasePath)), "PLAN.md")
	_, found := documents[planPath]
	return planPath, found
}

func phaseStatusCommand(status string) string {
	switch status {
	case "in-progress":
		return "start"
	case "done":
		return "done"
	case "planned":
		return "reset"
	case "conditional":
		return "set --status conditional"
	default:
		return "set --status <status>"
	}
}

func phaseSlugForID(phases []plan.StoredPhase, id int) string {
	for _, phase := range phases {
		if phase.ID == id {
			return phase.Slug
		}
	}
	return ""
}

func editablePhaseMeta(front map[string]any, planDirectory string, phaseID int, phases []plan.StoredPhase) (draft.Meta, map[string]any, error) {
	slug := phaseSlugForID(phases, phaseID)
	meta := draft.Meta{Phase: phaseID, Slug: slug}
	status, _ := front["status"].(string)
	meta.Status = status
	if perf, ok := front["perf_phase"].(bool); ok {
		meta.PerfPhase = perf
	}
	if condition, ok := front["entry_condition"].(string); ok && strings.TrimSpace(condition) != "" {
		value := strings.TrimSpace(condition)
		meta.EntryCondition = &value
	}
	dependencies, normalized, err := editablePhaseDependencies(front["depends_on"], planDirectory, phases)
	if err != nil {
		return draft.Meta{}, nil, err
	}
	meta.DependsOn = dependencies
	normalizedFront := mdoc.CopyFront(front)
	normalizedFront["depends_on"] = normalized
	if err := validateNewPhaseEditDependencies(planDirectory, meta, phases); err != nil {
		return draft.Meta{}, nil, err
	}
	return meta, normalizedFront, nil
}

func editablePhaseDependencies(value any, planDirectory string, phases []plan.StoredPhase) ([]int, any, error) {
	var values []any
	switch typed := value.(type) {
	case []any:
		values = typed
	case []string:
		for _, item := range typed {
			values = append(values, item)
		}
	case nil:
		values = nil
	default:
		return nil, nil, fmt.Errorf("phase depends_on must be a list")
	}
	refs := make([]draft.Ref, 0, len(values))
	for _, item := range values {
		switch typed := item.(type) {
		case int:
			id := typed
			refs = append(refs, draft.Ref{Number: &id})
		case int64:
			id := int(typed)
			refs = append(refs, draft.Ref{Number: &id})
		case float64:
			if typed != float64(int(typed)) {
				return nil, nil, fmt.Errorf("phase dependency %v must be a whole phase number", typed)
			}
			id := int(typed)
			refs = append(refs, draft.Ref{Number: &id})
		case string:
			raw := strings.TrimSpace(typed)
			if dependency, err := draft.ParseDependency(raw); err == nil && dependency.Phase != nil {
				if dependency.Plan != draft.Name(planDirectory) {
					return nil, nil, fmt.Errorf("phase dependency %q must reference a phase in %s", raw, draft.Name(planDirectory))
				}
				refs = append(refs, draft.Ref{Number: dependency.Phase})
			} else if parsed, parseErr := strconv.Atoi(raw); parseErr == nil {
				id := parsed
				refs = append(refs, draft.Ref{Number: &id})
			} else {
				refs = append(refs, draft.Ref{Slug: raw})
			}
		default:
			return nil, nil, fmt.Errorf("phase depends_on entries must be phase numbers or strings")
		}
	}
	ids, err := resolvePhaseDraftDependencies(refs, phases)
	if err != nil {
		return nil, nil, err
	}
	normalized := make([]string, len(ids))
	for index, id := range ids {
		normalized[index] = fmt.Sprintf("%s#%d", planDirectory, id)
	}
	return ids, normalized, nil
}

func replacePhaseChecklistEntry(body string, phaseID int, title, slug string, done bool) (string, error) {
	return plan.TransformChecklistEntry(body, phaseID, func(line string) (string, bool) {
		replacement := plan.ChecklistEntry(phaseID, title, slug, done) + "\n"
		if !strings.HasSuffix(line, "\n") {
			replacement = strings.TrimSuffix(replacement, "\n")
		}
		return replacement, true
	})
}

func validateNewPhaseEditDependencies(planDirectory string, edited draft.Meta, phases []plan.StoredPhase) error {
	planName := draft.Name(planDirectory)
	all := make([]draft.Phase, 0, len(phases))
	for _, phase := range phases {
		if phase.ID == edited.Phase {
			all = append(all, draft.Phase{Title: phase.Title, Meta: edited})
			continue
		}
		converted, err := storedPhaseToDraft(planName, phase)
		if err != nil {
			return err
		}
		all = append(all, converted)
	}
	return draft.ValidatePhaseDependencies(all)
}

func applySectionEdit(raw []byte, currentRaw []byte, target, planDirectory, section string, dryRun, jsonOutput bool) (applyOperation, error) {
	_, incomingBody, err := mdoc.Split(string(raw))
	if err != nil {
		return applyOperation{}, err
	}
	_, currentBody, err := mdoc.Split(string(currentRaw))
	if err != nil {
		return applyOperation{}, err
	}
	var updatedBody string
	switch section {
	case "plan":
		incomingStart, incomingEnd, incomingFound := doctor.ChecklistBounds(incomingBody)
		if !incomingFound || strings.TrimSpace(incomingBody[incomingStart:incomingEnd]) != planChecklistPlaceholder {
			detail := "plan section checkout must keep the derived checklist region unchanged"
			return applyOperation{}, validation.NewFailure(validation.Record{Rule: "derived_region", Section: "PLAN", Detail: detail}, detail)
		}
		start, end, found := doctor.ChecklistBounds(currentBody)
		if !found {
			detail := "PLAN.md does not contain a # Phases section"
			return applyOperation{}, validation.NewFailure(validation.Record{Rule: "derived_region", Section: "PLAN", Detail: detail}, detail)
		}
		updatedBody = incomingBody[:incomingStart] + currentBody[start:end] + incomingBody[incomingEnd:]
	default:
		updatedBody = incomingBody
	}
	updated, err := mdoc.WithBody(string(currentRaw), updatedBody)
	if err != nil {
		return applyOperation{}, err
	}
	op := makeOperation("edit_"+section, draft.Name(planDirectory), dryRun,
		map[string]string{target: updated}, []applyDiffJSON{{Path: absolutePath(target), Before: string(currentRaw), After: updated}})
	if dryRun || !op.changed {
		return op, nil
	}
	if !jsonOutput {
		fmt.Printf("Updated %s\n", target)
	}
	return op, mdoc.WriteAtomically(target, updated)
}
