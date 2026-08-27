package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type statusJSONOutput struct {
	Plans []statusPlanJSON `json:"plans"`
}

type statusPlanJSON struct {
	Name        string            `json:"name"`
	Directory   string            `json:"directory"`
	Status      string            `json:"status"`
	DonePhases  int               `json:"done_phases"`
	TotalPhases int               `json:"total_phases"`
	Remaining   []statusPhaseJSON `json:"remaining"`
	Wait        []string          `json:"wait"`
}

type statusPhaseJSON struct {
	PhaseNumber int    `json:"phase_number"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Status      string `json:"status"`
}

type overviewJSONOutput struct {
	Plans []overviewPlanJSON `json:"plans"`
}

type overviewPlanJSON struct {
	Name        string   `json:"name"`
	Directory   string   `json:"directory"`
	Status      string   `json:"status"`
	DonePhases  int      `json:"done_phases"`
	TotalPhases int      `json:"total_phases"`
	NextPhase   string   `json:"next_phase"`
	Wait        []string `json:"wait"`
}

type notesJSONOutput struct {
	Notes []noteJSON `json:"notes"`
}

type showJSONOutput struct {
	Plan        string   `json:"plan"`
	Directory   string   `json:"directory"`
	PhaseNumber int      `json:"phase_number"`
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	PlannedWork string   `json:"planned_work"`
	DoneWhen    string   `json:"done_when"`
	DependsOn   []string `json:"depends_on"`
	File        string   `json:"file"`
}

type configJSONOutput struct {
	ConfigFile     *string         `json:"config_file"`
	RepositoryRoot string          `json:"repository_root"`
	Agent          string          `json:"agent"`
	Language       string          `json:"language"`
	PlansDirs      []string        `json:"plans_dirs"`
	Ignore         []string        `json:"ignore"`
	Hooks          configHooksJSON `json:"hooks"`
}

type configHooksJSON struct {
	Timeout string         `json:"timeout"`
	Before  []hookRuleJSON `json:"before"`
	After   []hookRuleJSON `json:"after"`
}

type hookRuleJSON struct {
	On  []string `json:"on"`
	Run string   `json:"run"`
}

type doctorJSONOutput struct {
	Issues []doctorIssueJSON `json:"issues"`
}

type doctorIssueJSON struct {
	Location string `json:"location"`
	Message  string `json:"message"`
}

type noteJSON struct {
	CompletedAt string `json:"completed_at"`
	Plan        string `json:"plan"`
	Event       string `json:"event"`
	Phase       string `json:"phase"`
	Commit      string `json:"commit"`
	ShortCommit string `json:"short_commit"`
	Subject     string `json:"subject"`
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write JSON output: %w", err)
	}
	return nil
}

func makeStatusJSON(summaries []planSummary) statusJSONOutput {
	plans := make([]statusPlanJSON, 0, len(summaries))
	for _, summary := range summaries {
		done, total, _ := summary.progress()
		remaining := make([]statusPhaseJSON, 0)
		for _, phase := range summary.phases {
			if phase.status == "done" {
				continue
			}
			remaining = append(remaining, statusPhaseJSON{
				PhaseNumber: phase.id,
				Slug:        phase.slug,
				Title:       phase.title,
				Status:      phase.status,
			})
		}
		plans = append(plans, statusPlanJSON{
			Name:        summary.name,
			Directory:   filepath.ToSlash(summary.label),
			Status:      summary.status,
			DonePhases:  done,
			TotalPhases: total,
			Remaining:   remaining,
			Wait:        append([]string{}, summary.wait...),
		})
	}
	return statusJSONOutput{Plans: plans}
}

func makeOverviewJSON(summaries []planSummary) overviewJSONOutput {
	plans := make([]overviewPlanJSON, 0, len(summaries))
	for _, summary := range summaries {
		status := summary.status
		if status == "" {
			status = "unknown"
		}
		done, total, next := summary.progress()
		plans = append(plans, overviewPlanJSON{
			Name:        summary.name,
			Directory:   filepath.ToSlash(summary.label),
			Status:      status,
			DonePhases:  done,
			TotalPhases: total,
			NextPhase:   next,
			Wait:        append([]string{}, summary.wait...),
		})
	}
	return overviewJSONOutput{Plans: plans}
}

func makeNotesJSON(notes []planNote) notesJSONOutput {
	values := make([]noteJSON, 0, len(notes))
	for _, note := range notes {
		values = append(values, noteJSON{
			CompletedAt: note.at,
			Plan:        note.plan,
			Event:       note.event,
			Phase:       note.phase,
			Commit:      note.commit,
			ShortCommit: note.shortHash,
			Subject:     note.subject,
		})
	}
	return notesJSONOutput{Notes: values}
}

func makeShowJSON(phase phaseDetails) showJSONOutput {
	return showJSONOutput{
		Plan:        phase.plan,
		Directory:   phase.directory,
		PhaseNumber: phase.id,
		Slug:        phase.slug,
		Title:       phase.title,
		Status:      phase.status,
		PlannedWork: phase.plannedWork,
		DoneWhen:    phase.doneWhen,
		DependsOn:   append([]string{}, phase.dependencies...),
		File:        phase.file,
	}
}

func makeConfigJSON(settings config, root string) configJSONOutput {
	var configFile *string
	if settings.configPath != "" {
		value := settings.configPath
		configFile = &value
	}
	return configJSONOutput{
		ConfigFile:     configFile,
		RepositoryRoot: root,
		Agent:          currentAgentDescription(),
		Language:       settings.Language,
		PlansDirs:      append([]string{}, settings.PlansDirs...),
		Ignore:         append([]string{}, settings.Ignore...),
		Hooks: configHooksJSON{
			Timeout: settings.Hooks.timeoutDuration().String(),
			Before:  makeHookRulesJSON(settings.Hooks.Before),
			After:   makeHookRulesJSON(settings.Hooks.After),
		},
	}
}

func makeHookRulesJSON(rules []hookRule) []hookRuleJSON {
	result := make([]hookRuleJSON, 0, len(rules))
	for _, rule := range rules {
		result = append(result, hookRuleJSON{On: append([]string{}, rule.On...), Run: rule.Run})
	}
	return result
}

func makeDoctorJSON(issues []doctorIssue) doctorJSONOutput {
	result := make([]doctorIssueJSON, 0, len(issues))
	for _, issue := range issues {
		result = append(result, doctorIssueJSON{Location: issue.location, Message: issue.message})
	}
	return doctorJSONOutput{Issues: result}
}
