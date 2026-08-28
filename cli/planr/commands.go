package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/urfave/cli/v3"
)

func newCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() < 1 || cmd.NArg() > 2 {
		return fmt.Errorf("new requires <plan-name> and a short description")
	}
	selector := cmd.Args().First()
	if strings.Contains(selector, "#") {
		if cmd.NArg() != 1 {
			return fmt.Errorf("phase draft selector must be the only positional argument")
		}
		return newPhaseCommand(cmd, selector)
	}
	return newPlanCommand(cmd)
}

func newPlanCommand(cmd *cli.Command) error {
	name := cmd.Args().First()
	if !kebab.MatchString(name) {
		return fmt.Errorf("plan name %q must be lowercase kebab-case", name)
	}
	descriptionInput := cmd.String("description")
	if cmd.NArg() == 2 {
		if descriptionInput != "" {
			return fmt.Errorf("description must be provided either as the second argument or with --description, not both")
		}
		descriptionInput = cmd.Args().Get(1)
	}
	description, err := requireDescription(descriptionInput)
	if err != nil {
		return err
	}
	output := cmd.String("output")
	if output == "" {
		output = name + ".md"
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
	dependsOn, err := normalizePlanDependencies(cmd.StringSlice("depends-on"), name)
	if err != nil {
		return fmt.Errorf("invalid dependencies for plan %q: %w", name, err)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	settings, repoRoot, err := config.Load(workingDirectory)
	if err != nil {
		return err
	}
	settings = commandSettings(settings, cmd)
	if err := runDocumentHooks(repoRoot, settings, "before", hooks.EventNew, name, -1, "draft", cmd.Bool("json")); err != nil {
		return err
	}
	draft, err := doc.RenderNewDraft(settings.Language, name, dependsOn, description)
	if err != nil {
		return err
	}
	if cmd.Bool("json") {
		if err := writeJSON(makeTemplateJSON("plan", name, draft)); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(absOutput, []byte(draft), 0644); err != nil {
			return err
		}
		fmt.Printf("Created %s\n", absOutput)
	}
	if err := runDocumentHooks(repoRoot, settings, "after", hooks.EventNew, name, -1, "draft", cmd.Bool("json")); err != nil {
		return err
	}
	return nil
}

func requireDescription(value string) (string, error) {
	return normalizeDescription(value, true)
}

func normalizeDescription(value string, required bool) (string, error) {
	count := utf8.RuneCountInString(value)
	if count > 200 {
		return "", fmt.Errorf("description must be 200 characters or fewer (including spaces); got %d", count)
	}
	description := strings.TrimSpace(value)
	if required && description == "" {
		return "", fmt.Errorf("new requires --description (a short description up to 200 characters)")
	}
	return description, nil
}

func normalizePlanDependencies(values []string, plan string) ([]string, error) {
	result, err := canonicalPlanDependencies(values)
	if err != nil {
		return nil, err
	}
	for _, dependency := range result {
		parsed, _ := parseDependency(dependency)
		if parsed.plan == plan {
			return nil, fmt.Errorf("plan %q cannot depend on itself (dependency %q)", plan, dependency)
		}
	}
	return result, nil
}

func canonicalPlanDependencies(values []string) ([]string, error) {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("--depends-on must not be empty")
		}
		dependency, err := parseDependency(value)
		if err != nil {
			return nil, err
		}
		canonical := dependency.plan
		if dependency.phase != nil {
			canonical = fmt.Sprintf("%s#%d", canonical, *dependency.phase)
		}
		if !seen[canonical] {
			seen[canonical] = true
			result = append(result, canonical)
		} else {
			return nil, fmt.Errorf("duplicate plan dependency %q", canonical)
		}
	}
	return result, nil
}

type planDependency struct {
	plan  string
	phase *int
}

func parseDependency(value string) (planDependency, error) {
	plan, phaseText, hasPhase := strings.Cut(value, "#")
	plan = planName(plan)
	if !kebab.MatchString(plan) {
		return planDependency{}, fmt.Errorf("dependency %q must use plan-name or plan-name#phase-number", value)
	}
	if !hasPhase {
		return planDependency{plan: plan}, nil
	}
	phase, err := strconv.Atoi(phaseText)
	if err != nil || phase < 0 {
		return planDependency{}, fmt.Errorf("dependency %q must use a non-negative phase number", value)
	}
	return planDependency{plan: plan, phase: &phase}, nil
}

// planDirectoryPrefix matches a numbered plan directory name, capturing its
// index and plan name.
var planDirectoryPrefix = regexp.MustCompile(`^(\d+)-(.+)$`)

