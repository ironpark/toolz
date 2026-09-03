package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/ironpark/toolz/cli/planr/internal/vfs"
)

type Issue struct {
	Location string
	Message  string
}

type Phase struct {
	plan.StoredPhase
	path  string
	front map[string]any
}

// Inspection is everything doctor learned about one plan directory. The
// *OK flags record how far the read got, so later checks can skip work that
// would only produce noise on top of an earlier failure.
type Inspection struct {
	Directory string
	root      string
	planPath  string
	Front     map[string]any
	raw       string
	Body      string

	Phases          []Phase
	Checklist       []checklistEntry
	ChecklistIssues []Issue
	ChecklistStart  int
	PlanReadable    bool
	PlanFrontOK     bool
	PhaseDataOK     bool
}

type checklistEntry struct {
	phaseNumber int
	title       string
	path        string
	checked     bool
}

var checklistPattern = regexp.MustCompile(`^\s*-\s*\[([ xX])\]\s+\[Phase\s+([0-9]+):\s*(.*?)\]\(([^)]+)\)\s*$`)

func InspectPlan(planRoot, directory string) (Inspection, []Issue) {
	inspection := Inspection{
		Directory:   directory,
		root:        planRoot,
		planPath:    filepath.Join(planRoot, "PLAN.md"),
		PhaseDataOK: true,
	}
	issues := []Issue{}

	raw, err := vfs.ReadFile(inspection.planPath)
	if err != nil {
		issues = append(issues, Issue{Location: inspection.Directory, Message: fmt.Sprintf("cannot read PLAN.md: %v", err)})
		inspection.PhaseDataOK = false
	} else {
		if !hasDocumentFrontmatter(string(raw)) {
			issues = append(issues, Issue{Location: inspection.Directory + "/PLAN.md", Message: "missing frontmatter"})
		} else {
			front, body, frontErr := mdoc.Split(string(raw))
			if frontErr != nil {
				issues = append(issues, Issue{Location: inspection.Directory + "/PLAN.md", Message: frontErr.Error()})
			} else {
				inspection.Front, inspection.raw, inspection.Body, inspection.PlanReadable = front, string(raw), body, true
				frontIssues := validatePlanFrontmatter(directory, front)
				issues = append(issues, frontIssues...)
				inspection.PlanFrontOK = len(frontIssues) == 0
				var checklistIssues []Issue
				inspection.Checklist, inspection.ChecklistStart, checklistIssues = parseChecklist(body, directory)
				inspection.ChecklistIssues = append(inspection.ChecklistIssues, checklistIssues...)
			}
		}
	}

	phaseDirectory := filepath.Join(planRoot, "phases")
	entries, readErr := vfs.ReadDir(phaseDirectory)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			issues = append(issues, Issue{Location: inspection.Directory + "/phases", Message: "directory does not exist"})
		} else {
			issues = append(issues, Issue{Location: inspection.Directory + "/phases", Message: fmt.Sprintf("cannot read directory: %v", readErr)})
		}
		inspection.PhaseDataOK = false
	} else {
		ids := map[int]string{}
		slugs := map[string]string{}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			location := inspection.Directory + "/phases/" + entry.Name()
			match := plan.PhaseFilePrefix.FindStringSubmatch(entry.Name())
			if len(match) != 3 || !draft.KebabPattern.MatchString(match[2]) {
				issues = append(issues, Issue{Location: location, Message: "invalid phase filename; expected NN-lowercase-kebab-case.md"})
				inspection.PhaseDataOK = false
				continue
			}
			id, parseErr := strconv.Atoi(match[1])
			if parseErr != nil {
				issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("invalid phase number: %v", parseErr)})
				inspection.PhaseDataOK = false
				continue
			}
			if previous, found := ids[id]; found {
				issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("duplicate phase number %02d also used by %s", id, previous)})
				inspection.PhaseDataOK = false
				continue
			}
			if previous, found := slugs[match[2]]; found {
				issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("duplicate phase slug %q also used by %s", match[2], previous)})
				inspection.PhaseDataOK = false
				continue
			}
			ids[id] = entry.Name()
			slugs[match[2]] = entry.Name()

			phasePath := filepath.Join(phaseDirectory, entry.Name())
			phaseRaw, readPhaseErr := vfs.ReadFile(phasePath)
			if readPhaseErr != nil {
				issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("cannot read phase: %v", readPhaseErr)})
				inspection.PhaseDataOK = false
				continue
			}
			if !hasDocumentFrontmatter(string(phaseRaw)) {
				issues = append(issues, Issue{Location: location, Message: "missing frontmatter"})
				inspection.PhaseDataOK = false
				continue
			}
			phaseFront, _, frontErr := mdoc.Split(string(phaseRaw))
			if frontErr != nil {
				issues = append(issues, Issue{Location: location, Message: frontErr.Error()})
				inspection.PhaseDataOK = false
				continue
			}
			phase := Phase{
				StoredPhase: plan.StoredPhase{ID: id, Slug: match[2], Title: mdoc.Title(string(phaseRaw)), Status: stringValue(phaseFront["status"]), Dependencies: mdoc.Strings(phaseFront["depends_on"])},
				path:        phasePath,
				front:       phaseFront,
			}
			phaseIssues := validatePhase(inspection.Directory, phase)
			if len(phaseIssues) > 0 {
				inspection.PhaseDataOK = false
			}
			issues = append(issues, phaseIssues...)
			inspection.Phases = append(inspection.Phases, phase)
		}
		sort.Slice(inspection.Phases, func(i, j int) bool { return inspection.Phases[i].ID < inspection.Phases[j].ID })
		if len(inspection.Phases) == 0 {
			issues = append(issues, Issue{Location: inspection.Directory + "/phases", Message: "no phase documents found"})
			inspection.PhaseDataOK = false
		}
	}

	if inspection.PlanReadable && inspection.PhaseDataOK {
		inspection.ChecklistIssues = append(inspection.ChecklistIssues, compareChecklist(inspection)...)
		planStatus := stringValue(inspection.Front["plan_status"])
		expected := draft.StatusDone
		for _, phase := range inspection.Phases {
			if phase.Status != draft.StatusDone {
				expected = draft.StatusInProgress
				break
			}
		}
		if planStatus != "" && planStatus != expected {
			issues = append(issues, Issue{Location: inspection.Directory + "/PLAN.md", Message: fmt.Sprintf("plan_status is %q but phase files indicate %q", planStatus, expected)})
		}
	}
	return inspection, issues
}

