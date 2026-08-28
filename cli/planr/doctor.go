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

	"github.com/ironpark/toolz/cli/planr/internal/agentenv"
	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/gitrepo"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/urfave/cli/v3"
)

type doctorIssue struct {
	location string
	message  string
}

type doctorReporter struct {
	issues  int
	records []doctorIssue
	json    bool
}

func (r *doctorReporter) add(issue doctorIssue) {
	r.records = append(r.records, issue)
	if r.json {
		r.issues++
		return
	}
	if issue.location == "" {
		fmt.Printf("FAIL: %s\n", issue.message)
	} else {
		fmt.Printf("FAIL %s: %s\n", issue.location, issue.message)
	}
	r.issues++
}

func (r *doctorReporter) printf(format string, values ...any) {
	if !r.json {
		fmt.Printf(format, values...)
	}
}

type doctorPhase struct {
	storedPhase
	path  string
	front map[string]any
}

type doctorPlan struct {
	directory string
	root      string
	planPath  string
	front     map[string]any
	raw       string
	body      string

	phases          []doctorPhase
	checklist       []doctorChecklistEntry
	checklistIssues []doctorIssue
	checklistStart  int
	planReadable    bool
	planFrontOK     bool
	phaseDataOK     bool
}

type doctorChecklistEntry struct {
	phaseNumber int
	title       string
	path        string
	checked     bool
}

var doctorChecklistPattern = regexp.MustCompile(`^\s*-\s*\[([ xX])\]\s+\[Phase\s+([0-9]+):\s*(.*?)\]\(([^)]+)\)\s*$`)

func doctorCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 0 {
		return fmt.Errorf("doctor does not accept positional arguments")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	location, err := config.Discover(cwd)
	if err != nil {
		return err
	}
	reporter := &doctorReporter{json: cmd.Bool("json")}

	if err := gitrepo.EnsureRepository(cwd); err != nil {
		reporter.add(doctorIssue{location: "git", message: err.Error()})
	} else {
		reporter.printf("PASS git repository: %s\n", location.BaseRoot)
	}

	reporter.printf("INFO agent: %s\n", agentenv.CurrentDescription())

	settings := config.Default()
	if location.Path == "" {
		reporter.printf("INFO config: .planr.yaml not found; using defaults\n")
	} else {
		parsed, parseErr := config.ParseFile(location.Path)
		if parseErr != nil {
			reporter.add(doctorIssue{location: location.Path, message: parseErr.Error()})
		} else {
			settings = parsed
			reporter.printf("PASS config: %s\n", location.Path)
		}
	}

	planDirectories := settings.PlanDirs(location.BaseRoot)
	validDirectories := []string{}
	for _, plans := range planDirectories {
		info, statErr := os.Stat(plans)
		switch {
		case statErr != nil && os.IsNotExist(statErr):
			reporter.add(doctorIssue{location: "plans_dirs", message: fmt.Sprintf("directory does not exist: %s", plans)})
		case statErr != nil:
			reporter.add(doctorIssue{location: "plans_dirs", message: fmt.Sprintf("cannot inspect %s: %v", plans, statErr)})
		case !info.IsDir():
			reporter.add(doctorIssue{location: "plans_dirs", message: fmt.Sprintf("path is not a directory: %s", plans)})
		default:
			reporter.printf("PASS plans_dirs: %s\n", plans)
			validDirectories = append(validDirectories, plans)
		}
	}

	plans := []doctorPlan{}
	for _, plansRoot := range validDirectories {
		entries, readErr := os.ReadDir(plansRoot)
		if readErr != nil {
			reporter.add(doctorIssue{location: filepath.Base(plansRoot), message: fmt.Sprintf("cannot read plans directory: %v", readErr)})
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			planRoot := filepath.Join(plansRoot, entry.Name())
			record, issues := inspectDoctorPlan(planRoot, entry.Name())
			if cmd.Bool("fix") && len(record.checklistIssues) > 0 && record.planReadable && record.planFrontOK && record.phaseDataOK && record.checklistStart >= 0 {
				repaired, repairErr := repairDoctorChecklist(record.body, record.phases)
				if repairErr != nil {
					issues = append(issues, doctorIssue{location: record.directory + "/PLAN.md", message: fmt.Sprintf("cannot repair checklist: %v", repairErr)})
					issues = append(issues, record.checklistIssues...)
				} else if writeErr := writeDoctorPlanBody(record, repaired); writeErr != nil {
					issues = append(issues, doctorIssue{location: record.directory + "/PLAN.md", message: fmt.Sprintf("cannot repair checklist: %v", writeErr)})
					issues = append(issues, record.checklistIssues...)
				} else {
					reporter.printf("FIXED %s/PLAN.md: synchronized checklist with phases\n", record.directory)
				}
			} else {
				issues = append(issues, record.checklistIssues...)
			}
			for _, issue := range issues {
				reporter.add(issue)
			}
			plans = append(plans, record)
		}
	}

	byName := map[string]doctorPlan{}
	for _, record := range plans {
		name := plan.Name(record.directory)
		if previous, found := byName[name]; found {
			reporter.add(doctorIssue{location: record.directory, message: fmt.Sprintf("duplicate plan name %q also exists at %s", name, previous.directory)})
			continue
		}
		byName[name] = record
	}
	for _, record := range plans {
		checkDoctorPlanDependencies(reporter, record, byName)
	}

	if reporter.issues == 0 {
		if reporter.json {
			return writeJSON(makeDoctorJSON(reporter.records))
		}
		fmt.Println("Doctor found no problems")
		return nil
	}
	if reporter.json {
		if err := writeJSON(makeDoctorJSON(reporter.records)); err != nil {
			return err
		}
	}
	return fmt.Errorf("doctor found %d problem(s)", reporter.issues)
}