func nextPlanDirectory(planDirectories []string, name string) (string, error) {
	maxIndex := -1
	for _, directory := range planDirectories {
		entries, err := os.ReadDir(directory)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			match := planDirectoryPrefix.FindStringSubmatch(entry.Name())
			if len(match) != 3 {
				continue
			}
			if match[2] == name {
				return "", fmt.Errorf("plan %q already exists", name)
			}
			index, err := strconv.Atoi(match[1])
			if err != nil {
				continue
			}
			if index > maxIndex {
				maxIndex = index
			}
		}
	}
	return fmt.Sprintf("%02d-%s", maxIndex+1, name), nil
}

func writePlan(root string, d draft, planDirectory, language string) error {
	documents, err := renderPlanDocuments(d, planDirectory, language, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "phases"), 0755); err != nil {
		return err
	}
	for relative, contents := range documents {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(relative)), []byte(contents), 0644); err != nil {
			return err
		}
	}
	return nil
}

// renderPlanDocuments produces the complete set of files written when a plan
// is registered. Keeping this separate from writePlan lets apply --dry-run
// return the exact resulting documents without touching the repository.
func renderPlanDocuments(d draft, planDirectory, language, registeredAt string) (map[string]string, error) {
	text := doc.StringsFor(language)
	documents := map[string]string{
		"GOALS.md":   "# GOALS\n\n" + d.Goals + "\n",
		"CONTEXT.md": "# SCOPE\n\n" + d.Scope + "\n\n# CONTEXT\n\n" + d.Context + "\n",
	}
	checklist := []string{}
	for _, p := range d.Phases {
		checklist = append(checklist, phaseChecklistEntry(p.Meta.Phase, p.Title, p.Meta.Slug, p.Meta.Status == "done"))
		path := phaseDocumentPath(p.Meta.Phase, p.Meta.Slug)
		contents, err := renderFrontmatterDocument(phaseFrontmatter(planDirectory, p.Meta), phaseDocumentBody(language, p.Title, p.Planned, p.Completion))
		if err != nil {
			return nil, err
		}
		documents[path] = contents
	}
	meta := map[string]any{"description": d.Description, "registered_at": registeredAt, "plan_status": "in-progress", "depends_on": d.DependsOn, "succeeded_by": nil, "preceded_by": nil}
	header, err := yaml.Marshal(pruneEmptyMeta(meta))
	if err != nil {
		return nil, err
	}
	nextDoc := ""
	for _, p := range d.Phases {
		if p.Meta.Phase == d.NextPhase {
			nextDoc = phaseDocumentPath(p.Meta.Phase, p.Meta.Slug)
		}
	}
	documents["PLAN.md"] = fmt.Sprintf("---\n%s---\n> NEXT: %s ([Phase %d](%s))\n\n# Phases\n\n%s\n\n# %s\n\n%s\n\n# %s\n\n%s\n\n# %s\n\n%s\n",
		header, d.NextText, d.NextPhase, nextDoc, strings.Join(checklist, "\n"),
		text.Verification, d.Verification,
		text.Ordering, d.Ordering,
		text.NextTarget, d.NextText)
	return documents, nil
}

func renderFrontmatterDocument(front map[string]any, body string) (string, error) {
	header, err := yaml.Marshal(pruneEmptyMeta(front))
	if err != nil {
		return "", err
	}
	return "---\n" + string(header) + "---\n" + body, nil
}

// phaseFilePrefix matches a phase document filename, capturing its number and slug.
var phaseFilePrefix = regexp.MustCompile(`^(\d+)-(.*)\.md$`)

// phaseDocumentPath is the plan-relative location of a phase document. The
// same shape is matched by phaseFilePrefix when reading phases back.
func phaseDocumentPath(id int, slug string) string {
	return fmt.Sprintf("phases/%02d-%s.md", id, slug)
}

func phaseChecklistEntry(id int, title, slug string, done bool) string {
	checkmark := " "
	if done {
		checkmark = "x"
	}
	return fmt.Sprintf("- [%s] [Phase %02d: %s](%s)", checkmark, id, title, phaseDocumentPath(id, slug))
}

// transformChecklistEntry rewrites the single checklist line for a phase in a
// PLAN.md body. transform receives the matching line (including its trailing
// newline, when present) and returns the replacement plus true to count the
// line as handled; returning an empty replacement drops the line, and false
// keeps the original line without counting it as a match.
func transformChecklistEntry(body string, phaseID int, transform func(line string) (string, bool)) (string, error) {
	marker := fmt.Sprintf("[Phase %02d:", phaseID)
	lines := strings.SplitAfter(body, "\n")
	matched := 0
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if !strings.Contains(line, marker) || !strings.Contains(strings.TrimSpace(line), "- [") {
			result = append(result, line)
			continue
		}
		replacement, handled := transform(line)
		if !handled {
			result = append(result, line)
			continue
		}
		matched++
		if replacement != "" {
			result = append(result, replacement)
		}
	}
	if matched == 0 {
		return body, fmt.Errorf("checklist entry for phase %02d not found", phaseID)
	}
	if matched > 1 {
		return body, fmt.Errorf("multiple checklist entries found for phase %02d", phaseID)
	}
	return strings.Join(result, ""), nil
}