func hasDocumentFrontmatter(raw string) bool {
	return strings.HasPrefix(raw, "---\n")
}

func validatePlanFrontmatter(directory string, front map[string]any) []Issue {
	issues := []Issue{}
	location := directory + "/PLAN.md"
	status := stringValue(front["plan_status"])
	if status != draft.StatusInProgress && status != draft.StatusDone {
		issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("plan_status %q is invalid; expected in-progress or done", status)})
	}
	if value, found := front["depends_on"]; found {
		dependencies, ok := stringList(value)
		if !ok {
			issues = append(issues, Issue{Location: location, Message: "depends_on must be a list of strings"})
		} else {
			seen := map[string]bool{}
			for _, raw := range dependencies {
				dependency, err := draft.ParseDependency(strings.TrimSpace(raw))
				if err != nil {
					issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("broken dependency %q: %v", raw, err)})
					continue
				}
				canonical := draft.DependencyLabel(dependency)
				if seen[canonical] {
					issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("duplicate dependency %q", raw)})
				}
				seen[canonical] = true
				if dependency.Plan == draft.PlanName(directory) {
					issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("plan cannot depend on itself: %s", draft.DependencyLabel(dependency))})
				}
			}
		}
	}
	for _, key := range []string{"registered_at", "completed_at"} {
		if value, found := front[key]; found {
			stamp, ok := value.(string)
			if !ok {
				issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("%s must be an RFC3339 string", key)})
			} else if _, err := time.Parse(time.RFC3339, stamp); err != nil {
				issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("%s is not valid RFC3339: %v", key, err)})
			}
		}
	}
	return issues
}

