package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"
)

var phaseAddStatusValues = map[string]bool{
	"planned":     true,
	"conditional": true,
}

func phaseAddCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 2 {
		return fmt.Errorf("phase add requires <plan-name> <phase-title>")
	}
	title := strings.TrimSpace(cmd.Args().Get(1))
	if title == "" {
		return fmt.Errorf("phase title must not be empty")
	}
	if strings.ContainsAny(title, "\r\n") {
		return fmt.Errorf("phase title must be a single line")
	}
	planned, err := requiredPhaseText(cmd.String("work"), "--work")
	if err != nil {
		return err
	}
	completion, err := requiredPhaseText(cmd.String("done-when"), "--done-when")
	if err != nil {
		return err
	}
	status := strings.TrimSpace(cmd.String("status"))
	if !phaseAddStatusValues[status] {
		return fmt.Errorf("invalid new phase status %q; use planned or conditional", status)
	}
	entryCondition := strings.TrimSpace(cmd.String("entry-condition"))
	if status == "conditional" && entryCondition == "" {
		return fmt.Errorf("conditional phase requires --entry-condition")
	}
	if status == "planned" && entryCondition != "" {
		return fmt.Errorf("planned phase cannot set --entry-condition")
	}
	slug := strings.TrimSpace(cmd.String("slug"))
	if slug == "" {
		slug = slugifyPhaseTitle(title)
	}
	if !kebab.MatchString(slug) {
		return fmt.Errorf("phase slug %q must be lowercase kebab-case; pass --slug explicitly for non-ASCII titles", slug)
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
	planRaw, err := os.ReadFile(filepath.Join(planRoot, "PLAN.md"))
	if err != nil {
		return err
	}
	planFront, planBody, err := frontmatter(string(planRaw))
	if err != nil {
		return fmt.Errorf("parse %s/PLAN.md: %w", planDirectory, err)
	}
	if planStatus, _ := planFront["plan_status"].(string); planStatus == "done" {
		return fmt.Errorf("plan %q is already done; phase add is only allowed for open plans", planDirectory)
	}
	phases, err := readPlanPhases(planRoot)
	if err != nil {
		return err
	}
	phaseID := nextPhaseID(phases)
	for _, phase := range phases {
		if phase.slug == slug {
			return fmt.Errorf("phase slug %q already exists in plan %q", slug, planDirectory)
		}
	}
	dependencies, err := parsePhaseAddDependencies(cmd.StringSlice("depends-on"), phases)
	if err != nil {
		return err
	}
	var entryValue *string
	if entryCondition != "" {
		entryValue = &entryCondition
	}
	meta := phaseMeta{
		Phase:          phaseID,
		Slug:           slug,
		PerfPhase:      cmd.Bool("perf-phase"),
		DependsOn:      dependencies,
		Status:         status,
		EntryCondition: entryValue,
	}
	if err := validateNewPhaseDependencies(planDirectory, meta, title, phases); err != nil {
		return err
	}
	updatedPlanBody, err := appendPhaseChecklist(planBody, phaseID, title, slug)
	if err != nil {
		return fmt.Errorf("update %s/PLAN.md: %w", planDirectory, err)
	}
	if err := runConfiguredHooks(repoRoot, settings, "before", hookEventPhaseAdd, planDirectory, phaseID, status); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(planRoot, "phases"), 0755); err != nil {
		return err
	}
	phasePath := filepath.Join(planRoot, phaseDocumentPath(phaseID, slug))
	if err := writeFrontmatterFile(phasePath, phaseFrontmatter(planDirectory, meta), phaseDocumentBody(title, planned, completion)); err != nil {
		return err
	}
	planFront["plan_status"] = "in-progress"
	if err := writeFrontmatterFile(filepath.Join(planRoot, "PLAN.md"), planFront, updatedPlanBody); err != nil {
		_ = os.Remove(phasePath)
		return err
	}
	fmt.Printf("Added %s phase %02d: %s\n", planDirectory, phaseID, phasePath)
	if err := runConfiguredHooks(repoRoot, settings, "after", hookEventPhaseAdd, planDirectory, phaseID, status); err != nil {
		return err
	}
	return nil
}

