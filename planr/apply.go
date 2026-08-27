package main

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
	"github.com/urfave/cli/v3"
)

const (
	applyKindPlan  = "plan"
	applyKindPhase = "phase"
	applyKindEdit  = "edit"

	planChecklistPlaceholder = "<!-- planr: phase checklist is derived; do not edit -->"
)

type phaseDraftInput struct {
	Plan  string
	Title string
	draftPhase
}

type applyOperation struct {
	action    string
	selector  string
	dryRun    bool
	changed   bool
	documents map[string]string
	diffs     []applyDiffJSON
}

func newPhaseCommand(cmd *cli.Command, selector string) error {
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
	settings, repoRoot, err := loadConfig(cwd)
	if err != nil {
		return err
	}
	settings = commandConfig(settings, cmd)
	planDirectories := settings.planDirs(repoRoot)
	planRoot, planDirectory, err := findPlanDirectory(planDirectories, planArg)
	if err != nil {
		return err
	}
	done, err := planAlreadyDone(planRoot)
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
		output = planName(planDirectory) + "-" + slug + ".md"
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
	draft, err := renderNewPhaseDraft(settings.Language, planName(planDirectory), strings.TrimSpace(title), slug)
	if err != nil {
		return err
	}
	if err := runDocumentHooks(repoRoot, settings, "before", hookEventNew, planDirectory, -1, "draft", cmd.Bool("json")); err != nil {
		return err
	}
	if cmd.Bool("json") {
		if err := writeJSON(makeTemplateJSON(applyKindPhase, planName(planDirectory)+"#"+strings.TrimSpace(title), draft)); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(absOutput, []byte(draft), 0644); err != nil {
			return err
		}
		fmt.Printf("Created %s\n", absOutput)
	}
	return runDocumentHooks(repoRoot, settings, "after", hookEventNew, planDirectory, -1, "draft", cmd.Bool("json"))
}

func runDocumentHooks(repoRoot string, settings config, when, event, planDirectory string, phaseID int, status string, jsonOutput bool) error {
	if jsonOutput {
		return runConfiguredHooksTo(repoRoot, settings, when, event, planDirectory, phaseID, status, io.Discard)
	}
	return runConfiguredHooks(repoRoot, settings, when, event, planDirectory, phaseID, status)
}

