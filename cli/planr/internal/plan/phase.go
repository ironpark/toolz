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

	git "github.com/go-git/go-git/v5"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
)

var StatusValues = map[string]bool{
	"planned":     true,
	"conditional": true,
	"in-progress": true,
	"done":        true,
}

// EnsureDependenciesMet refuses to advance a phase whose prerequisites are not
// done. It covers both the phase's own depends_on and the plan-level
// depends_on in PLAN.md, which is what `status` reports as `wait`. Resetting a
// phase to planned or conditional moves backwards and is never blocked.
func EnsureDependenciesMet(planDirectories []string, planRoot, planDirectory string, phaseID int, status string) error {
	if status != "in-progress" && status != "done" {
		return nil
	}
	phases, err := ReadPhases(planRoot)
	if err != nil {
		return err
	}
	local := map[int]StoredPhase{}
	var target *StoredPhase
	for index, phase := range phases {
		local[phase.ID] = phase
		if phase.ID == phaseID {
			target = &phases[index]
		}
	}
	// A missing phase is reported by updatePhaseStatus with a better message.
	if target == nil {
		return nil
	}

	unmet := []string{}
	for _, raw := range target.Dependencies {
		dependency, parseErr := draft.ParseDependency(raw)
		if parseErr != nil {
			unmet = append(unmet, fmt.Sprintf("%s (unreadable dependency)", raw))
			continue
		}
		if dependency.Plan == draft.Name(planDirectory) && dependency.Phase != nil {
			phase, found := local[*dependency.Phase]
			switch {
			case !found:
				unmet = append(unmet, fmt.Sprintf("phase %02d (not found)", *dependency.Phase))
			case phase.Status != "done":
				unmet = append(unmet, fmt.Sprintf("phase %02d %q (%s)", phase.ID, phase.Title, phase.Status))
			}
			continue
		}
		if reason := unmetDependency(planDirectories, dependency); reason != "" {
			unmet = append(unmet, reason)
		}
	}
	planDependencies, err := planLevelDependencies(planRoot)
	if err != nil {
		return err
	}
	for _, dependency := range planDependencies {
		if reason := unmetDependency(planDirectories, dependency); reason != "" {
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

// unmetDependency describes why a dependency on another plan is not
// satisfied, or returns an empty string when it is. A dependency naming a plan
// that was never registered counts as unmet: drafts may reference plans that do
// not exist yet, but work cannot proceed past one.
func unmetDependency(planDirectories []string, dependency draft.Dependency) string {
	label := draft.DependencyLabel(dependency)
	planRoot, _, err := FindDirectory(planDirectories, dependency.Plan)
	if err != nil {
		return fmt.Sprintf("%s (not registered)", label)
	}
	if dependency.Phase == nil {
		done, err := AlreadyDone(planRoot)
		if err != nil {
			return fmt.Sprintf("%s (unreadable)", label)
		}
		if !done {
			return fmt.Sprintf("%s (in-progress)", label)
		}
		return ""
	}
	phases, err := ReadPhases(planRoot)
	if err != nil {
		return fmt.Sprintf("%s (unreadable)", label)
	}
	for _, phase := range phases {
		if phase.ID != *dependency.Phase {
			continue
		}
		if phase.Status != "done" {
			return fmt.Sprintf("%s (%s)", label, phase.Status)
		}
		return ""
	}
	return fmt.Sprintf("%s (phase not found)", label)
}

// planLevelDependencies reads the depends_on list from a plan's PLAN.md.
func planLevelDependencies(planRoot string) ([]draft.Dependency, error) {
	front, _, err := ReadDocument(planRoot, "PLAN.md")
	if err != nil {
		return nil, err
	}
	dependencies := []draft.Dependency{}
	for _, raw := range mdoc.Strings(front["depends_on"]) {
		dependency, err := draft.ParseDependency(raw)
		if err != nil {
			continue
		}
		dependencies = append(dependencies, dependency)
	}
	return dependencies, nil
}

func EnsureCleanSource(repoRoot string, planDirectories, ignore []string) error {
	paths, err := UncommittedSourcePaths(repoRoot, planDirectories, ignore)
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

func UncommittedSourcePaths(repoRoot string, planDirectories, ignore []string) ([]string, error) {
	repository, err := git.PlainOpenWithOptions(repoRoot, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
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
	ignorePatterns := compileIgnorePatterns(ignore)
	for path, fileStatus := range status {
		if fileStatus == nil || (fileStatus.Staging == git.Unmodified && fileStatus.Worktree == git.Unmodified) {
			continue
		}
		if !isGeneratedPlanPath(repoRoot, planDirectories, path) && !isPlanDraftPath(repoRoot, path) && !matchesIgnorePatterns(path, ignorePatterns) {
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
	front, _, err := mdoc.Split(string(raw))
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

// ignorePattern is one config ignore entry, with its glob form compiled once
// so matching many paths does not recompile the same expression.
type ignorePattern struct {
	raw        string
	expression *regexp.Regexp
}

func compileIgnorePatterns(patterns []string) []ignorePattern {
	compiled := make([]ignorePattern, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		pattern = strings.TrimPrefix(pattern, "./")
		if pattern == "" {
			continue
		}
		compiled = append(compiled, ignorePattern{raw: pattern, expression: globPathExpression(pattern)})
	}
	return compiled
}

func (p ignorePattern) match(path string) bool {
	if p.expression != nil && p.expression.MatchString(path) {
		return true
	}
	return !strings.ContainsAny(p.raw, "*?") && (path == p.raw || strings.HasPrefix(path, strings.TrimSuffix(p.raw, "/")+"/"))
}

func IsIgnoredPath(relativePath string, patterns []string) bool {
	return matchesIgnorePatterns(relativePath, compileIgnorePatterns(patterns))
}

func matchesIgnorePatterns(relativePath string, patterns []ignorePattern) bool {
	path := filepath.ToSlash(filepath.Clean(relativePath))
	for _, pattern := range patterns {
		if pattern.match(path) {
			return true
		}
	}
	return false
}

func globPathExpression(pattern string) *regexp.Regexp {
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
	compiled, err := regexp.Compile(expression.String())
	if err != nil {
		return nil
	}
	return compiled
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

func HookEvent(status string) string {
	switch status {
	case "planned":
		return hooks.EventReset
	case "conditional":
		return hooks.EventConditional
	case "in-progress":
		return hooks.EventStart
	case "done":
		return hooks.EventDone
	default:
		return ""
	}
}

func WillComplete(planRoot string, phaseID int) (bool, error) {
	phases, err := ReadPhases(planRoot)
	if err != nil {
		return false, err
	}
	if len(phases) == 0 {
		return false, nil
	}
	found := false
	for _, phase := range phases {
		if phase.ID == phaseID {
			found = true
			continue
		}
		if phase.Status != "done" {
			return false, nil
		}
	}
	if !found {
		return false, fmt.Errorf("phase %02d not found", phaseID)
	}
	return true, nil
}

func AlreadyDone(planRoot string) (bool, error) {
	front, _, err := ReadDocument(planRoot, "PLAN.md")
	if err != nil {
		return false, err
	}
	status, _ := front["plan_status"].(string)
	return status == "done", nil
}

func UpdatePhaseStatusLocked(planRoot, planDirectory string, phaseID int, status string) (string, bool, error) {
	phasePath, err := FindPhaseFile(planRoot, phaseID)
	if err != nil {
		return "", false, fmt.Errorf("%s: %w", planDirectory, err)
	}
	phaseRaw, err := os.ReadFile(phasePath)
	if err != nil {
		return "", false, err
	}
	phaseFront, phaseBody, err := mdoc.Split(string(phaseRaw))
	if err != nil {
		return "", false, fmt.Errorf("parse %s: %w", filepath.Base(phasePath), err)
	}
	if err := ValidateStatusChange(phaseFront, status); err != nil {
		return "", false, fmt.Errorf("%s phase %02d: %w", planDirectory, phaseID, err)
	}
	phaseFront["status"] = status
	// completed_at records when the phase reached done; reopening it clears the stamp.
	if status == "done" {
		phaseFront["completed_at"] = CompletionTimestamp()
	} else {
		delete(phaseFront, "completed_at")
	}
	if err := mdoc.WriteFile(phasePath, phaseFront, phaseBody); err != nil {
		return "", false, err
	}

	phases, err := ReadPhases(planRoot)
	if err != nil {
		return "", false, err
	}
	planPath := filepath.Join(planRoot, "PLAN.md")
	planRaw, err := os.ReadFile(planPath)
	if err != nil {
		return "", false, err
	}
	planFront, planBody, err := mdoc.Split(string(planRaw))
	if err != nil {
		return "", false, fmt.Errorf("parse PLAN.md: %w", err)
	}
	completed := len(phases) > 0
	for _, phase := range phases {
		if phase.Status != "done" {
			completed = false
			break
		}
	}
	if completed {
		planFront["plan_status"] = "done"
		planFront["completed_at"] = CompletionTimestamp()
	} else {
		planFront["plan_status"] = "in-progress"
		delete(planFront, "completed_at")
	}
	planBody, err = UpdateChecklist(planBody, phaseID, status == "done")
	if err != nil {
		return "", false, fmt.Errorf("update PLAN.md phase checklist: %w", err)
	}
	if err := mdoc.WriteFile(planPath, planFront, planBody); err != nil {
		return "", false, err
	}
	return planDirectory, completed, nil
}

func UpdateChecklist(body string, phaseID int, done bool) (string, error) {
	checkmark := " "
	if done {
		checkmark = "x"
	}
	return TransformChecklistEntry(body, phaseID, func(line string) (string, bool) {
		open := strings.Index(line, "[")
		if open < 0 || open+2 >= len(line) || line[open+2] != ']' {
			return "", false
		}
		return line[:open] + "[" + checkmark + "]" + line[open+3:], true
	})
}

func ValidateStatusChange(front map[string]any, status string) error {
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

func FindDirectory(planDirectories []string, planArg string) (string, string, error) {
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
			if entry.Name() == planArg || draft.Name(entry.Name()) == planArg {
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

func FindPhaseFile(planRoot string, phaseID int) (string, error) {
	entries, err := os.ReadDir(filepath.Join(planRoot, "phases"))
	if err != nil {
		return "", fmt.Errorf("read phases: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := PhaseFilePrefix.FindStringSubmatch(entry.Name())
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

// CompletionTimestamp is the stamp written into completed_at frontmatter.
func CompletionTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
