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

	"github.com/goccy/go-yaml"
	"github.com/urfave/cli/v3"
)

func newCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 1 {
		return fmt.Errorf("new requires <plan-name>")
	}
	name := cmd.Args().First()
	if !kebab.MatchString(name) {
		return fmt.Errorf("plan name %q must be lowercase kebab-case", name)
	}
	output := cmd.String("output")
	if output == "" {
		output = name + ".md"
	}
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absOutput); err == nil {
		return fmt.Errorf("draft file already exists: %s", absOutput)
	} else if !os.IsNotExist(err) {
		return err
	}
	draft, err := renderNewDraft(name)
	if err != nil {
		return err
	}
	if err := os.WriteFile(absOutput, []byte(draft), 0644); err != nil {
		return err
	}
	fmt.Printf("Created %s\n", absOutput)
	return nil
}

func addCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 1 {
		return fmt.Errorf("register requires <draft-file>")
	}
	file := cmd.Args().First()
	raw, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	d, err := parseDraft(raw, file)
	if err != nil {
		return err
	}
	if name := cmd.String("name"); name != "" {
		if !kebab.MatchString(name) {
			return fmt.Errorf("--name must be lowercase kebab-case")
		}
		d.Name = name
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	planDirectories, err := planPaths(workingDirectory)
	if err != nil {
		return err
	}
	plans := planDirectories[0]
	planDirectory, err := nextPlanDirectory(planDirectories, d.Name)
	if err != nil {
		return err
	}
	target := filepath.Join(plans, planDirectory)
	tmp, err := os.MkdirTemp(plans, ".planr-")
	if err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(plans, 0755); err != nil {
				return err
			}
			tmp, err = os.MkdirTemp(plans, ".planr-")
		}
		if err != nil {
			return err
		}
	}
	defer os.RemoveAll(tmp)
	if err := writePlan(tmp, d, planDirectory); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		return err
	}
	fmt.Printf("Registered %s\n", planDirectory)
	return nil
}

func nextPlanDirectory(planDirectories []string, name string) (string, error) {
	maxIndex := -1
	prefix := regexp.MustCompile(`^(\d+)-(.+)$`)
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
			match := prefix.FindStringSubmatch(entry.Name())
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

func writePlan(root string, d draft, planDirectory string) error {
	if err := os.MkdirAll(filepath.Join(root, "phases"), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "GOALS.md"), []byte("# GOALS\n\n"+d.Goals+"\n"), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "CONTEXT.md"), []byte("# SCOPE\n\n"+d.Scope+"\n\n# CONTEXT\n\n"+d.Context+"\n"), 0644); err != nil {
		return err
	}
	checklist := []string{}
	for _, p := range d.Phases {
		doc := fmt.Sprintf("phases/%02d-%s.md", p.Meta.Phase, p.Meta.Slug)
		checklist = append(checklist, fmt.Sprintf("- [ ] [Phase %02d: %s](%s)", p.Meta.Phase, p.Title, doc))
		dependencies := make([]string, len(p.Meta.DependsOn))
		for index, dependency := range p.Meta.DependsOn {
			dependencies[index] = fmt.Sprintf("%s#%d", planDirectory, dependency)
		}
		meta := map[string]any{"status": p.Meta.Status, "entry_condition": p.Meta.EntryCondition, "impl_commits": []string{}, "followup_commits": []string{}, "perf_phase": p.Meta.PerfPhase, "depends_on": dependencies, "blocks": []string{}}
		header, err := yaml.Marshal(meta)
		if err != nil {
			return err
		}
		done := strings.Split(p.Completion, "\n")[0]
		content := fmt.Sprintf("---\n%s---\n> DONE-WHEN: %s\n> NEXT: 없음\n\n# %s\n\n## 계획된 작업\n\n%s\n\n## 완료 조건\n\n%s\n", header, strings.TrimPrefix(done, "- "), p.Title, p.Planned, p.Completion)
		if err := os.WriteFile(filepath.Join(root, doc), []byte(content), 0644); err != nil {
			return err
		}
	}
	meta := map[string]any{"plan_status": "in-progress", "succeeded_by": nil, "preceded_by": nil}
	header, err := yaml.Marshal(meta)
	if err != nil {
		return err
	}
	nextDoc := ""
	for _, p := range d.Phases {
		if p.Meta.Phase == d.NextPhase {
			nextDoc = fmt.Sprintf("phases/%02d-%s.md", p.Meta.Phase, p.Meta.Slug)
		}
	}
	plan := fmt.Sprintf("---\n%s---\n> NEXT: %s ([Phase %d](%s))\n\n# Phases\n\n%s\n\n# 공통 검증\n\n%s\n\n# 구현 순서를 제한하는 결정\n\n%s\n\n# 다음 구현 대상\n\n%s\n", header, d.NextText, d.NextPhase, nextDoc, strings.Join(checklist, "\n"), d.Verification, d.Ordering, d.NextText)
	return os.WriteFile(filepath.Join(root, "PLAN.md"), []byte(plan), 0644)
}