func applyCommand(_ context.Context, cmd *cli.Command) error {
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
	settings, repoRoot, err := loadConfig(cwd)
	if err != nil {
		return applyCommandError(cmd, err)
	}
	settings = commandConfig(settings, cmd)

	kind, document, err := detectApplyDocument(raw, fallback)
	if err != nil {
		return applyCommandError(cmd, err)
	}
	var operation applyOperation
	switch kind {
	case applyKindPlan:
		operation, err = applyPlanDraft(document.(draft), settings, repoRoot, cmd.Bool("dry-run"), cmd.Bool("json"))
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

func applyCommandError(cmd *cli.Command, err error) error {
	if !cmd.Bool("json") {
		return err
	}
	records := validationRecords(err)
	if len(records) == 0 {
		records = []validationRecord{{Rule: "document", Detail: err.Error()}}
	}
	if writeErr := writeJSON(applyFailureJSON{Ok: false, Errors: makeValidationJSON(records)}); writeErr != nil {
		return writeErr
	}
	return err
}

func detectApplyDocument(raw []byte, fallback string) (string, any, error) {
	front, _, err := frontmatter(string(raw))
	if err != nil {
		return "", nil, wrapValidationError(err, "frontmatter", "frontmatter")
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
	draft, err := parseDraft(raw, fallback)
	if err != nil {
		return "", nil, err
	}
	if err := validateDraftDependencies(&draft); err != nil {
		return "", nil, err
	}
	return applyKindPlan, draft, nil
}

func parsePhaseDraft(raw []byte) (phaseDraftInput, error) {
	front, body, err := frontmatter(string(raw))
	if err != nil {
		return phaseDraftInput{}, wrapValidationError(err, "frontmatter", "frontmatter")
	}
	if err := checkDraftPlaceholders(string(raw)); err != nil {
		return phaseDraftInput{}, err
	}
	plan, ok := front["planr_plan"].(string)
	if !ok || strings.TrimSpace(plan) == "" {
		return phaseDraftInput{}, phaseDraftValidationError("frontmatter", "phase draft requires planr_plan")
	}
	plan = strings.TrimSpace(plan)
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
	if !ok || !kebab.MatchString(strings.TrimSpace(slug)) {
		return phaseDraftInput{}, phaseDraftValidationError("frontmatter", fmt.Sprintf("phase slug %q must be lowercase kebab-case", slug))
	}
	slug = strings.TrimSpace(slug)

	var meta phaseMeta
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
	if markdownTitle(body) != title {
		return phaseDraftInput{}, phaseDraftValidationError("phase", fmt.Sprintf("phase title %q in the body does not match phase_title %q", markdownTitle(body), title))
	}
	planned, completion, err := splitPhaseDocumentSections(title, body)
	if err != nil {
		return phaseDraftInput{}, phaseDraftValidationError("phase", err.Error())
	}
	if planned == "" || completion == "" {
		return phaseDraftInput{}, phaseDraftValidationError("phase", "phase work and completion must not be empty")
	}
	return phaseDraftInput{Plan: plan, Title: title, draftPhase: draftPhase{Title: title, Meta: meta, Planned: planned, Completion: completion}}, nil
}

func phaseDraftValidationError(section, detail string) error {
	return newValidationFailure(validationRecord{Rule: "phase_document", Section: section, Detail: detail}, detail)
}

func applyPlanDraft(d draft, settings config, repoRoot string, dryRun, jsonOutput bool) (applyOperation, error) {
	planDirectories := settings.planDirs(repoRoot)
	if len(planDirectories) == 0 {
		return applyOperation{}, fmt.Errorf("no plans directory is configured")
	}
	if !dryRun {
		lock, err := acquirePlansDirectoryLock(planDirectories[0])
		if err != nil {
			return applyOperation{}, err
		}
		defer lock.close()
	}
	planDirectory, err := nextPlanDirectory(planDirectories, d.Name)
	if err != nil {
		return applyOperation{}, err
	}
	documents, err := renderPlanDocuments(d, planDirectory, settings.Language, completionTimestamp())
	if err != nil {
		return applyOperation{}, err
	}
	target := filepath.Join(planDirectories[0], planDirectory)
	op := makeOperation("register_plan", d.Name, dryRun, documentsWithRoot(target, documents), newDocumentDiffs(target, documents))
	if dryRun {
		return op, nil
	}
	if err := runDocumentHooks(repoRoot, settings, "before", hookEventAdd, planDirectory, -1, "registered", jsonOutput); err != nil {
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
	if err := runDocumentHooks(repoRoot, settings, "after", hookEventAdd, planDirectory, -1, "registered", jsonOutput); err != nil {
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

func applyPhaseDraft(d phaseDraftInput, settings config, repoRoot string, dryRun, jsonOutput bool) (applyOperation, error) {
	planDirectories := settings.planDirs(repoRoot)
	planRoot, planDirectory, err := findPlanDirectory(planDirectories, d.Plan)
	if err != nil {
		return applyOperation{}, err
	}
	if !dryRun {
		lock, err := acquirePlanLock(planRoot)
		if err != nil {
			return applyOperation{}, err
		}
		defer lock.close()
	}

	planPath := filepath.Join(planRoot, "PLAN.md")
	planRaw, err := os.ReadFile(planPath)
	if err != nil {
		return applyOperation{}, err
	}
	planFront, planBody, err := frontmatter(string(planRaw))
	if err != nil {
		return applyOperation{}, fmt.Errorf("parse %s/PLAN.md: %w", planDirectory, err)
	}
	if status, _ := planFront["plan_status"].(string); status == "done" {
		detail := fmt.Sprintf("plan %q is already done; new phases can only be applied to open plans", planDirectory)
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "plan_done", Section: "frontmatter", Detail: detail}, detail)
	}
	phases, err := readPlanPhases(planRoot)
	if err != nil {
		return applyOperation{}, err
	}
	phaseID := nextPhaseID(phases)
	for _, phase := range phases {
		if phase.slug == d.Meta.Slug {
			detail := fmt.Sprintf("phase slug %q already exists in plan %q", d.Meta.Slug, planDirectory)
			return applyOperation{}, newValidationFailure(validationRecord{Rule: "phase_slug_duplicate", Section: "frontmatter", Detail: detail}, detail)
		}
	}
	dependencies, err := resolvePhaseDraftDependencies(d.Meta.DependsOnRefs, phases, planDirectory)
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
	updatedPlanFront := copyFrontmatter(planFront)
	updatedPlanFront["plan_status"] = "in-progress"
	delete(updatedPlanFront, "completed_at")
	phasePath := filepath.Join(planRoot, phaseDocumentPath(phaseID, meta.Slug))
	phaseContents, err := renderFrontmatterDocument(phaseFrontmatter(planDirectory, meta), phaseDocumentBody(settings.Language, d.Title, d.Planned, d.Completion))
	if err != nil {
		return applyOperation{}, err
	}
	updatedPlanContents, err := renderFrontmatterDocument(updatedPlanFront, updatedPlanBody)
	if err != nil {
		return applyOperation{}, err
	}
	documents := map[string]string{phasePath: phaseContents, planPath: updatedPlanContents}
	diffs := []applyDiffJSON{{Path: absolutePath(phasePath), After: phaseContents}, {Path: absolutePath(planPath), Before: string(planRaw), After: updatedPlanContents}}
	op := makeOperation("add_phase", planName(planDirectory)+"#"+d.Title, dryRun, documents, diffs)
	if dryRun {
		return op, nil
	}
	if err := runDocumentHooks(repoRoot, settings, "before", hookEventPhaseAdd, planDirectory, phaseID, meta.Status, jsonOutput); err != nil {
		return applyOperation{}, err
	}
	if err := os.MkdirAll(filepath.Join(planRoot, "phases"), 0755); err != nil {
		return applyOperation{}, err
	}
	if err := writeFileAtomically(phasePath, phaseContents); err != nil {
		return applyOperation{}, err
	}
	if err := writeFileAtomically(planPath, updatedPlanContents); err != nil {
		_ = os.Remove(phasePath)
		return applyOperation{}, err
	}
	if !jsonOutput {
		fmt.Printf("Added %s phase %02d: %s\n", planDirectory, phaseID, phasePath)
	}
	if err := runDocumentHooks(repoRoot, settings, "after", hookEventPhaseAdd, planDirectory, phaseID, meta.Status, jsonOutput); err != nil {
		return applyOperation{}, err
	}
	return op, nil
}

func resolvePhaseDraftDependencies(refs []phaseRef, existing []storedPhase, planDirectory string) ([]int, error) {
	bySlug := map[string]int{}
	known := make([]string, 0, len(existing))
	for _, phase := range existing {
		bySlug[phase.slug] = phase.id
		known = append(known, phase.slug)
	}
	sort.Strings(known)
	dependencies := []int{}
	seen := map[int]bool{}
	for _, ref := range refs {
		id := -1
		if ref.number != nil {
			id = *ref.number
		} else {
			var found bool
			id, found = bySlug[ref.slug]
			if !found {
				detail := fmt.Sprintf("phase dependency %q is neither a phase number nor a slug of an existing phase; available slugs: %s", ref.slug, strings.Join(known, ", "))
				return nil, newValidationFailure(validationRecord{Rule: "dependency_reference", Section: "frontmatter", Detail: detail}, detail)
			}
		}
		if id < 0 {
			detail := fmt.Sprintf("phase dependency %d must be a non-negative phase number", id)
			return nil, newValidationFailure(validationRecord{Rule: "dependency_reference", Section: "frontmatter", Detail: detail}, detail)
		}
		if seen[id] {
			detail := fmt.Sprintf("phase dependency %d is listed more than once", id)
			return nil, newValidationFailure(validationRecord{Rule: "dependency_duplicate", Section: "frontmatter", Detail: detail}, detail)
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

func nextPhaseID(phases []storedPhase) int {
	next := 0
	for _, phase := range phases {
		if phase.id >= next {
			next = phase.id + 1
		}
	}
	return next
}

func validateNewPhaseDependencies(planDirectory string, newPhase phaseMeta, title string, existing []storedPhase) error {
	plan := planName(planDirectory)
	all := make([]draftPhase, 0, len(existing)+1)
	for _, phase := range existing {
		meta := phaseMeta{Phase: phase.id, Slug: phase.slug, Status: phase.status}
		for _, raw := range phase.dependencies {
			dependency, err := parseDependency(raw)
			if err != nil || dependency.phase == nil || dependency.plan != plan {
				return fmt.Errorf("phase %d has invalid internal dependency %q", phase.id, raw)
			}
			meta.DependsOn = append(meta.DependsOn, *dependency.phase)
		}
		all = append(all, draftPhase{Title: phase.title, Meta: meta})
	}
	all = append(all, draftPhase{Title: title, Meta: newPhase})
	if err := validatePhaseDependencies(all); err != nil {
		message := fmt.Sprintf("invalid dependencies for new phase %d: %v", newPhase.Phase, err)
		records := validationRecords(err)
		if len(records) == 0 {
			records = []validationRecord{{Rule: "dependency", Section: "frontmatter", Phase: validationIntPointer(newPhase.Phase), Detail: err.Error()}}
		}
		return newValidationFailures(records, message)
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
	entry := phaseChecklistEntry(phaseID, title, slug, false)
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

func copyFrontmatter(front map[string]any) map[string]any {
	result := make(map[string]any, len(front))
	for key, value := range front {
		result[key] = value
	}
	return result
}

func documentHash(raw []byte) string {
	digest := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func applyEditDocument(raw []byte, settings config, repoRoot string, dryRun, jsonOutput bool) (applyOperation, error) {
	front, _, err := frontmatter(string(raw))
	if err != nil {
		return applyOperation{}, wrapValidationError(err, "frontmatter", "frontmatter")
	}
	selector, ok := front["planr_edit"].(string)
	if !ok || strings.TrimSpace(selector) == "" {
		detail := "edit document requires planr_edit in frontmatter"
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "edit_identity", Section: "frontmatter", Detail: detail}, detail)
	}
	targetValue, ok := front["planr_target"].(string)
	if !ok || strings.TrimSpace(targetValue) == "" {
		detail := "edit document requires planr_target in frontmatter"
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "target_required", Section: "frontmatter", Detail: detail}, detail)
	}
	base, ok := front["planr_base"].(string)
	if !ok || strings.TrimSpace(base) == "" {
		detail := "edit document requires mandatory planr_base in frontmatter; run planr edit again"
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "base_required", Detail: detail}, detail)
	}
	if !strings.HasPrefix(base, "sha256:") {
		detail := "planr_base must be a sha256 hash; run planr edit again"
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "base_invalid", Detail: detail}, detail)
	}
	decodedBase, decodeErr := hex.DecodeString(strings.TrimPrefix(base, "sha256:"))
	if decodeErr != nil || len(decodedBase) != sha256.Size {
		detail := "planr_base must be a sha256 hash; run planr edit again"
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "base_invalid", Detail: detail}, detail)
	}
	planArg, targetKind, phaseID, section, err := parseEditDocumentSelector(selector, front)
	if err != nil {
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "edit_selector", Section: "frontmatter", Detail: err.Error()}, err.Error())
	}
	planDirectories := settings.planDirs(repoRoot)
	planRoot, planDirectory, err := findPlanDirectory(planDirectories, planArg)
	if err != nil {
		return applyOperation{}, err
	}
	if !dryRun {
		lock, err := acquirePlanLock(planRoot)
		if err != nil {
			return applyOperation{}, err
		}
		defer lock.close()
	}
	var target string
	if targetKind == "phase" {
		target, err = findPhaseFile(planRoot, phaseID)
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
	currentHash := documentHash(currentRaw)
	if currentHash != base {
		detail := fmt.Sprintf("cannot apply edit for %s: planr_base %s does not match the current on-disk document hash %s; run planr edit again", selector, base, currentHash)
		record := validationRecord{Rule: "base_mismatch", Detail: detail}
		if targetKind == "phase" {
			record.Phase = validationIntPointer(phaseID)
		}
		return applyOperation{}, newValidationFailure(record, detail)
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
	plan, suffix, _ := strings.Cut(selector, "#")
	plan = strings.TrimSpace(plan)
	suffix = strings.TrimSpace(suffix)
	if plan == "" || suffix == "" {
		return "", "", 0, "", fmt.Errorf("planr_edit has an empty plan or document selector")
	}
	phaseID, err := strconv.Atoi(suffix)
	if err != nil || phaseID < 0 {
		return "", "", 0, "", fmt.Errorf("edit phase number %q must be a non-negative integer", suffix)
	}
	return plan, "phase", phaseID, "", nil
}

func parseEditDocumentSelector(selector string, front map[string]any) (string, string, int, string, error) {
	if value, found := front["planr_section"]; found {
		section, ok := value.(string)
		if !ok || strings.TrimSpace(section) == "" {
			return "", "", 0, "", fmt.Errorf("planr_section must be goals, context, or plan")
		}
		section = strings.TrimSpace(section)
		if section != "goals" && section != "context" && section != "plan" {
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

func sectionFile(section string) string {
	switch section {
	case "goals":
		return "GOALS.md"
	case "context":
		return "CONTEXT.md"
	default:
		return "PLAN.md"
	}
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
	currentFront, currentBody, err := frontmatter(string(currentRaw))
	if err != nil {
		return applyOperation{}, fmt.Errorf("parse %s: %w", filepath.Base(target), err)
	}
	currentStatus, _ := currentFront["status"].(string)
	incomingStatus, _ := incomingFront["status"].(string)
	if incomingStatus != currentStatus {
		detail := fmt.Sprintf("cannot apply phase edit for %s phase %02d: status changed from %q to %q; use `planr phase %s` to change phase status", planDirectory, phaseID, currentStatus, incomingStatus, phaseStatusCommand(incomingStatus))
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "status_transition", Section: "frontmatter", Phase: validationIntPointer(phaseID), Detail: detail}, detail)
	}
	if !phaseStatusValues[currentStatus] {
		detail := fmt.Sprintf("%s phase %02d has invalid status %q", planDirectory, phaseID, currentStatus)
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "status", Section: "frontmatter", Phase: validationIntPointer(phaseID), Detail: detail}, detail)
	}
	if err := validatePhaseStatusChange(incomingFront, currentStatus); err != nil {
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "status_metadata", Section: "frontmatter", Phase: validationIntPointer(phaseID), Detail: err.Error()}, err.Error())
	}
	if value, found := incomingFront["planr_phase"]; found && fmt.Sprint(value) != strconv.Itoa(phaseID) {
		detail := fmt.Sprintf("edit document identifies phase %v, but target is phase %02d", value, phaseID)
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "phase_identity", Section: "frontmatter", Phase: validationIntPointer(phaseID), Detail: detail}, detail)
	}
	phases, err := readPlanPhases(planRoot)
	if err != nil {
		return applyOperation{}, err
	}
	meta, normalizedFront, err := editablePhaseMeta(incomingFront, planDirectory, phaseID, phases)
	if err != nil {
		if len(validationRecords(err)) > 0 {
			return applyOperation{}, err
		}
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "phase_metadata", Section: "frontmatter", Phase: validationIntPointer(phaseID), Detail: err.Error()}, err.Error())
	}
	if value, found := incomingFront["planr_slug"]; found && fmt.Sprint(value) != meta.Slug {
		detail := fmt.Sprintf("edit document identifies slug %q, but target is %q", value, meta.Slug)
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "phase_identity", Section: "frontmatter", Phase: validationIntPointer(phaseID), Detail: detail}, detail)
	}
	if value, found := incomingFront["slug"]; found {
		if fmt.Sprint(value) != meta.Slug {
			detail := fmt.Sprintf("edit document cannot change phase slug from %q to %q", meta.Slug, value)
			return applyOperation{}, newValidationFailure(validationRecord{Rule: "phase_identity", Section: "frontmatter", Phase: validationIntPointer(phaseID), Detail: detail}, detail)
		}
	}
	if meta.Slug != phaseSlugForID(phases, phaseID) {
		detail := "phase edit cannot change the phase slug"
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "phase_identity", Section: "frontmatter", Phase: validationIntPointer(phaseID), Detail: detail}, detail)
	}
	title := markdownTitle(documentBody(raw))
	if title == "unnamed phase" {
		detail := "phase document must contain a Markdown title"
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "phase_document", Section: "phase", Phase: validationIntPointer(phaseID), Detail: detail}, detail)
	}
	planned, completion, err := splitPhaseDocumentSections(title, documentBody(raw))
	if err != nil {
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "phase_document", Section: "phase", Phase: validationIntPointer(phaseID), Detail: err.Error()}, err.Error())
	}
	if planned == "" || completion == "" {
		detail := fmt.Sprintf("phase %q work and completion must not be empty", title)
		return applyOperation{}, newValidationFailure(validationRecord{Rule: "phase_document", Section: "phase", Phase: validationIntPointer(phaseID), Detail: detail}, detail)
	}
	delete(normalizedFront, "planr_edit")
	delete(normalizedFront, "planr_target")
	delete(normalizedFront, "planr_base")
	delete(normalizedFront, "planr_phase")
	delete(normalizedFront, "planr_slug")
	delete(normalizedFront, "planr_section")
	delete(normalizedFront, "planr_new")
	delete(normalizedFront, "planr_plan")
	delete(normalizedFront, "phase_title")
	delete(normalizedFront, "phase")
	delete(normalizedFront, "slug")
	normalizedFront["status"] = currentStatus
	if completedAt, found := currentFront["completed_at"]; found {
		normalizedFront["completed_at"] = completedAt
	} else {
		delete(normalizedFront, "completed_at")
	}
	newPhase, err := renderFrontmatterDocument(normalizedFront, documentBody(raw))
	if err != nil {
		return applyOperation{}, err
	}
	documents := map[string]string{target: newPhase}
	diffs := []applyDiffJSON{{Path: absolutePath(target), Before: string(currentRaw), After: newPhase}}
	if title != markdownTitle(currentBody) {
		planPath := filepath.Join(planRoot, "PLAN.md")
		planRaw, readErr := os.ReadFile(planPath)
		if readErr != nil {
			return applyOperation{}, readErr
		}
		_, planBody, frontErr := frontmatter(string(planRaw))
		if frontErr != nil {
			return applyOperation{}, frontErr
		}
		updatedBody, updateErr := replacePhaseChecklistEntry(planBody, phaseID, title, meta.Slug, currentStatus == "done")
		if updateErr != nil {
			return applyOperation{}, updateErr
		}
		updatedPlan, renderErr := documentWithBody(string(planRaw), updatedBody)
		if renderErr != nil {
			return applyOperation{}, renderErr
		}
		documents[planPath] = updatedPlan
		diffs = append(diffs, applyDiffJSON{Path: absolutePath(planPath), Before: string(planRaw), After: updatedPlan})
	}
	op := makeOperation("edit_phase", planName(planDirectory)+"#"+strconv.Itoa(phaseID), dryRun, documents, diffs)
	if dryRun || !op.changed {
		return op, nil
	}
	if err := writeFileAtomically(target, newPhase); err != nil {
		return applyOperation{}, err
	}
	if planPath, ok := changedPlanPath(documents, target); ok {
		if err := writeFileAtomically(planPath, documents[planPath]); err != nil {
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

func documentBody(raw []byte) string {
	_, body, err := frontmatter(string(raw))
	if err != nil {
		return string(raw)
	}
	return body
}

// documentWithBody replaces only the body of a frontmatter document. Edits to
// a derived or prose section must not reserialize unrelated frontmatter and
// accidentally create a noisy change.
func documentWithBody(raw, body string) (string, error) {
	if !strings.HasPrefix(raw, "---\n") {
		return body, nil
	}
	_, currentBody, err := frontmatter(raw)
	if err != nil {
		return "", err
	}
	offset := len(raw) - len(currentBody)
	if offset < 0 || offset > len(raw) {
		return "", fmt.Errorf("could not locate document body")
	}
	return raw[:offset] + body, nil
}

func phaseSlugForID(phases []storedPhase, id int) string {
	for _, phase := range phases {
		if phase.id == id {
			return phase.slug
		}
	}
	return ""
}

func editablePhaseMeta(front map[string]any, planDirectory string, phaseID int, phases []storedPhase) (phaseMeta, map[string]any, error) {
	slug := phaseSlugForID(phases, phaseID)
	meta := phaseMeta{Phase: phaseID, Slug: slug}
	status, _ := front["status"].(string)
	meta.Status = status
	if perf, ok := front["perf_phase"].(bool); ok {
		meta.PerfPhase = perf
	}
	if condition, ok := front["entry_condition"].(string); ok && strings.TrimSpace(condition) != "" {
		value := strings.TrimSpace(condition)
		meta.EntryCondition = &value
	}
	dependencies, normalized, err := editablePhaseDependencies(front["depends_on"], planDirectory, phaseID, phases)
	if err != nil {
		return phaseMeta{}, nil, err
	}
	meta.DependsOn = dependencies
	normalizedFront := copyFrontmatter(front)
	normalizedFront["depends_on"] = normalized
	if err := validateNewPhaseEditDependencies(planDirectory, meta, phases); err != nil {
		return phaseMeta{}, nil, err
	}
	return meta, normalizedFront, nil
}

func editablePhaseDependencies(value any, planDirectory string, phaseID int, phases []storedPhase) ([]int, any, error) {
	bySlug := map[string]int{}
	for _, phase := range phases {
		bySlug[phase.slug] = phase.id
	}
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
	ids := []int{}
	seen := map[int]bool{}
	for _, item := range values {
		var id int
		switch typed := item.(type) {
		case int:
			id = typed
		case int64:
			id = int(typed)
		case float64:
			if typed != float64(int(typed)) {
				return nil, nil, fmt.Errorf("phase dependency %v must be a whole phase number", typed)
			}
			id = int(typed)
		case string:
			raw := strings.TrimSpace(typed)
			if dependency, err := parseDependency(raw); err == nil && dependency.phase != nil {
				if dependency.plan != planName(planDirectory) {
					return nil, nil, fmt.Errorf("phase dependency %q must reference a phase in %s", raw, planName(planDirectory))
				}
				id = *dependency.phase
			} else if parsed, parseErr := strconv.Atoi(raw); parseErr == nil {
				id = parsed
			} else if found, ok := bySlug[raw]; ok {
				id = found
			} else {
				return nil, nil, fmt.Errorf("phase dependency %q is neither a phase number nor an existing phase slug", raw)
			}
		default:
			return nil, nil, fmt.Errorf("phase depends_on entries must be phase numbers or strings")
		}
		if id < 0 {
			return nil, nil, fmt.Errorf("phase dependency %d must be non-negative", id)
		}
		if seen[id] {
			return nil, nil, fmt.Errorf("phase dependency %d is listed more than once", id)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	sort.Ints(ids)
	normalized := make([]string, len(ids))
	for index, id := range ids {
		normalized[index] = fmt.Sprintf("%s#%d", planDirectory, id)
	}
	return ids, normalized, nil
}

func replacePhaseChecklistEntry(body string, phaseID int, title, slug string, done bool) (string, error) {
	marker := fmt.Sprintf("[Phase %02d:", phaseID)
	lines := strings.SplitAfter(body, "\n")
	replaced := 0
	for index, line := range lines {
		if !strings.Contains(line, marker) || !strings.Contains(strings.TrimSpace(line), "- [") {
			continue
		}
		lines[index] = phaseChecklistEntry(phaseID, title, slug, done) + "\n"
		if !strings.HasSuffix(line, "\n") {
			lines[index] = strings.TrimSuffix(lines[index], "\n")
		}
		replaced++
	}
	if replaced == 0 {
		return body, fmt.Errorf("checklist entry for phase %02d not found", phaseID)
	}
	if replaced > 1 {
		return body, fmt.Errorf("multiple checklist entries found for phase %02d", phaseID)
	}
	return strings.Join(lines, ""), nil
}

func validateNewPhaseEditDependencies(planDirectory string, edited phaseMeta, phases []storedPhase) error {
	all := make([]draftPhase, 0, len(phases))
	for _, phase := range phases {
		meta := phaseMeta{Phase: phase.id, Slug: phase.slug, Status: phase.status}
		if phase.id == edited.Phase {
			meta = edited
		} else {
			for _, raw := range phase.dependencies {
				dependency, err := parseDependency(raw)
				if err != nil || dependency.phase == nil || dependency.plan != planName(planDirectory) {
					return fmt.Errorf("phase %d has invalid internal dependency %q", phase.id, raw)
				}
				meta.DependsOn = append(meta.DependsOn, *dependency.phase)
			}
		}
		all = append(all, draftPhase{Title: phase.title, Meta: meta})
	}
	if err := validatePhaseDependencies(all); err != nil {
		return err
	}
	return nil
}

func applySectionEdit(raw []byte, currentRaw []byte, target, planDirectory, section string, dryRun, jsonOutput bool) (applyOperation, error) {
	_, incomingBody, err := frontmatter(string(raw))
	if err != nil {
		return applyOperation{}, err
	}
	_, currentBody, err := frontmatter(string(currentRaw))
	if err != nil {
		return applyOperation{}, err
	}
	var updatedBody string
	switch section {
	case "plan":
		incomingStart, incomingEnd, incomingFound := doctorChecklistBounds(incomingBody)
		if !incomingFound || strings.TrimSpace(incomingBody[incomingStart:incomingEnd]) != planChecklistPlaceholder {
			detail := "plan section checkout must keep the derived checklist region unchanged"
			return applyOperation{}, newValidationFailure(validationRecord{Rule: "derived_region", Section: "PLAN", Detail: detail}, detail)
		}
		start, end, found := doctorChecklistBounds(currentBody)
		if !found {
			detail := "PLAN.md does not contain a # Phases section"
			return applyOperation{}, newValidationFailure(validationRecord{Rule: "derived_region", Section: "PLAN", Detail: detail}, detail)
		}
		updatedBody = incomingBody[:incomingStart] + currentBody[start:end] + incomingBody[incomingEnd:]
	default:
		updatedBody = incomingBody
	}
	updated, err := documentWithBody(string(currentRaw), updatedBody)
	if err != nil {
		return applyOperation{}, err
	}
	op := makeOperation("edit_"+section, planName(planDirectory), dryRun,
		map[string]string{target: updated}, []applyDiffJSON{{Path: absolutePath(target), Before: string(currentRaw), After: updated}})
	if dryRun || !op.changed {
		return op, nil
	}
	if !jsonOutput {
		fmt.Printf("Updated %s\n", target)
	}
	return op, writeFileAtomically(target, updated)
}
