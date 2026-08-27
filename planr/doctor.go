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

	"github.com/urfave/cli/v3"
)

type doctorIssue struct {
	location string
	message  string
}

type doctorReporter struct {
	issues int
}

func (r *doctorReporter) add(issue doctorIssue) {
	if issue.location == "" {
		fmt.Printf("FAIL: %s\n", issue.message)
	} else {
		fmt.Printf("FAIL %s: %s\n", issue.location, issue.message)
	}
	r.issues++
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
	location, err := discoverConfig(cwd)
	if err != nil {
		return err
	}
	reporter := &doctorReporter{}

	if err := ensureGitRepository(cwd); err != nil {
		reporter.add(doctorIssue{location: "git", message: err.Error()})
	} else {
		fmt.Printf("PASS git repository: %s\n", location.baseRoot)
	}

	settings := defaultConfig()
	if location.path == "" {
		fmt.Println("INFO config: .planr.yaml not found; using defaults")
	} else {
		parsed, parseErr := parseConfigFile(location.path)
		if parseErr != nil {
			reporter.add(doctorIssue{location: location.path, message: parseErr.Error()})
		} else {
			settings = parsed
			fmt.Printf("PASS config: %s\n", location.path)
		}
	}

	planDirectories := settings.planDirs(location.baseRoot)
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
			fmt.Printf("PASS plans_dirs: %s\n", plans)
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
			plan, issues := inspectDoctorPlan(planRoot, entry.Name())
			if cmd.Bool("fix") && len(plan.checklistIssues) > 0 && plan.planReadable && plan.planFrontOK && plan.phaseDataOK && plan.checklistStart >= 0 {
				repaired, repairErr := repairDoctorChecklist(plan.body, plan.phases)
				if repairErr != nil {
					issues = append(issues, doctorIssue{location: plan.directory + "/PLAN.md", message: fmt.Sprintf("cannot repair checklist: %v", repairErr)})
					issues = append(issues, plan.checklistIssues...)
				} else if writeErr := writeDoctorPlanBody(plan, repaired); writeErr != nil {
					issues = append(issues, doctorIssue{location: plan.directory + "/PLAN.md", message: fmt.Sprintf("cannot repair checklist: %v", writeErr)})
					issues = append(issues, plan.checklistIssues...)
				} else {
					fmt.Printf("FIXED %s/PLAN.md: synchronized checklist with phases\n", plan.directory)
				}
			} else {
				issues = append(issues, plan.checklistIssues...)
			}
			for _, issue := range issues {
				reporter.add(issue)
			}
			plans = append(plans, plan)
		}
	}

	byName := map[string]doctorPlan{}
	for _, plan := range plans {
		name := planName(plan.directory)
		if previous, found := byName[name]; found {
			reporter.add(doctorIssue{location: plan.directory, message: fmt.Sprintf("duplicate plan name %q also exists at %s", name, previous.directory)})
			continue
		}
		byName[name] = plan
	}
	for _, plan := range plans {
		checkDoctorPlanDependencies(reporter, plan, byName)
	}

	if reporter.issues == 0 {
		fmt.Println("Doctor found no problems")
		return nil
	}
	return fmt.Errorf("doctor found %d problem(s)", reporter.issues)
}

