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

	git "github.com/go-git/go-git/v5"
	"github.com/goccy/go-yaml"
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
	planDirectories := settings.planDirs(repoRoot)
	planRoot, planDirectory, err := findPlanDirectory(planDirectories, cmd.Args().First())
	if err != nil {
		return err
	}
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
	if status == "done" && !cmd.Bool("force") {
		if err := ensureCleanSource(repoRoot, planDirectories, settings.Ignore); err != nil {
			return err
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
	planDirectory, completed, err = updatePhaseStatus(planDirectories, cmd.Args().First(), phaseID, status)
	if err != nil {
		return err
	}
	fmt.Printf("Updated %s phase %02d: %s\n", planDirectory, phaseID, status)
	if completed {
		fmt.Printf("Plan %s marked done\n", planDirectory)
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

// isPlanDraftPath reports whether a dirty file is a draft produced by
// `planr new`. Drafts survive `planr add`, so without this they show up as
// "uncommitted source changes" and force the author to either commit planr's
// own scratch output or reach for --force.
func isPlanDraftPath(repoRoot, relativePath string) bool {
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
	name, ok := front["plan_name"].(string)
	return ok && name != ""
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
	} else {
		planFront["plan_status"] = "in-progress"
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
	header, err := yaml.Marshal(front)
	if err != nil {
		return fmt.Errorf("encode %s frontmatter: %w", filepath.Base(path), err)
	}
	contents := "---\n" + string(header) + "---\n" + body
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}
