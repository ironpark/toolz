package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/vfs"
)

// planDirectoryPrefix matches a numbered plan directory name, capturing its
// index and plan name.
var planDirectoryPrefix = regexp.MustCompile(`^(\d+)-(.+)$`)

// PhaseFilePrefix matches a phase document filename, capturing its number and slug.
var PhaseFilePrefix = regexp.MustCompile(`^(\d+)-(.*)\.md$`)

// Summary is the shared on-disk view of a plan used by both `status` and
// `overview`; each command only differs in how it renders it.
type Summary struct {
	Name, Label, Status string
	DependsOn           []draft.Dependency
	Phases              []StoredPhase
	Wait                []string
}

type StoredPhase struct {
	ID                  int
	Slug, Title, Status string
	Dependencies        []string
}

func (p *Summary) AddDependency(raw string) {
	dependency, err := draft.ParseDependency(raw)
	if err != nil {
		return
	}
	for _, existing := range p.DependsOn {
		if existing.Plan == dependency.Plan && draft.SameDependencyPhase(existing, dependency) {
			return
		}
	}
	p.DependsOn = append(p.DependsOn, dependency)
}

// Progress reports completed and total phase counts plus the first phase that
// is not done yet.
func (p Summary) Progress() (done, total int, next string) {
	for _, phase := range p.Phases {
		total++
		if phase.Status == "done" {
			done++
		} else if next == "" {
			next = phase.Title
		}
	}
	return done, total, next
}