func phaseFrontmatter(planDirectory string, meta phaseMeta) map[string]any {
	dependencies := make([]string, len(meta.DependsOn))
	for index, dependency := range meta.DependsOn {
		dependencies[index] = fmt.Sprintf("%s#%d", planDirectory, dependency)
	}
	return map[string]any{
		"status":          meta.Status,
		"entry_condition": meta.EntryCondition,
		"perf_phase":      meta.PerfPhase,
		"depends_on":      dependencies,
		"blocks":          []string{},
	}
}

func phaseDocumentBody(language, title, planned, completion string) string {
	text := doc.StringsFor(language)
	return fmt.Sprintf("> DONE-WHEN: %s\n> NEXT: %s\n\n# %s\n\n## %s\n\n%s\n\n## %s\n\n%s\n",
		firstPhaseLine(completion), text.NoNext, title, text.PlannedWork, planned, text.DoneWhen, completion)
}

func firstPhaseLine(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(strings.SplitN(value, "\n", 2)[0]), "- ")
}

// planSummary is the shared on-disk view of a plan used by both `status` and
// `overview`; each command only differs in how it renders it.
type planSummary struct {
	name, label, status string
	dependsOn           []planDependency
	phases              []storedPhase
	wait                []string
}

func (p *planSummary) addDependency(raw string) {
	dependency, err := parseDependency(raw)
	if err != nil {
		return
	}
	for _, existing := range p.dependsOn {
		if existing.plan == dependency.plan && sameDependencyPhase(existing, dependency) {
			return
		}
	}
	p.dependsOn = append(p.dependsOn, dependency)
}

// progress reports completed and total phase counts plus the first phase that
// is not done yet.
func (p planSummary) progress() (done, total int, next string) {
	for _, phase := range p.phases {
		total++
		if phase.status == "done" {
			done++
		} else if next == "" {
			next = phase.title
		}
	}
	return done, total, next
}

func sameDependencyPhase(left, right planDependency) bool {
	if left.phase == nil || right.phase == nil {
		return left.phase == nil && right.phase == nil
	}
	return *left.phase == *right.phase
}

// collectPlanSummaries reads every plan under planDirectories, optionally
// keeping only the one matching filter (by directory or plan name).
func collectPlanSummaries(planDirectories []string, filter string) ([]planSummary, bool, error) {
	summaries := []planSummary{}
	foundDirectory := false
	for _, plans := range planDirectories {
		entries, err := os.ReadDir(plans)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		foundDirectory = true
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if filter != "" && entry.Name() != filter && planName(entry.Name()) != filter {
				continue
			}
			planRoot := filepath.Join(plans, entry.Name())
			raw, err := os.ReadFile(filepath.Join(planRoot, "PLAN.md"))
			if err != nil {
				continue
			}
			front, _, err := frontmatter(string(raw))
			if err != nil {
				return nil, false, fmt.Errorf("%s: %w", entry.Name(), err)
			}
			phases, err := readPlanPhases(planRoot)
			if err != nil {
				return nil, false, err
			}
			status, _ := front["plan_status"].(string)
			summary := planSummary{
				name:   planName(entry.Name()),
				label:  filepath.Join(filepath.Base(plans), entry.Name()),
				status: status,
				phases: phases,
			}
			for _, dependency := range yamlStrings(front["depends_on"]) {
				summary.addDependency(dependency)
			}
			if status != "done" {
				for _, phase := range phases {
					for _, dependency := range phase.dependencies {
						if _, _, found := strings.Cut(dependency, "#"); found {
							summary.addDependency(dependency)
						}
					}
				}
			}
			summaries = append(summaries, summary)
		}
	}
	return summaries, foundDirectory, nil
}