func statusCommand(_ context.Context, cmd *cli.Command) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	planDirectories, err := planPaths(cwd)
	if err != nil {
		return err
	}
	if cmd.NArg() > 1 {
		return fmt.Errorf("status accepts at most one plan name")
	}
	type planStatus struct {
		name, label, status string
		done, total         int
		dependsOn           map[string]bool
		remaining           []string
		blockedBy           []string
	}
	var plansToShow []planStatus
	foundDirectory := false
	for _, plans := range planDirectories {
		entries, err := os.ReadDir(plans)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		foundDirectory = true
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if cmd.NArg() == 1 && entry.Name() != cmd.Args().First() && planName(entry.Name()) != cmd.Args().First() {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(plans, entry.Name(), "PLAN.md"))
			if err != nil {
				continue
			}
			front, _, err := frontmatter(string(raw))
			if err != nil {
				return fmt.Errorf("%s: %w", entry.Name(), err)
			}
			phases, err := readPlanPhases(filepath.Join(plans, entry.Name()))
			if err != nil {
				return err
			}
			done := 0
			dependencies := map[string]bool{}
			remaining := []string{}
			for _, phase := range phases {
				if phase.status == "done" {
					done++
				} else {
					remaining = append(remaining, fmt.Sprintf("%s (%s)", phase.title, phase.status))
				}
				if front["plan_status"] != "done" {
					for _, dependency := range phase.dependencies {
						if planName, _, found := strings.Cut(dependency, "#"); found {
							dependencies[planName] = true
						}
					}
				}
			}
			label := filepath.Join(filepath.Base(plans), entry.Name())
			status, _ := front["plan_status"].(string)
			plansToShow = append(plansToShow, planStatus{entry.Name(), label, status, done, len(phases), dependencies, remaining, nil})
		}
	}
	if !foundDirectory {
		return fmt.Errorf("no plans directories found: %s", strings.Join(planDirectories, ", "))
	}
	requiredPlans := map[string]bool{}
	planStatuses := map[string]string{}
	for _, plan := range plansToShow {
		planStatuses[plan.name] = plan.status
	}
	for index := range plansToShow {
		plan := &plansToShow[index]
		if plan.status == "done" {
			continue
		}
		for dependency := range plan.dependsOn {
			requiredPlans[dependency] = true
			if dependency == plan.name {
				continue
			}
			dependencyStatus, found := planStatuses[dependency]
			if !found {
				plan.blockedBy = append(plan.blockedBy, fmt.Sprintf("%s (not found)", dependency))
			} else if dependencyStatus != "done" {
				plan.blockedBy = append(plan.blockedBy, fmt.Sprintf("%s (%s)", dependency, dependencyStatus))
			}
		}
	}
	currentDirectory := ""
	for _, plan := range plansToShow {
		if cmd.NArg() == 0 && plan.status == "done" && !requiredPlans[plan.name] {
			continue
		}
		directory, name := filepath.Split(plan.label)
		if directory != currentDirectory {
			fmt.Printf("%s\n", directory)
			currentDirectory = directory
		}
		fmt.Printf("  %s: %s (%d/%d phases done)\n", name, plan.status, plan.done, plan.total)
		if len(plan.remaining) > 0 {
			fmt.Println("    remaining:")
			for _, phase := range plan.remaining {
				fmt.Printf("      - %s\n", phase)
			}
		}
		if len(plan.blockedBy) > 0 {
			fmt.Println("    wait:")
			for _, dependency := range plan.blockedBy {
				fmt.Printf("      - %s\n", dependency)
			}
		}
	}
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
	id            int
	title, status string
	dependencies  []string
}

func readPlanPhases(planRoot string) ([]storedPhase, error) {
	entries, err := os.ReadDir(filepath.Join(planRoot, "phases"))
	if err != nil {
		return nil, fmt.Errorf("read phases for %s: %w", filepath.Base(planRoot), err)
	}
	prefix := regexp.MustCompile(`^(\d+)-.*\.md$`)
	phases := []storedPhase{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := prefix.FindStringSubmatch(entry.Name())
		if len(match) != 2 {
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
		phases = append(phases, storedPhase{id, markdownTitle(string(contents)), status, yamlStrings(front["depends_on"])})
	}
	sort.Slice(phases, func(i, j int) bool { return phases[i].id < phases[j].id })
	return phases, nil
}

func planName(directory string) string {
	if _, name, found := strings.Cut(directory, "-"); found {
		return name
	}
	return directory
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