func inspectDoctorPlan(planRoot, directory string) (doctorPlan, []doctorIssue) {
	record := doctorPlan{
		directory:   directory,
		root:        planRoot,
		planPath:    filepath.Join(planRoot, "PLAN.md"),
		phaseDataOK: true,
	}
	issues := []doctorIssue{}

	raw, err := os.ReadFile(record.planPath)
	if err != nil {
		issues = append(issues, doctorIssue{location: record.directory, message: fmt.Sprintf("cannot read PLAN.md: %v", err)})
		record.phaseDataOK = false
	} else {
		if !hasDocumentFrontmatter(string(raw)) {
			issues = append(issues, doctorIssue{location: record.directory + "/PLAN.md", message: "missing mdoc.Split"})
		} else {
			front, body, frontErr := mdoc.Split(string(raw))
			if frontErr != nil {
				issues = append(issues, doctorIssue{location: record.directory + "/PLAN.md", message: frontErr.Error()})
			} else {
				record.front, record.raw, record.body, record.planReadable = front, string(raw), body, true
				frontIssues := validateDoctorPlanFrontmatter(directory, front)
				issues = append(issues, frontIssues...)
				record.planFrontOK = len(frontIssues) == 0
				var checklistIssues []doctorIssue
				record.checklist, record.checklistStart, checklistIssues = parseDoctorChecklist(body, directory)
				record.checklistIssues = append(record.checklistIssues, checklistIssues...)
			}
		}
	}

	phaseDirectory := filepath.Join(planRoot, "phases")
	entries, readErr := os.ReadDir(phaseDirectory)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			issues = append(issues, doctorIssue{location: record.directory + "/phases", message: "directory does not exist"})
		} else {
			issues = append(issues, doctorIssue{location: record.directory + "/phases", message: fmt.Sprintf("cannot read directory: %v", readErr)})
		}
		record.phaseDataOK = false
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
			location := record.directory + "/phases/" + entry.Name()
			match := phaseFilePrefix.FindStringSubmatch(entry.Name())
			if len(match) != 3 || !plan.KebabPattern.MatchString(match[2]) {
				issues = append(issues, doctorIssue{location: location, message: "invalid phase filename; expected NN-lowercase-plan.KebabPattern-case.md"})
				record.phaseDataOK = false
				continue
			}
			id, parseErr := strconv.Atoi(match[1])
			if parseErr != nil {
				issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("invalid phase number: %v", parseErr)})
				record.phaseDataOK = false
				continue
			}
			if previous, found := ids[id]; found {
				issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("duplicate phase number %02d also used by %s", id, previous)})
				record.phaseDataOK = false
				continue
			}
			if previous, found := slugs[match[2]]; found {
				issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("duplicate phase slug %q also used by %s", match[2], previous)})
				record.phaseDataOK = false
				continue
			}
			ids[id] = entry.Name()
			slugs[match[2]] = entry.Name()

			phasePath := filepath.Join(phaseDirectory, entry.Name())
			phaseRaw, readPhaseErr := os.ReadFile(phasePath)
			if readPhaseErr != nil {
				issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("cannot read phase: %v", readPhaseErr)})
				record.phaseDataOK = false
				continue
			}
			if !hasDocumentFrontmatter(string(phaseRaw)) {
				issues = append(issues, doctorIssue{location: location, message: "missing mdoc.Split"})
				record.phaseDataOK = false
				continue
			}
			phaseFront, _, frontErr := mdoc.Split(string(phaseRaw))
			if frontErr != nil {
				issues = append(issues, doctorIssue{location: location, message: frontErr.Error()})
				record.phaseDataOK = false
				continue
			}
			phase := doctorPhase{
				storedPhase: storedPhase{id: id, slug: match[2], title: mdoc.Title(string(phaseRaw)), status: doctorStringValue(phaseFront["status"]), dependencies: mdoc.Strings(phaseFront["depends_on"])},
				path:        phasePath,
				front:       phaseFront,
			}
			phaseIssues := validateDoctorPhase(record.directory, phase)
			if len(phaseIssues) > 0 {
				record.phaseDataOK = false
			}
			issues = append(issues, phaseIssues...)
			record.phases = append(record.phases, phase)
		}
		sort.Slice(record.phases, func(i, j int) bool { return record.phases[i].id < record.phases[j].id })
		if len(record.phases) == 0 {
			issues = append(issues, doctorIssue{location: record.directory + "/phases", message: "no phase documents found"})
			record.phaseDataOK = false
		}
	}

	if record.planReadable && record.phaseDataOK {
		record.checklistIssues = append(record.checklistIssues, compareDoctorChecklist(record)...)
		planStatus := doctorStringValue(record.front["plan_status"])
		expected := "done"
		for _, phase := range record.phases {
			if phase.status != "done" {
				expected = "in-progress"
				break
			}
		}
		if planStatus != "" && planStatus != expected {
			issues = append(issues, doctorIssue{location: record.directory + "/PLAN.md", message: fmt.Sprintf("plan_status is %q but phase files indicate %q", planStatus, expected)})
		}
	}
	return record, issues
}

