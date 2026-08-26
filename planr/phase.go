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
	planDirectories, err := planPaths(cwd)
	if err != nil {
		return err
	}
	if status == "done" && !cmd.Bool("force") {
		_, repoRoot, err := loadConfig(cwd)
		if err != nil {
			return err
		}
		if err := ensureCleanSource(repoRoot, planDirectories); err != nil {
			return err
		}
	}
	planDirectory, completed, err := updatePhaseStatus(planDirectories, cmd.Args().First(), phaseID, status)
	if err != nil {
		return err
	}
	fmt.Printf("Updated %s phase %02d: %s\n", planDirectory, phaseID, status)
	if completed {
		fmt.Printf("Plan %s marked done\n", planDirectory)
	}
	return nil
}

func ensureCleanSource(repoRoot string, planDirectories []string) error {
	paths, err := uncommittedSourcePaths(repoRoot, planDirectories)
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

func uncommittedSourcePaths(repoRoot string, planDirectories []string) ([]string, error) {
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
		if !isGeneratedPlanPath(repoRoot, planDirectories, path) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
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
	prefix := regexp.MustCompile(`^(\d+)-.*\.md$`)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := prefix.FindStringSubmatch(entry.Name())
		if len(match) != 2 {
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