func validatePhase(directory string, phase Phase) []Issue {
	location := directory + "/phases/" + filepath.Base(phase.path)
	issues := []Issue{}
	if !draft.ValidStatus(phase.Status) {
		issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("status %q is invalid", phase.Status)})
	}
	if err := plan.ValidateStatusChange(phase.front, phase.Status); err != nil {
		issues = append(issues, Issue{Location: location, Message: err.Error()})
	}
	if value, found := phase.front["depends_on"]; found {
		dependencies, ok := stringList(value)
		if !ok {
			issues = append(issues, Issue{Location: location, Message: "depends_on must be a list of strings"})
		} else {
			for _, raw := range dependencies {
				dependency, err := draft.ParseDependency(strings.TrimSpace(raw))
				if err != nil {
					issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("broken dependency %q: %v", raw, err)})
					continue
				}
				if dependency.Plan != draft.PlanName(directory) || dependency.Phase == nil {
					issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("phase dependency %q must reference a phase in %s", raw, draft.PlanName(directory))})
				}
			}
		}
	}
	if value, found := phase.front["completed_at"]; found {
		stamp, ok := value.(string)
		if !ok {
			issues = append(issues, Issue{Location: location, Message: "completed_at must be an RFC3339 string"})
		} else if _, err := time.Parse(time.RFC3339, stamp); err != nil {
			issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("completed_at is not valid RFC3339: %v", err)})
		}
	}
	if phase.Title == "unnamed phase" {
		issues = append(issues, Issue{Location: location, Message: "missing Markdown phase title"})
	}
	return issues
}

func parseChecklist(body, directory string) ([]checklistEntry, int, []Issue) {
	start, end, found := plan.ChecklistBounds(body)
	if !found {
		return nil, -1, []Issue{{Location: directory + "/PLAN.md", Message: "missing # Phases section"}}
	}
	issues := []Issue{}
	entries := []checklistEntry{}
	seen := map[int]bool{}
	section := body[start:end]
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.Contains(trimmed, "[Phase") {
			continue
		}
		match := checklistPattern.FindStringSubmatch(trimmed)
		if len(match) != 5 {
			issues = append(issues, Issue{Location: directory + "/PLAN.md", Message: fmt.Sprintf("malformed phase checklist entry: %s", trimmed)})
			continue
		}
		id, err := strconv.Atoi(match[2])
		if err != nil {
			issues = append(issues, Issue{Location: directory + "/PLAN.md", Message: fmt.Sprintf("invalid checklist phase number %q", match[2])})
			continue
		}
		if strings.TrimSpace(match[3]) == "" {
			issues = append(issues, Issue{Location: directory + "/PLAN.md", Message: fmt.Sprintf("checklist phase %02d has an empty title", id)})
		}
		if seen[id] {
			issues = append(issues, Issue{Location: directory + "/PLAN.md", Message: fmt.Sprintf("duplicate checklist entry for phase %02d", id)})
			continue
		}
		seen[id] = true
		entries = append(entries, checklistEntry{
			phaseNumber: id,
			title:       strings.TrimSpace(match[3]),
			path:        filepath.ToSlash(strings.TrimSpace(match[4])),
			checked:     match[1] != " ",
		})
	}
	return entries, start, issues
}

func compareChecklist(inspection Inspection) []Issue {
	issues := []Issue{}
	location := inspection.Directory + "/PLAN.md"
	byID := map[int]checklistEntry{}
	for _, entry := range inspection.Checklist {
		byID[entry.phaseNumber] = entry
	}
	phaseIDs := map[int]bool{}
	for _, phase := range inspection.Phases {
		phaseIDs[phase.ID] = true
		entry, found := byID[phase.ID]
		if !found {
			issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("phase file %02d-%s.md has no checklist entry", phase.ID, phase.Slug)})
			continue
		}
		expectedPath := filepath.ToSlash(plan.PhaseDocumentPath(phase.ID, phase.Slug))
		if entry.path != expectedPath {
			issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("checklist phase %02d links to %q; expected %q", phase.ID, entry.path, expectedPath)})
		}
		if entry.title != phase.Title {
			issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("checklist phase %02d title %q does not match phase file title %q", phase.ID, entry.title, phase.Title)})
		}
		expectedChecked := phase.Status == draft.StatusDone
		if entry.checked != expectedChecked {
			issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("checklist phase %02d is %s but phase file status is %q", phase.ID, checklistState(entry.checked), phase.Status)})
		}
	}
	for _, entry := range inspection.Checklist {
		if !phaseIDs[entry.phaseNumber] {
			issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("checklist entry for phase %02d has no phase file", entry.phaseNumber)})
		}
	}
	return issues
}

