package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/urfave/cli/v3"
)

var phaseStatusValues = map[string]bool{
	"planned":     true,
	"conditional": true,
	"in-progress": true,
	"done":        true,
}

func phaseSetCommand(_ context.Context, cmd *cli.Command) error {
	return phaseCommand(cmd, strings.TrimSpace(cmd.String("status")))
}

func phaseShortcutCommand(status string) func(context.Context, *cli.Command) error {
	return func(_ context.Context, cmd *cli.Command) error {
		return phaseCommand(cmd, status)
	}
}

func phaseCommand(cmd *cli.Command, status string) error {
	if cmd.NArg() != 2 {
		return fmt.Errorf("phase command requires <plan-name> <phase-number>")
	}
	if !phaseStatusValues[status] {
		return fmt.Errorf("invalid phase status %q; use planned, conditional, in-progress, or done", status)
	}
	phaseID, err := strconv.Atoi(cmd.Args().Get(1))
	if err != nil || phaseID < 0 {
		return fmt.Errorf("phase number %q must be a non-negative integer", cmd.Args().Get(1))
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
	planRoot, planDirectory, err := findPlanDirectory(planDirectories, cmd.Args().First())
	if err != nil {
		return err
	}
	planLock, err := acquirePlanLock(planRoot)
	if err != nil {
		return err
	}
	defer planLock.close()
	event := phaseHookEvent(status)
	willComplete := false
	planWasDone := false
	if status == "done" {
		planWasDone, err = planAlreadyDone(planRoot)
		if err != nil {
			return err
		}
	}
	if status == "done" && len(settings.Hooks.commands("before", hookEventPlanDone)) > 0 {
		willComplete, err = phaseWillComplete(planRoot, phaseID, status)
		if err != nil {
			return err
		}
		willComplete = willComplete && !planWasDone
	}
	if !cmd.Bool("force") {
		// Starting or completing a phase out of order silently invalidates the
		// ordering the plan was validated against, so the same graph `apply`
		// checked is enforced here too.
		if err := ensureDependenciesMet(planDirectories, planRoot, planDirectory, phaseID, status); err != nil {
			return err
		}
		if status == "done" {
			if err := ensureCleanSource(repoRoot, planDirectories, settings.Ignore); err != nil {
				return err
			}
		}
	}
	if err := runConfiguredHooks(repoRoot, settings, "before", event, planDirectory, phaseID, status); err != nil {
		return err
	}
	if willComplete {
		if err := runConfiguredHooks(repoRoot, settings, "before", hookEventPlanDone, planDirectory, -1, "done"); err != nil {
			return err
		}
	}
	var completed bool
	planDirectory, completed, err = updatePhaseStatusLocked(planRoot, planDirectory, phaseID, status)
	if err != nil {
		return err
	}
	fmt.Printf("Updated %s phase %02d: %s\n", planDirectory, phaseID, status)
	// Link the completion to the commit it landed on, for `planr notes`.
	if status == "in-progress" {
		if err := recordCompletionNote(repoRoot, planDirectory, hookEventStart, phaseID); err != nil {
			warnStartNoteFailure(err)
		}
	}
	if status == "done" {
		if err := recordCompletionNote(repoRoot, planDirectory, hookEventDone, phaseID); err != nil {
			warnNoteFailure(err)
		}
	}
	if completed {
		fmt.Printf("Plan %s marked done\n", planDirectory)
		if !planWasDone {
			if err := recordCompletionNote(repoRoot, planDirectory, hookEventPlanDone, -1); err != nil {
				warnNoteFailure(err)
			}
		}
	}
	if err := runConfiguredHooks(repoRoot, settings, "after", event, planDirectory, phaseID, status); err != nil {
		return err
	}
	if completed && status == "done" && !planWasDone {
		if err := runConfiguredHooks(repoRoot, settings, "after", hookEventPlanDone, planDirectory, -1, "done"); err != nil {
			return err
		}
	}
	return nil
}

// ensureDependenciesMet refuses to advance a phase whose prerequisites are not
// done. It covers both the phase's own depends_on and the plan-level
// depends_on in PLAN.md, which is what `status` reports as `wait`. Resetting a
// phase to planned or conditional moves backwards and is never blocked.
func ensureDependenciesMet(planDirectories []string, planRoot, planDirectory string, phaseID int, status string) error {
	if status != "in-progress" && status != "done" {
		return nil
	}
	phases, err := readPlanPhases(planRoot)
	if err != nil {
		return err
	}
	local := map[int]storedPhase{}
	var target *storedPhase
	for index, phase := range phases {
		local[phase.id] = phase
		if phase.id == phaseID {
			target = &phases[index]
		}
	}
	// A missing phase is reported by updatePhaseStatus with a better message.
	if target == nil {
		return nil
	}

	unmet := []string{}
	for _, raw := range target.dependencies {
		dependency, parseErr := parseDependency(raw)
		if parseErr != nil {
			unmet = append(unmet, fmt.Sprintf("%s (unreadable dependency)", raw))
			continue
		}
		if dependency.plan == planName(planDirectory) && dependency.phase != nil {
			phase, found := local[*dependency.phase]
			switch {
			case !found:
				unmet = append(unmet, fmt.Sprintf("phase %02d (not found)", *dependency.phase))
			case phase.status != "done":
				unmet = append(unmet, fmt.Sprintf("phase %02d %q (%s)", phase.id, phase.title, phase.status))
			}
			continue
		}
		if reason := unmetPlanDependency(planDirectories, dependency); reason != "" {
			unmet = append(unmet, reason)
		}
	}
	planDependencies, err := planLevelDependencies(planRoot)
	if err != nil {
		return err
	}
	for _, dependency := range planDependencies {
		if reason := unmetPlanDependency(planDirectories, dependency); reason != "" {
			unmet = append(unmet, reason)
		}
	}
	if len(unmet) == 0 {
		return nil
	}
	lines := make([]string, len(unmet))
	for index, reason := range unmet {
		lines[index] = "  - " + reason
	}
	return fmt.Errorf("cannot set %s phase %02d to %s while its dependencies are unfinished:\n%s\nfinish them first or use --force",
		planDirectory, phaseID, status, strings.Join(lines, "\n"))
}

// unmetPlanDependency describes why a dependency on another plan is not
// satisfied, or returns an empty string when it is. A dependency naming a plan
// that was never registered counts as unmet: drafts may reference plans that do
// not exist yet, but work cannot proceed past one.
func unmetPlanDependency(planDirectories []string, dependency planDependency) string {
	label := dependencyLabel(dependency)
	planRoot, _, err := findPlanDirectory(planDirectories, dependency.plan)
	if err != nil {
		return fmt.Sprintf("%s (not registered)", label)
	}
	if dependency.phase == nil {
		done, err := planAlreadyDone(planRoot)
		if err != nil {
			return fmt.Sprintf("%s (unreadable)", label)
		}
		if !done {
			return fmt.Sprintf("%s (in-progress)", label)
		}
		return ""
	}
	phases, err := readPlanPhases(planRoot)
	if err != nil {
		return fmt.Sprintf("%s (unreadable)", label)
	}
	for _, phase := range phases {
		if phase.id != *dependency.phase {
			continue
		}
		if phase.status != "done" {
			return fmt.Sprintf("%s (%s)", label, phase.status)
		}
		return ""
	}
	return fmt.Sprintf("%s (phase not found)", label)
}

// planLevelDependencies reads the depends_on list from a plan's PLAN.md.
func planLevelDependencies(planRoot string) ([]planDependency, error) {
	raw, err := os.ReadFile(filepath.Join(planRoot, "PLAN.md"))
	if err != nil {
		return nil, err
	}
	front, _, err := frontmatter(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse PLAN.md: %w", err)
	}
	dependencies := []planDependency{}
	for _, raw := range yamlStrings(front["depends_on"]) {
		dependency, err := parseDependency(raw)
		if err != nil {
			continue
		}
		dependencies = append(dependencies, dependency)
	}
	return dependencies, nil
}

func ensureCleanSource(repoRoot string, planDirectories, ignore []string) error {
	paths, err := uncommittedSourcePaths(repoRoot, planDirectories, ignore)
	if err != nil {
		return fmt.Errorf("cannot check uncommitted source changes: %w; use --force to bypass this check", err)
	}
	if len(paths) == 0 {
		return nil
	}
	lines := make([]string, len(paths))
	for index, path := range paths {
		lines[index] = "  - " + path
	}
	return fmt.Errorf("cannot mark phase done while source changes are uncommitted:\n%s\ncommit the source changes first or use --force", strings.Join(lines, "\n"))
}

func uncommittedSourcePaths(repoRoot string, planDirectories, ignore []string) ([]string, error) {
	repository, err := git.PlainOpen(repoRoot)
	if err != nil {
		return nil, err
	}
	worktree, err := repository.Worktree()
	if err != nil {
		return nil, err
	}
	status, err := worktree.Status()
	if err != nil {
		return nil, err
	}
	paths := []string{}
	for path, fileStatus := range status {
		if fileStatus == nil || (fileStatus.Staging == git.Unmodified && fileStatus.Worktree == git.Unmodified) {
			continue
		}
		if !isGeneratedPlanPath(repoRoot, planDirectories, path) && !isPlanDraftPath(repoRoot, path) && !isIgnoredPath(path, ignore) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// isPlanDraftPath reports whether a dirty file is planr's own scratch output.
// Drafts and checkouts survive their apply command, so without this they show
// up as "uncommitted source changes" and force the author to commit planr's
// own scratch output or reach for --force.
func isPlanDraftPath(repoRoot, relativePath string) bool {
	clean := filepath.ToSlash(filepath.Clean(relativePath))
	if clean == ".planr" || strings.HasPrefix(clean, ".planr/") {
		return true
	}
	if !strings.EqualFold(filepath.Ext(relativePath), ".md") {
		return false
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
	if err != nil {
		return false
	}
	front, _, err := frontmatter(string(raw))
	if err != nil {
		return false
	}
	if name, ok := front["plan_name"].(string); ok && name != "" {
		return true
	}
	if value, ok := front["planr_new"].(string); ok && value == "phase" {
		return true
	}
	_, ok := front["planr_edit"]
	return ok
}

func isIgnoredPath(relativePath string, patterns []string) bool {
	path := filepath.ToSlash(filepath.Clean(relativePath))
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		pattern = strings.TrimPrefix(pattern, "./")
		if pattern == "" {
			continue
		}
		if globPathMatch(pattern, path) {
			return true
		}
		if !strings.ContainsAny(pattern, "*?") && (path == pattern || strings.HasPrefix(path, strings.TrimSuffix(pattern, "/")+"/")) {
			return true
		}
	}
	return false
}

func globPathMatch(pattern, value string) bool {
	pattern = filepath.ToSlash(pattern)
	var expression strings.Builder
	expression.WriteString("^")
	for index := 0; index < len(pattern); index++ {
		switch pattern[index] {
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				expression.WriteString(".*")
				index++
			} else {
				expression.WriteString("[^/]*")
			}
		case '?':
			expression.WriteString("[^/]")
		default:
			expression.WriteString(regexp.QuoteMeta(string(pattern[index])))
		}
	}
	expression.WriteString("$")
	matched, err := regexp.MatchString(expression.String(), value)
	return err == nil && matched
}

func isGeneratedPlanPath(repoRoot string, planDirectories []string, relativePath string) bool {
	absPath := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	if filepath.Clean(absPath) == filepath.Join(repoRoot, ".planr.yaml") {
		return true
	}
	for _, planDirectory := range planDirectories {
		relative, err := filepath.Rel(planDirectory, absPath)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relative) {
			return true
		}
	}
	return false
}

func phaseHookEvent(status string) string {
	switch status {
	case "planned":
		return hookEventReset
	case "conditional":
		return hookEventConditional
	case "in-progress":
		return hookEventStart
	case "done":
		return hookEventDone
	default:
		return ""
	}
}

func phaseWillComplete(planRoot string, phaseID int, status string) (bool, error) {
	if status != "done" {
		return false, nil
	}
	phases, err := readPlanPhases(planRoot)
	if err != nil {
		return false, err
	}
	if len(phases) == 0 {
		return false, nil
	}
	found := false
	for _, phase := range phases {
		if phase.id == phaseID {
			found = true
			continue
		}
		if phase.status != "done" {
			return false, nil
		}
	}
	if !found {
		return false, fmt.Errorf("phase %02d not found", phaseID)
	}
	return true, nil
}

func planAlreadyDone(planRoot string) (bool, error) {
	raw, err := os.ReadFile(filepath.Join(planRoot, "PLAN.md"))
	if err != nil {
		return false, err
	}
	front, _, err := frontmatter(string(raw))
	if err != nil {
		return false, fmt.Errorf("parse PLAN.md: %w", err)
	}
	status, _ := front["plan_status"].(string)
	return status == "done", nil
}

func updatePhaseStatus(planDirectories []string, planArg string, phaseID int, status string) (string, bool, error) {
	planRoot, planDirectory, err := findPlanDirectory(planDirectories, planArg)
	if err != nil {
		return "", false, err
	}
	planLock, err := acquirePlanLock(planRoot)
	if err != nil {
		return "", false, err
	}
	defer planLock.close()
	return updatePhaseStatusLocked(planRoot, planDirectory, phaseID, status)
}

func updatePhaseStatusLocked(planRoot, planDirectory string, phaseID int, status string) (string, bool, error) {
	phasePath, err := findPhaseFile(planRoot, phaseID)
	if err != nil {
		return "", false, fmt.Errorf("%s: %w", planDirectory, err)
	}
	phaseRaw, err := os.ReadFile(phasePath)
	if err != nil {
		return "", false, err
	}
	phaseFront, phaseBody, err := frontmatter(string(phaseRaw))
	if err != nil {
		return "", false, fmt.Errorf("parse %s: %w", filepath.Base(phasePath), err)
	}
	if err := validatePhaseStatusChange(phaseFront, status); err != nil {
		return "", false, fmt.Errorf("%s phase %02d: %w", planDirectory, phaseID, err)
	}
	phaseFront["status"] = status
	// completed_at records when the phase reached done; reopening it clears the stamp.
	if status == "done" {
		phaseFront["completed_at"] = completionTimestamp()
	} else {
		delete(phaseFront, "completed_at")
	}
	if err := writeFrontmatterFile(phasePath, phaseFront, phaseBody); err != nil {
		return "", false, err
	}

	phases, err := readPlanPhases(planRoot)
	if err != nil {
		return "", false, err
	}
	planPath := filepath.Join(planRoot, "PLAN.md")
	planRaw, err := os.ReadFile(planPath)
	if err != nil {
		return "", false, err
	}
	planFront, planBody, err := frontmatter(string(planRaw))
	if err != nil {
		return "", false, fmt.Errorf("parse PLAN.md: %w", err)
	}
	completed := len(phases) > 0
	for _, phase := range phases {
		if phase.status != "done" {
			completed = false
			break
		}
	}
	if completed {
		planFront["plan_status"] = "done"
		planFront["completed_at"] = completionTimestamp()
	} else {
		planFront["plan_status"] = "in-progress"
		delete(planFront, "completed_at")
	}
	planBody, err = updatePhaseChecklist(planBody, phaseID, status == "done")
	if err != nil {
		return "", false, fmt.Errorf("update PLAN.md phase checklist: %w", err)
	}
	if err := writeFrontmatterFile(planPath, planFront, planBody); err != nil {
		return "", false, err
	}
	return planDirectory, completed, nil
}

func updatePhaseChecklist(body string, phaseID int, done bool) (string, error) {
	marker := fmt.Sprintf("[Phase %02d:", phaseID)
	checkmark := " "
	if done {
		checkmark = "x"
	}
	lines := strings.SplitAfter(body, "\n")
	updated := 0
	for index, line := range lines {
		if !strings.Contains(line, marker) || !strings.Contains(strings.TrimSpace(line), "- [") {
			continue
		}
		open := strings.Index(line, "[")
		if open < 0 || open+2 >= len(line) || line[open+2] != ']' {
			continue
		}
		lines[index] = line[:open] + "[" + checkmark + "]" + line[open+3:]
		updated++
	}
	if updated == 0 {
		return body, fmt.Errorf("checklist entry for phase %02d not found", phaseID)
	}
	if updated > 1 {
		return body, fmt.Errorf("multiple checklist entries found for phase %02d", phaseID)
	}
	return strings.Join(lines, ""), nil
}

func validatePhaseStatusChange(front map[string]any, status string) error {
	if status == "conditional" {
		condition, _ := front["entry_condition"].(string)
		if strings.TrimSpace(condition) == "" {
			return fmt.Errorf("conditional status requires a non-empty entry_condition")
		}
	}
	if status == "planned" && front["entry_condition"] != nil {
		return fmt.Errorf("planned status requires entry_condition: null")
	}
	return nil
}

func findPlanDirectory(planDirectories []string, planArg string) (string, string, error) {
	type match struct {
		root, directory string
	}
	matches := []match{}
	for _, plans := range planDirectories {
		entries, err := os.ReadDir(plans)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if entry.Name() == planArg || planName(entry.Name()) == planArg {
				matches = append(matches, match{root: filepath.Join(plans, entry.Name()), directory: entry.Name()})
			}
		}
	}
	if len(matches) == 0 {
		return "", "", fmt.Errorf("plan %q not found", planArg)
	}
	if len(matches) > 1 {
		return "", "", fmt.Errorf("plan %q is ambiguous; use its numbered directory name", planArg)
	}
	return matches[0].root, matches[0].directory, nil
}