func NextDirectory(planDirectories []string, name string) (string, error) {
	maxIndex := -1
	for _, directory := range planDirectories {
		entries, err := vfs.ReadDir(directory)
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

func Write(root string, d draft.Draft, planDirectory, language string) error {
	documents, err := RenderDocuments(d, planDirectory, language, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	if err := vfs.MkdirAll(filepath.Join(root, "phases"), 0755); err != nil {
		return err
	}
	for relative, contents := range documents {
		if err := vfs.WriteFile(filepath.Join(root, filepath.FromSlash(relative)), []byte(contents), 0644); err != nil {
			return err
		}
	}
	return nil
}

// RenderDocuments produces the complete set of files written when a plan is
// registered. Rendering is separate from writing so apply --dry-run can report
// the exact resulting documents without touching the repository.
func RenderDocuments(d draft.Draft, planDirectory, language, registeredAt string) (map[string]string, error) {
	text := doc.StringsFor(language)
	documents := map[string]string{
		"GOALS.md":   "# GOALS\n\n" + d.Goals + "\n",
		"CONTEXT.md": "# SCOPE\n\n" + d.Scope + "\n\n# CONTEXT\n\n" + d.Context + "\n",
	}
	checklist := []string{}
	for _, p := range d.Phases {
		checklist = append(checklist, ChecklistEntry(p.Meta.Phase, p.Title, p.Meta.Slug, p.Meta.Status == "done"))
		path := PhaseDocumentPath(p.Meta.Phase, p.Meta.Slug)
		contents, err := mdoc.Render(PhaseFrontmatter(planDirectory, p.Meta), PhaseDocumentBody(language, p.Title, p.Planned, p.Completion))
		if err != nil {
			return nil, err
		}
		documents[path] = contents
	}
	meta := map[string]any{"description": d.Description, "registered_at": registeredAt, "plan_status": "in-progress", "depends_on": d.DependsOn, "succeeded_by": nil, "preceded_by": nil}
	header, err := yaml.Marshal(mdoc.PruneEmptyMeta(meta))
	if err != nil {
		return nil, err
	}
	nextDoc := ""
	for _, p := range d.Phases {
		if p.Meta.Phase == d.NextPhase {
			nextDoc = PhaseDocumentPath(p.Meta.Phase, p.Meta.Slug)
		}
	}
	documents["PLAN.md"] = fmt.Sprintf("---\n%s---\n> NEXT: %s ([Phase %d](%s))\n\n# Phases\n\n%s\n\n# %s\n\n%s\n\n# %s\n\n%s\n\n# %s\n\n%s\n",
		header, d.NextText, d.NextPhase, nextDoc, strings.Join(checklist, "\n"),
		text.Verification, d.Verification,
		text.Ordering, d.Ordering,
		text.NextTarget, d.NextText)
	return documents, nil
}

// PhaseDocumentPath is the plan-relative location of a phase document. The
// same shape is matched by PhaseFilePrefix when reading phases back.
func PhaseDocumentPath(id int, slug string) string {
	return fmt.Sprintf("phases/%02d-%s.md", id, slug)
}

// sectionFiles maps the editable/showable plan section names to the documents
// they live in; it also serves as the section-name validation set.
var sectionFiles = map[string]string{
	"goals":   "GOALS.md",
	"context": "CONTEXT.md",
	"plan":    "PLAN.md",
}

// ValidSection reports whether section names an editable plan section.
func ValidSection(section string) bool {
	_, ok := sectionFiles[section]
	return ok
}

// SectionFile returns the plan document a section lives in.
func SectionFile(section string) string {
	if file, ok := sectionFiles[section]; ok {
		return file
	}
	return "PLAN.md"
}

func ChecklistEntry(id int, title, slug string, done bool) string {
	checkmark := " "
	if done {
		checkmark = "x"
	}
	return fmt.Sprintf("- [%s] [Phase %02d: %s](%s)", checkmark, id, title, PhaseDocumentPath(id, slug))
}

// TransformChecklistEntry rewrites the single checklist line for a phase in a
// PLAN.md body. transform receives the matching line (including its trailing
// newline, when present) and returns the replacement plus true to count the
// line as handled; returning an empty replacement drops the line, and false
// keeps the original line without counting it as a match.
func TransformChecklistEntry(body string, phaseID int, transform func(line string) (string, bool)) (string, error) {
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

// AppendChecklistEntry inserts a checklist entry for a new phase at the end of
// the `# Phases` section of a PLAN.md body.
func AppendChecklistEntry(body string, phaseID int, title, slug string) (string, error) {
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
	entry := ChecklistEntry(phaseID, title, slug, false)
	before := strings.TrimRight(body[:insertion], "\n")
	after := strings.TrimLeft(body[insertion:], "\n")
	if after == "" {
		return before + "\n\n" + entry + "\n", nil
	}
	return before + "\n" + entry + "\n\n" + after, nil
}

// ReplaceChecklistEntry rewrites the checklist entry of an existing phase,
// which is how a phase title change propagates back into PLAN.md.
func ReplaceChecklistEntry(body string, phaseID int, title, slug string, done bool) (string, error) {
	return TransformChecklistEntry(body, phaseID, func(line string) (string, bool) {
		replacement := ChecklistEntry(phaseID, title, slug, done) + "\n"
		if !strings.HasSuffix(line, "\n") {
			replacement = strings.TrimSuffix(replacement, "\n")
		}
		return replacement, true
	})
}

func PhaseFrontmatter(planDirectory string, meta draft.Meta) map[string]any {
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

func PhaseDocumentBody(language, title, planned, completion string) string {
	text := doc.StringsFor(language)
	return fmt.Sprintf("> DONE-WHEN: %s\n> NEXT: %s\n\n# %s\n\n## %s\n\n%s\n\n## %s\n\n%s\n",
		firstPhaseLine(completion), text.NoNext, title, text.PlannedWork, planned, text.DoneWhen, completion)
}

func firstPhaseLine(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(strings.SplitN(value, "\n", 2)[0]), "- ")
}

// CollectSummaries reads every plan under planDirectories, optionally
// keeping only the one matching filter (by directory or plan name).
func CollectSummaries(planDirectories []string, filter string) ([]Summary, bool, error) {
	summaries := []Summary{}
	foundDirectory := false
	for _, plans := range planDirectories {
		entries, err := vfs.ReadDir(plans)
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
			if filter != "" && entry.Name() != filter && draft.PlanName(entry.Name()) != filter {
				continue
			}
			planRoot := filepath.Join(plans, entry.Name())
			raw, err := vfs.ReadFile(filepath.Join(planRoot, "PLAN.md"))
			if err != nil {
				continue
			}
			front, _, err := mdoc.Split(string(raw))
			if err != nil {
				return nil, false, fmt.Errorf("%s: %w", entry.Name(), err)
			}
			phases, err := ReadPhases(planRoot)
			if err != nil {
				return nil, false, err
			}
			status := mdoc.FrontString(front, "plan_status")
			summary := Summary{
				Name:   draft.PlanName(entry.Name()),
				Label:  filepath.Join(filepath.Base(plans), entry.Name()),
				Status: status,
				Phases: phases,
			}
			for _, dependency := range mdoc.Strings(front["depends_on"]) {
				summary.AddDependency(dependency)
			}
			if status != "done" {
				for _, phase := range phases {
					for _, dependency := range phase.Dependencies {
						if _, _, found := strings.Cut(dependency, "#"); found {
							summary.AddDependency(dependency)
						}
					}
				}
			}
			summaries = append(summaries, summary)
		}
	}
	return summaries, foundDirectory, nil
}

// AnnotateWaits fills in each summary's unmet dependencies and returns the
// set of plans some open plan still depends on.
func AnnotateWaits(summaries []Summary) map[string]bool {
	required := map[string]bool{}
	byName := map[string]*Summary{}
	for index := range summaries {
		byName[summaries[index].Name] = &summaries[index]
	}
	for index := range summaries {
		summary := &summaries[index]
		if summary.Status == "done" {
			continue
		}
		for _, dependency := range summary.DependsOn {
			required[dependency.Plan] = true
			if dependency.Plan == summary.Name {
				continue
			}
			label := draft.DependencyLabel(dependency)
			target, found := byName[dependency.Plan]
			if !found {
				summary.Wait = append(summary.Wait, label+" (not found)")
				continue
			}
			if dependency.Phase == nil {
				if target.Status != "done" {
					summary.Wait = append(summary.Wait, fmt.Sprintf("%s (%s)", label, target.Status))
				}
				continue
			}
			phaseFound := false
			for _, phase := range target.Phases {
				if phase.ID != *dependency.Phase {
					continue
				}
				phaseFound = true
				if phase.Status != "done" {
					summary.Wait = append(summary.Wait, fmt.Sprintf("%s (%s)", label, phase.Status))
				}
				break
			}
			if !phaseFound {
				summary.Wait = append(summary.Wait, label+" (phase not found)")
			}
		}
	}
	return required
}

func ReadPhases(planRoot string) ([]StoredPhase, error) {
	entries, err := vfs.ReadDir(filepath.Join(planRoot, "phases"))
	if err != nil {
		return nil, fmt.Errorf("read phases for %s: %w", filepath.Base(planRoot), err)
	}
	phases := []StoredPhase{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := PhaseFilePrefix.FindStringSubmatch(entry.Name())
		if len(match) != 3 {
			continue
		}
		id, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		contents, err := vfs.ReadFile(filepath.Join(planRoot, "phases", entry.Name()))
		if err != nil {
			return nil, err
		}
		front, _, err := mdoc.Split(string(contents))
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", filepath.Base(planRoot), entry.Name(), err)
		}
		status := mdoc.FrontString(front, "status")
		phases = append(phases, StoredPhase{id, match[2], mdoc.Title(string(contents)), status, mdoc.Strings(front["depends_on"])})
	}
	sort.Slice(phases, func(i, j int) bool { return phases[i].ID < phases[j].ID })
	return phases, nil
}

type PhaseDetails struct {
	Plan, Directory string
	ID              int
	Slug, Title     string
	Status          string
	PlannedWork     string
	DoneWhen        string
	Dependencies    []string
	File            string
}

func ReadDocument(planRoot, name string) (map[string]any, string, error) {
	raw, err := vfs.ReadFile(filepath.Join(planRoot, name))
	if err != nil {
		return nil, "", err
	}
	front, body, err := mdoc.Split(string(raw))
	if err != nil {
		return nil, "", fmt.Errorf("parse %s: %w", name, err)
	}
	return front, body, nil
}

func ReadPhaseDetails(planRoot, planDirectory string, stored StoredPhase) (PhaseDetails, error) {
	phasePath, err := FindPhaseFile(planRoot, stored.ID)
	if err != nil {
		return PhaseDetails{}, fmt.Errorf("%s: %w", planDirectory, err)
	}
	raw, err := vfs.ReadFile(phasePath)
	if err != nil {
		return PhaseDetails{}, err
	}
	front, body, err := mdoc.Split(string(raw))
	if err != nil {
		return PhaseDetails{}, fmt.Errorf("parse %s: %w", filepath.Base(phasePath), err)
	}
	plannedWork, doneWhen, err := draft.SplitPhaseDocumentSections(stored.Title, body)
	if err != nil {
		return PhaseDetails{}, fmt.Errorf("parse %s: %w", filepath.Base(phasePath), err)
	}
	absPath, err := filepath.Abs(phasePath)
	if err != nil {
		return PhaseDetails{}, err
	}
	details := PhaseDetails{Plan: draft.PlanName(planDirectory), Directory: planDirectory, ID: stored.ID, Slug: stored.Slug, Title: stored.Title, Status: stored.Status, PlannedWork: plannedWork, DoneWhen: doneWhen, Dependencies: mdoc.Strings(front["depends_on"]), File: absPath}
	if status, ok := front["status"].(string); ok && status != "" {
		details.Status = status
	}
	return details, nil
}

// ChecklistBounds locates the derived phase checklist inside a PLAN.md body,
// returning the offsets of the region between the `# Phases` heading and the
// next top-level heading. Both the diagnostics and the write path need it, so
// it lives with the rest of the checklist rendering.
func ChecklistBounds(body string) (int, int, bool) {
	lines := strings.SplitAfter(body, "\n")
	offset := 0
	start := -1
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if start < 0 {
			if trimmed == "# Phases" {
				start = offset + len(line)
			}
		} else if strings.HasPrefix(trimmed, "# ") {
			return start, offset, true
		}
		offset += len(line)
	}
	if start >= 0 {
		return start, len(body), true
	}
	return -1, -1, false
}

// Details is the whole plan aggregate read off disk: the PLAN.md frontmatter,
// every plan-level document, and the details of every phase. `show --all`
// renders it directly.
type Details struct {
	Plan, Directory string
	Status          string
	Description     string
	DependsOn       []string
	Phases          []PhaseDetails
	// Documents holds every document in the plan directory keyed by its
	// plan-relative slash path, including the sectionDocuments below.
	Documents map[string]string
}

// sectionDocuments are the plan-level documents always present in a plan
// directory, in the order they are read.
var sectionDocuments = []string{"GOALS.md", "CONTEXT.md", "PLAN.md"}

// SectionDetails is one plan-level section document read off disk.
type SectionDetails struct {
	Plan, Directory string
	Section         string
	Content         string
	File            string
}

// ReadSection reads the document a plan section lives in.
func ReadSection(planRoot, planDirectory, section string) (SectionDetails, error) {
	path := filepath.Join(planRoot, SectionFile(section))
	raw, err := vfs.ReadFile(path)
	if err != nil {
		return SectionDetails{}, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return SectionDetails{}, err
	}
	return SectionDetails{
		Plan:      draft.PlanName(planDirectory),
		Directory: planDirectory,
		Section:   section,
		Content:   string(raw),
		File:      absolute,
	}, nil
}

// ReadAll assembles the complete plan aggregate from disk.
func ReadAll(planRoot, planDirectory string) (Details, error) {
	documents := map[string]string{}
	for _, relative := range sectionDocuments {
		raw, readErr := vfs.ReadFile(filepath.Join(planRoot, relative))
		if readErr != nil {
			return Details{}, readErr
		}
		documents[relative] = string(raw)
	}
	front, _, err := mdoc.Split(documents["PLAN.md"])
	if err != nil {
		return Details{}, err
	}
	stored, err := ReadPhases(planRoot)
	if err != nil {
		return Details{}, err
	}
	phases := make([]PhaseDetails, 0, len(stored))
	for _, phase := range stored {
		details, detailsErr := ReadPhaseDetails(planRoot, planDirectory, phase)
		if detailsErr != nil {
			return Details{}, detailsErr
		}
		phases = append(phases, details)
		// ReadPhaseDetails already resolved the phase file, so reuse its path
		// rather than scanning the phases directory a second time.
		raw, readErr := vfs.ReadFile(details.File)
		if readErr != nil {
			return Details{}, readErr
		}
		relative, relErr := filepath.Rel(planRoot, details.File)
		if relErr != nil {
			return Details{}, relErr
		}
		documents[filepath.ToSlash(relative)] = string(raw)
	}
	return Details{
		Plan:        draft.PlanName(planDirectory),
		Directory:   planDirectory,
		Status:      mdoc.FrontString(front, "plan_status"),
		Description: mdoc.FrontString(front, "description"),
		DependsOn:   mdoc.Strings(front["depends_on"]),
		Phases:      phases,
		Documents:   documents,
	}, nil
}