func requiredPhaseText(value, flag string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("phase add requires %s", flag)
	}
	return value, nil
}

// parsePhaseAddDependencies accepts each dependency as a phase number or as the
// slug of an existing phase, matching what a draft's depends_on list allows.
func parsePhaseAddDependencies(values []string, existing []storedPhase) ([]int, error) {
	numbers := map[string]int{}
	known := make([]string, 0, len(existing))
	for _, phase := range existing {
		numbers[phase.slug] = phase.id
		known = append(known, phase.slug)
	}
	sort.Strings(known)
	seen := map[int]bool{}
	dependencies := []int{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				return nil, fmt.Errorf("--depends-on must contain phase numbers or phase slugs")
			}
			phase, err := strconv.Atoi(part)
			if err != nil {
				resolved, ok := numbers[part]
				if !ok {
					return nil, fmt.Errorf("phase dependency %q is neither a phase number nor a slug of an existing phase; available slugs: %s",
						part, strings.Join(known, ", "))
				}
				phase = resolved
			} else if phase < 0 {
				return nil, fmt.Errorf("phase dependency %q must be a non-negative phase number", part)
			}
			if seen[phase] {
				return nil, fmt.Errorf("phase dependency %d is listed more than once", phase)
			}
			seen[phase] = true
			dependencies = append(dependencies, phase)
		}
	}
	sort.Ints(dependencies)
	return dependencies, nil
}

// slugifyPhaseTitle collapses each run of non-alphanumeric characters into a
// single dash and trims the dashes at both ends.
func slugifyPhaseTitle(title string) string {
	var builder strings.Builder
	pendingDash := false
	for _, value := range strings.ToLower(title) {
		if value >= 'a' && value <= 'z' || value >= '0' && value <= '9' {
			if pendingDash {
				builder.WriteByte('-')
				pendingDash = false
			}
			builder.WriteRune(value)
			continue
		}
		pendingDash = builder.Len() > 0
	}
	return builder.String()
}

func nextPhaseID(phases []storedPhase) int {
	next := 0
	for _, phase := range phases {
		if phase.id >= next {
			next = phase.id + 1
		}
	}
	return next
}

// validateNewPhaseDependencies projects the stored phases plus the phase about
// to be added into the draft shape so the single dependency validator used by
// `add` decides whether the resulting graph is legal.
func validateNewPhaseDependencies(planDirectory string, newPhase phaseMeta, title string, existing []storedPhase) error {
	plan := planName(planDirectory)
	all := make([]draftPhase, 0, len(existing)+1)
	for _, phase := range existing {
		meta := phaseMeta{Phase: phase.id, Slug: phase.slug, Status: phase.status}
		for _, raw := range phase.dependencies {
			dependency, err := parseDependency(raw)
			if err != nil || dependency.phase == nil || dependency.plan != plan {
				return fmt.Errorf("phase %d has invalid internal dependency %q", phase.id, raw)
			}
			meta.DependsOn = append(meta.DependsOn, *dependency.phase)
		}
		all = append(all, draftPhase{Title: phase.title, Meta: meta})
	}
	all = append(all, draftPhase{Title: title, Meta: newPhase})
	if err := validatePhaseDependencies(all); err != nil {
		return fmt.Errorf("invalid dependencies for new phase %d: %w", newPhase.Phase, err)
	}
	return nil
}

func appendPhaseChecklist(body string, phaseID int, title, slug string) (string, error) {
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
	entry := phaseChecklistEntry(phaseID, title, slug)
	before := strings.TrimRight(body[:insertion], "\n")
	after := strings.TrimLeft(body[insertion:], "\n")
	if after == "" {
		return before + "\n\n" + entry + "\n", nil
	}
	return before + "\n" + entry + "\n\n" + after, nil
}

func firstPhaseLine(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(strings.SplitN(value, "\n", 2)[0]), "- ")
}