func findPhaseFile(planRoot string, phaseID int) (string, error) {
	entries, err := os.ReadDir(filepath.Join(planRoot, "phases"))
	if err != nil {
		return "", fmt.Errorf("read phases: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := phaseFilePrefix.FindStringSubmatch(entry.Name())
		if len(match) != 3 {
			continue
		}
		id, err := strconv.Atoi(match[1])
		if err == nil && id == phaseID {
			return filepath.Join(planRoot, "phases", entry.Name()), nil
		}
	}
	return "", fmt.Errorf("phase %02d not found", phaseID)
}

func writeFrontmatterFile(path string, front map[string]any, body string) error {
	contents, err := renderFrontmatterDocument(front, body)
	if err != nil {
		return fmt.Errorf("encode %s frontmatter: %w", filepath.Base(path), err)
	}
	return writeFileAtomically(path, contents)
}

// writeFileAtomically rewrites a document that may already be tracked in git,
// so the contents are staged next to the target and renamed into place: an
// interrupted write leaves the previous contents rather than a truncated
// document.
func writeFileAtomically(path, contents string) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".")
	if err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.WriteString(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := os.Chmod(temporary.Name(), 0644); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}

// pruneEmptyMeta drops keys whose value carries no information: nil, empty
// strings, and empty collections. Plan documents are read by humans, so an
// unset field is better left out than written as `key: null` or `key: []`.
// Booleans and numbers are kept, since false and 0 are real values.
// completionTimestamp is the stamp written into completed_at frontmatter.
func completionTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func pruneEmptyMeta(front map[string]any) map[string]any {
	for key, value := range front {
		if isEmptyMeta(value) {
			delete(front, key)
		}
	}
	return front
}

func isEmptyMeta(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case *string:
		return typed == nil || strings.TrimSpace(*typed) == ""
	}
	switch reflected := reflect.ValueOf(value); reflected.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array:
		return reflected.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return reflected.IsNil()
	}
	return false
}