func hasDocumentFrontmatter(raw string) bool {
	return strings.HasPrefix(raw, "---\n")
}

func validateDoctorPlanFrontmatter(directory string, front map[string]any) []doctorIssue {
	issues := []doctorIssue{}
	location := directory + "/PLAN.md"
	status := doctorStringValue(front["plan_status"])
	if status != "in-progress" && status != "done" {
		issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("plan_status %q is invalid; expected in-progress or done", status)})
	}
	if value, found := front["depends_on"]; found {
		dependencies, ok := doctorStringList(value)
		if !ok {
			issues = append(issues, doctorIssue{location: location, message: "depends_on must be a list of strings"})
		} else {
			seen := map[string]bool{}
			for _, raw := range dependencies {
				dependency, err := plan.ParseDependency(strings.TrimSpace(raw))
				if err != nil {
					issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("broken dependency %q: %v", raw, err)})
					continue
				}
				canonical := plan.DependencyLabel(dependency)
				if seen[canonical] {
					issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("duplicate dependency %q", raw)})
				}
				seen[canonical] = true
				if dependency.Plan == plan.Name(directory) {
					issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("plan cannot depend on itself: %s", plan.DependencyLabel(dependency))})
				}
			}
		}
	}
	for _, key := range []string{"registered_at", "completed_at"} {
		if value, found := front[key]; found {
			stamp, ok := value.(string)
			if !ok {
				issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("%s must be an RFC3339 string", key)})
			} else if _, err := time.Parse(time.RFC3339, stamp); err != nil {
				issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("%s is not valid RFC3339: %v", key, err)})
			}
		}
	}
	return issues
}