func RepairChecklist(body string, phases []Phase) (string, error) {
	start, end, found := plan.ChecklistBounds(body)
	if !found {
		return "", fmt.Errorf("PLAN.md does not contain a # Phases section")
	}
	sortedPhases := append([]Phase{}, phases...)
	sort.Slice(sortedPhases, func(i, j int) bool { return sortedPhases[i].ID < sortedPhases[j].ID })
	entries := make([]string, 0, len(sortedPhases))
	for _, phase := range sortedPhases {
		entries = append(entries, plan.ChecklistEntry(phase.ID, phase.Title, phase.Slug, phase.Status == draft.StatusDone))
	}
	replacement := "\n"
	if len(entries) > 0 {
		replacement += strings.Join(entries, "\n") + "\n"
	}
	if end < len(body) {
		replacement += "\n"
	}
	return body[:start] + replacement + body[end:], nil
}

func WritePlanBody(inspection Inspection, body string) error {
	bodyOffset := len(inspection.raw) - len(inspection.Body)
	if bodyOffset < 0 || bodyOffset > len(inspection.raw) {
		return fmt.Errorf("could not locate PLAN.md body")
	}
	return mdoc.WriteAtomically(inspection.planPath, inspection.raw[:bodyOffset]+body)
}

func checklistState(checked bool) string {
	if checked {
		return "checked"
	}
	return "unchecked"
}

func CheckPlanDependencies(inspection Inspection, byName map[string]Inspection) []Issue {
	issues := []Issue{}
	location := inspection.Directory + "/PLAN.md"
	if inspection.Front != nil {
		if value, found := inspection.Front["depends_on"]; found {
			if dependencies, ok := stringList(value); ok {
				for _, raw := range dependencies {
					dependency, err := draft.ParseDependency(strings.TrimSpace(raw))
					if err != nil {
						continue
					}
					target, found := byName[dependency.Plan]
					if !found {
						issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("broken dependency %q: plan is not registered", raw)})
						continue
					}
					if dependency.Phase != nil && !hasPhase(target, *dependency.Phase) {
						issues = append(issues, Issue{Location: location, Message: fmt.Sprintf("broken dependency %q: phase %02d is not present in %s", raw, *dependency.Phase, target.Directory)})
					}
				}
			}
		}
	}

	if !inspection.PhaseDataOK {
		return issues
	}
	drafts := make([]draft.Phase, 0, len(inspection.Phases))
	for _, phase := range inspection.Phases {
		meta := draft.Meta{Phase: phase.ID, Slug: phase.Slug, Status: phase.Status}
		for _, raw := range phase.Dependencies {
			dependency, err := draft.ParseDependency(strings.TrimSpace(raw))
			if err != nil || dependency.Phase == nil || dependency.Plan != draft.PlanName(inspection.Directory) {
				continue
			}
			meta.DependsOn = append(meta.DependsOn, *dependency.Phase)
		}
		drafts = append(drafts, draft.Phase{Title: phase.Title, Meta: meta})
	}
	if err := draft.ValidatePhaseDependencies(drafts); err != nil {
		issues = append(issues, Issue{Location: inspection.Directory, Message: "broken phase dependency graph: " + err.Error()})
	}
	return issues
}

func hasPhase(inspection Inspection, id int) bool {
	for _, phase := range inspection.Phases {
		if phase.ID == id {
			return true
		}
	}
	return false
}

func stringValue(value any) string {
	return strings.TrimSpace(mdoc.StringValue(value))
}

// stringList is the strict variant of mdoc.Strings: a non-list value or a
// list with any non-string element is rejected instead of silently skipped.
func stringList(value any) ([]string, bool) {
	values, ok := value.([]any)
	if !ok {
		if typed, isStrings := value.([]string); isStrings {
			return append([]string{}, typed...), true
		}
		return nil, false
	}
	result := mdoc.Strings(value)
	if len(result) != len(values) {
		return nil, false
	}
	return result, true
}