func inspectDoctorPlan(planRoot, directory string) (doctorPlan, []doctorIssue) {
	plan := doctorPlan{
		directory:   directory,
		root:        planRoot,
		planPath:    filepath.Join(planRoot, "PLAN.md"),
		phaseDataOK: true,
	}
	issues := []doctorIssue{}

	raw, err := os.ReadFile(plan.planPath)
	if err != nil {
		issues = append(issues, doctorIssue{location: plan.directory, message: fmt.Sprintf("cannot read PLAN.md: %v", err)})
		plan.phaseDataOK = false
	} else {
		if !hasDocumentFrontmatter(string(raw)) {
			issues = append(issues, doctorIssue{location: plan.directory + "/PLAN.md", message: "missing frontmatter"})
		} else {
			front, body, frontErr := frontmatter(string(raw))
			if frontErr != nil {
				issues = append(issues, doctorIssue{location: plan.directory + "/PLAN.md", message: frontErr.Error()})
			} else {
				plan.front, plan.raw, plan.body, plan.planReadable = front, string(raw), body, true
				frontIssues := validateDoctorPlanFrontmatter(directory, front)
				issues = append(issues, frontIssues...)
				plan.planFrontOK = len(frontIssues) == 0
				var checklistIssues []doctorIssue
				plan.checklist, plan.checklistStart, checklistIssues = parseDoctorChecklist(body, directory)
				plan.checklistIssues = append(plan.checklistIssues, checklistIssues...)
			}
		}
	}

	phaseDirectory := filepath.Join(planRoot, "phases")
	entries, readErr := os.ReadDir(phaseDirectory)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			issues = append(issues, doctorIssue{location: plan.directory + "/phases", message: "directory does not exist"})
		} else {
			issues = append(issues, doctorIssue{location: plan.directory + "/phases", message: fmt.Sprintf("cannot read directory: %v", readErr)})
		}
		plan.phaseDataOK = false
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
			location := plan.directory + "/phases/" + entry.Name()
			match := phaseFilePrefix.FindStringSubmatch(entry.Name())
			if len(match) != 3 || !kebab.MatchString(match[2]) {
				issues = append(issues, doctorIssue{location: location, message: "invalid phase filename; expected NN-lowercase-kebab-case.md"})
				plan.phaseDataOK = false
				continue
			}
			id, parseErr := strconv.Atoi(match[1])
			if parseErr != nil {
				issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("invalid phase number: %v", parseErr)})
				plan.phaseDataOK = false
				continue
			}
			if previous, found := ids[id]; found {
				issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("duplicate phase number %02d also used by %s", id, previous)})
				plan.phaseDataOK = false
				continue
			}
			if previous, found := slugs[match[2]]; found {
				issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("duplicate phase slug %q also used by %s", match[2], previous)})
				plan.phaseDataOK = false
				continue
			}
			ids[id] = entry.Name()
			slugs[match[2]] = entry.Name()

			phasePath := filepath.Join(phaseDirectory, entry.Name())
			phaseRaw, readPhaseErr := os.ReadFile(phasePath)
			if readPhaseErr != nil {
				issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("cannot read phase: %v", readPhaseErr)})
				plan.phaseDataOK = false
				continue
			}
			if !hasDocumentFrontmatter(string(phaseRaw)) {
				issues = append(issues, doctorIssue{location: location, message: "missing frontmatter"})
				plan.phaseDataOK = false
				continue
			}
			phaseFront, _, frontErr := frontmatter(string(phaseRaw))
			if frontErr != nil {
				issues = append(issues, doctorIssue{location: location, message: frontErr.Error()})
				plan.phaseDataOK = false
				continue
			}
			phase := doctorPhase{
				storedPhase: storedPhase{id: id, slug: match[2], title: markdownTitle(string(phaseRaw)), status: doctorStringValue(phaseFront["status"]), dependencies: yamlStrings(phaseFront["depends_on"])},
				path:        phasePath,
				front:       phaseFront,
			}
			phaseIssues := validateDoctorPhase(plan.directory, phase)
			if len(phaseIssues) > 0 {
				plan.phaseDataOK = false
			}
			issues = append(issues, phaseIssues...)
			plan.phases = append(plan.phases, phase)
		}
		sort.Slice(plan.phases, func(i, j int) bool { return plan.phases[i].id < plan.phases[j].id })
		if len(plan.phases) == 0 {
			issues = append(issues, doctorIssue{location: plan.directory + "/phases", message: "no phase documents found"})
			plan.phaseDataOK = false
		}
	}

	if plan.planReadable && plan.phaseDataOK {
		plan.checklistIssues = append(plan.checklistIssues, compareDoctorChecklist(plan)...)
		planStatus := doctorStringValue(plan.front["plan_status"])
		expected := "done"
		for _, phase := range plan.phases {
			if phase.status != "done" {
				expected = "in-progress"
				break
			}
		}
		if planStatus != "" && planStatus != expected {
			issues = append(issues, doctorIssue{location: plan.directory + "/PLAN.md", message: fmt.Sprintf("plan_status is %q but phase files indicate %q", planStatus, expected)})
		}
	}
	return plan, issues
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
				dependency, err := parseDependency(strings.TrimSpace(raw))
				if err != nil {
					issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("broken dependency %q: %v", raw, err)})
					continue
				}
				canonical := dependencyLabel(dependency)
				if seen[canonical] {
					issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("duplicate dependency %q", raw)})
				}
				seen[canonical] = true
				if dependency.plan == planName(directory) {
					issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("plan cannot depend on itself: %s", dependencyLabel(dependency))})
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
				dependency, err := parseDependency(strings.TrimSpace(raw))
				if err != nil {
					issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("broken dependency %q: %v", raw, err)})
					continue
				}
				if dependency.plan != planName(directory) || dependency.phase == nil {
					issues = append(issues, doctorIssue{location: location, message: fmt.Sprintf("phase dependency %q must reference a phase in %s", raw, planName(directory))})
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

func compareDoctorChecklist(plan doctorPlan) []doctorIssue {
	issues := []doctorIssue{}
	location := plan.directory + "/PLAN.md"
	byID := map[int]doctorChecklistEntry{}
	for _, entry := range plan.checklist {
		byID[entry.phaseNumber] = entry
	}
	phaseIDs := map[int]bool{}
	for _, phase := range plan.phases {
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
	for _, entry := range plan.checklist {
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

func writeDoctorPlanBody(plan doctorPlan, body string) error {
	bodyOffset := len(plan.raw) - len(plan.body)
	if bodyOffset < 0 || bodyOffset > len(plan.raw) {
		return fmt.Errorf("could not locate PLAN.md body")
	}
	return writeFileAtomically(plan.planPath, plan.raw[:bodyOffset]+body)
}

func checklistState(checked bool) string {
	if checked {
		return "checked"
	}
	return "unchecked"
}

func checkDoctorPlanDependencies(reporter *doctorReporter, plan doctorPlan, byName map[string]doctorPlan) {
	location := plan.directory + "/PLAN.md"
	if plan.front != nil {
		if value, found := plan.front["depends_on"]; found {
			if dependencies, ok := doctorStringList(value); ok {
				for _, raw := range dependencies {
					dependency, err := parseDependency(strings.TrimSpace(raw))
					if err != nil {
						continue
					}
					target, found := byName[dependency.plan]
					if !found {
						reporter.add(doctorIssue{location: location, message: fmt.Sprintf("broken dependency %q: plan is not registered", raw)})
						continue
					}
					if dependency.phase != nil && !doctorHasPhase(target, *dependency.phase) {
						reporter.add(doctorIssue{location: location, message: fmt.Sprintf("broken dependency %q: phase %02d is not present in %s", raw, *dependency.phase, target.directory)})
					}
				}
			}
		}
	}

	if !plan.phaseDataOK {
		return
	}
	drafts := make([]draftPhase, 0, len(plan.phases))
	for _, phase := range plan.phases {
		meta := phaseMeta{Phase: phase.id, Slug: phase.slug, Status: phase.status}
		for _, raw := range phase.dependencies {
			dependency, err := parseDependency(strings.TrimSpace(raw))
			if err != nil || dependency.phase == nil || dependency.plan != planName(plan.directory) {
				continue
			}
			meta.DependsOn = append(meta.DependsOn, *dependency.phase)
		}
		drafts = append(drafts, draftPhase{Title: phase.title, Meta: meta})
	}
	if err := validatePhaseDependencies(drafts); err != nil {
		reporter.add(doctorIssue{location: plan.directory, message: "broken phase dependency graph: " + err.Error()})
	}
}

func doctorHasPhase(plan doctorPlan, id int) bool {
	for _, phase := range plan.phases {
		if phase.id == id {
			return true
		}
	}
	return false
}

func doctorStringValue(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func doctorStringList(value any) ([]string, bool) {
	switch values := value.(type) {
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, false
			}
			result = append(result, text)
		}
		return result, true
	case []string:
		return append([]string{}, values...), true
	default:
		return nil, false
	}
}