func validateDoctorPhase(directory string, phase doctorPhase) []doctorIssue {
	location := directory + "/phases/" + filepath.Base(phase.path)
	issues := []doctorIssue{}
	if !phaseStatusValues[phase.status] {
		issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("status %q is invalid", phase.status)})
	}
	if err := validatePhaseStatusChange(phase.front, phase.status); err != nil {
		issues = append(issues, doctorIssue{location: location, message: err.Error()})
	}
	if value, found := phase.front["depends_on"]; found {
		dependencies, ok := doctorStringList(value)
		if !ok {
			issues = append(issues, doctorIssue{location: location, message: "depends_on must be a list of strings"})
		} else {
			for _, raw := range dependencies {
				dependency, err := plan.ParseDependency(strings.TrimSpace(raw))
				if err != nil {
					issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("broken dependency %q: %v", raw, err)})
					continue
				}
				if dependency.Plan != plan.Name(directory) || dependency.Phase == nil {
					issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("phase dependency %q must reference a phase in %s", raw, plan.Name(directory))})
				}
			}
		}
	}
	if value, found := phase.front["completed_at"]; found {
		stamp, ok := value.(string)
		if !ok {
			issues = append(issues, doctorIssue{location: location, message: "completed_at must be an RFC3339 string"})
		} else if _, err := time.Parse(time.RFC3339, stamp); err != nil {
			issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("completed_at is not valid RFC3339: %v", err)})
		}
	}
	if phase.title == "unnamed phase" {
		issues = append(issues, doctorIssue{location: location, message: "missing Markdown phase title"})
	}
	return issues
}

func parseDoctorChecklist(body, directory string) ([]doctorChecklistEntry, int, []doctorIssue) {
	start, end, found := doctorChecklistBounds(body)
	if !found {
		return nil, -1, []doctorIssue{{location: directory + "/PLAN.md", message: "missing # Phases section"}}
	}
	issues := []doctorIssue{}
	entries := []doctorChecklistEntry{}
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
		match := doctorChecklistPattern.FindStringSubmatch(trimmed)
		if len(match) != 5 {
			issues = append(issues, doctorIssue{location: directory + "/PLAN.md", message: fmt.Sprintf("malformed phase checklist entry: %s", trimmed)})
			continue
		}
		id, err := strconv.Atoi(match[2])
		if err != nil {
			issues = append(issues, doctorIssue{location: directory + "/PLAN.md", message: fmt.Sprintf("invalid checklist phase number %q", match[2])})
			continue
		}
		if strings.TrimSpace(match[3]) == "" {
			issues = append(issues, doctorIssue{location: directory + "/PLAN.md", message: fmt.Sprintf("checklist phase %02d has an empty title", id)})
		}
		if seen[id] {
			issues = append(issues, doctorIssue{location: directory + "/PLAN.md", message: fmt.Sprintf("duplicate checklist entry for phase %02d", id)})
			continue
		}
		seen[id] = true
		entries = append(entries, doctorChecklistEntry{
			phaseNumber: id,
			title:       strings.TrimSpace(match[3]),
			path:        filepath.ToSlash(strings.TrimSpace(match[4])),
			checked:     match[1] != " ",
		})
	}
	return entries, start, issues
}