// annotatePlanWaits fills in each summary's unmet dependencies and returns the
// set of plans some open plan still depends on.
func annotatePlanWaits(summaries []planSummary) map[string]bool {
	required := map[string]bool{}
	byName := map[string]*planSummary{}
	for index := range summaries {
		byName[summaries[index].name] = &summaries[index]
	}
	for index := range summaries {
		summary := &summaries[index]
		if summary.status == "done" {
			continue
		}
		for _, dependency := range summary.dependsOn {
			required[dependency.plan] = true
			if dependency.plan == summary.name {
				continue
			}
			label := dependencyLabel(dependency)
			target, found := byName[dependency.plan]
			if !found {
				summary.wait = append(summary.wait, label+" (not found)")
				continue
			}
			if dependency.phase == nil {
				if target.status != "done" {
					summary.wait = append(summary.wait, fmt.Sprintf("%s (%s)", label, target.status))
				}
				continue
			}
			phaseFound := false
			for _, phase := range target.phases {
				if phase.id != *dependency.phase {
					continue
				}
				phaseFound = true
				if phase.status != "done" {
					summary.wait = append(summary.wait, fmt.Sprintf("%s (%s)", label, phase.status))
				}
				break
			}
			if !phaseFound {
				summary.wait = append(summary.wait, label+" (phase not found)")
			}
		}
	}
	return required
}

// printPlanGroups prints each summary grouped by its plans directory, letting
// the caller render the per-plan detail lines.
func printPlanGroups(summaries []planSummary, render func(name string, summary planSummary)) {
	currentDirectory := ""
	for _, summary := range summaries {
		directory, name := filepath.Split(summary.label)
		if directory != currentDirectory {
			fmt.Printf("%s\n", directory)
			currentDirectory = directory
		}
		render(name, summary)
	}
}

func printPlanList(title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Printf("    %s:\n", title)
	for _, value := range values {
		fmt.Printf("      - %s\n", value)
	}
}

func statusCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() > 1 {
		return fmt.Errorf("status accepts at most one plan name")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	planDirectories, err := config.PlanPaths(cwd)
	if err != nil {
		return err
	}
	summaries, _, err := collectPlanSummaries(planDirectories, cmd.Args().First())
	if err != nil {
		return err
	}
	// A repository with no plans yet is an empty result, not a failure; only an
	// explicitly requested plan that does not exist is an error.
	if len(summaries) == 0 {
		if filter := cmd.Args().First(); filter != "" {
			return fmt.Errorf("plan %q not found", filter)
		}
		if !cmd.Bool("json") {
			fmt.Println("No plans found")
			return nil
		}
	}
	requiredPlans := annotatePlanWaits(summaries)
	if cmd.NArg() == 0 {
		// Completed plans stay hidden unless an open plan still depends on them.
		visible := summaries[:0]
		for _, summary := range summaries {
			if summary.status != "done" || requiredPlans[summary.name] {
				visible = append(visible, summary)
			}
		}
		summaries = visible
	}
	if cmd.Bool("json") {
		return writeJSON(makeStatusJSON(summaries))
	}
	printPlanGroups(summaries, func(name string, summary planSummary) {
		done, total, _ := summary.progress()
		fmt.Printf("  %s: %s (%d/%d phases done)\n", name, summary.status, done, total)
		remaining := []string{}
		for _, phase := range summary.phases {
			if phase.status != "done" {
				remaining = append(remaining, fmt.Sprintf("%s (%s)", phase.title, phase.status))
			}
		}
		printPlanList("remaining", remaining)
		printPlanList("wait", summary.wait)
	})
	return nil
}

func markdownTitle(contents string) string {
	for _, line := range strings.Split(contents, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return "unnamed phase"
}

type storedPhase struct {
	id                  int
	slug, title, status string
	dependencies        []string
}

func readPlanPhases(planRoot string) ([]storedPhase, error) {
	entries, err := os.ReadDir(filepath.Join(planRoot, "phases"))
	if err != nil {
		return nil, fmt.Errorf("read phases for %s: %w", filepath.Base(planRoot), err)
	}
	phases := []storedPhase{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := phaseFilePrefix.FindStringSubmatch(entry.Name())
		if len(match) != 3 {
			continue
		}
		id, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(planRoot, "phases", entry.Name()))
		if err != nil {
			return nil, err
		}
		front, _, err := frontmatter(string(contents))
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", filepath.Base(planRoot), entry.Name(), err)
		}
		status, _ := front["status"].(string)
		phases = append(phases, storedPhase{id, match[2], markdownTitle(string(contents)), status, yamlStrings(front["depends_on"])})
	}
	sort.Slice(phases, func(i, j int) bool { return phases[i].id < phases[j].id })
	return phases, nil
}

func planName(directory string) string {
	parts := strings.SplitN(directory, "-", 2)
	if len(parts) == 2 {
		if len(parts[0]) >= 2 {
			if _, err := strconv.Atoi(parts[0]); err == nil {
				return parts[1]
			}
		}
	}
	return directory
}

func dependencyLabel(dependency planDependency) string {
	if dependency.phase == nil {
		return dependency.plan
	}
	return fmt.Sprintf("%s#%d", dependency.plan, *dependency.phase)
}

func yamlStrings(value any) []string {
	values, _ := value.([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}