func doctorChecklistBounds(body string) (int, int, bool) {
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

func compareDoctorChecklist(record doctorPlan) []doctorIssue {
	issues := []doctorIssue{}
	location := record.directory + "/PLAN.md"
	byID := map[int]doctorChecklistEntry{}
	for _, entry := range record.checklist {
		byID[entry.phaseNumber] = entry
	}
	phaseIDs := map[int]bool{}
	for _, phase := range record.phases {
		phaseIDs[phase.id] = true
		entry, found := byID[phase.id]
		if !found {
			issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("phase file %02d-%s.md has no checklist entry", phase.id, phase.slug)})
			continue
		}
		expectedPath := filepath.ToSlash(phaseDocumentPath(phase.id, phase.slug))
		if entry.path != expectedPath {
			issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("checklist phase %02d links to %q; expected %q", phase.id, entry.path, expectedPath)})
		}
		if entry.title != phase.title {
			issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("checklist phase %02d title %q does not match phase file title %q", phase.id, entry.title, phase.title)})
		}
		expectedChecked := phase.status == "done"
		if entry.checked != expectedChecked {
			issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("checklist phase %02d is %s but phase file status is %q", phase.id, checklistState(entry.checked), phase.status)})
		}
	}
	for _, entry := range record.checklist {
		if !phaseIDs[entry.phaseNumber] {
			issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("checklist entry for phase %02d has no phase file", entry.phaseNumber)})
		}
	}
	return issues
}

func repairDoctorChecklist(body string, phases []doctorPhase) (string, error) {
	start, end, found := doctorChecklistBounds(body)
	if !found {
		return "", fmt.Errorf("PLAN.md does not contain a # Phases section")
	}
	sortedPhases := append([]doctorPhase{}, phases...)
	sort.Slice(sortedPhases, func(i, j int) bool { return sortedPhases[i].id < sortedPhases[j].id })
	entries := make([]string, 0, len(sortedPhases))
	for _, phase := range sortedPhases {
		entries = append(entries, phaseChecklistEntry(phase.id, phase.title, phase.slug, phase.status == "done"))
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

func writeDoctorPlanBody(record doctorPlan, body string) error {
	bodyOffset := len(record.raw) - len(record.body)
	if bodyOffset < 0 || bodyOffset > len(record.raw) {
		return fmt.Errorf("could not locate PLAN.md body")
	}
	return mdoc.WriteAtomically(record.planPath, record.raw[:bodyOffset]+body)
}

func checklistState(checked bool) string {
	if checked {
		return "checked"
	}
	return "unchecked"
}

func checkDoctorPlanDependencies(reporter *doctorReporter, record doctorPlan, byName map[string]doctorPlan) {
	location := record.directory + "/PLAN.md"
	if record.front != nil {
		if value, found := record.front["depends_on"]; found {
			if dependencies, ok := doctorStringList(value); ok {
				for _, raw := range dependencies {
					dependency, err := plan.ParseDependency(strings.TrimSpace(raw))
					if err != nil {
						continue
					}
					target, found := byName[dependency.Plan]
					if !found {
						reporter.add(doctorIssue{location: location, message: fmt.Sprintf("broken dependency %q: plan is not registered", raw)})
						continue
					}
					if dependency.Phase != nil && !doctorHasPhase(target, *dependency.Phase) {
						reporter.add(doctorIssue{location: location, message: fmt.Sprintf("broken dependency %q: phase %02d is not present in %s", raw, *dependency.Phase, target.directory)})
					}
				}
			}
		}
	}

	if !record.phaseDataOK {
		return
	}
	drafts := make([]draft.Phase, 0, len(record.phases))
	for _, phase := range record.phases {
		meta := draft.Meta{Phase: phase.id, Slug: phase.slug, Status: phase.status}
		for _, raw := range phase.dependencies {
			dependency, err := plan.ParseDependency(strings.TrimSpace(raw))
			if err != nil || dependency.Phase == nil || dependency.Plan != plan.Name(record.directory) {
				continue
			}
			meta.DependsOn = append(meta.DependsOn, *dependency.Phase)
		}
		drafts = append(drafts, draft.Phase{Title: phase.title, Meta: meta})
	}
	if err := draft.ValidatePhaseDependencies(drafts); err != nil {
		reporter.add(doctorIssue{location: record.directory, message: "broken phase dependency graph: " + err.Error()})
	}
}

func doctorHasPhase(record doctorPlan, id int) bool {
	for _, phase := range record.phases {
		if phase.id == id {
			return true
		}
	}
	return false
}

func doctorStringValue(value any) string {
	return strings.TrimSpace(mdoc.StringValue(value))
}

// doctorStringList is the strict variant of mdoc.Strings: a non-list value or a
// list with any non-string element is rejected instead of silently skipped.
func doctorStringList(value any) ([]string, bool) {
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
