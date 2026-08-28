package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ironpark/toolz/cli/planr/internal/agentenv"
	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/doctor"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/ironpark/toolz/cli/planr/internal/notes"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/ironpark/toolz/cli/planr/internal/validation"
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

type templateJSONOutput struct {
	Kind     string `json:"kind"`
	Selector string `json:"selector"`
	Template string `json:"template"`
}

type editJSONOutput struct {
	Kind     string `json:"kind"`
	Selector string `json:"selector"`
	Section  string `json:"section,omitempty"`
	Target   string `json:"target"`
	Base     string `json:"base"`
	Document string `json:"document"`
}

type applyJSONOutput struct {
	Ok        bool              `json:"ok"`
	Action    string            `json:"action"`
	Selector  string            `json:"selector"`
	DryRun    bool              `json:"dry_run"`
	Changed   bool              `json:"changed"`
	Documents map[string]string `json:"documents"`
	Diff      []applyDiffJSON   `json:"diff"`
}

type applyDiffJSON struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type applyFailureJSON struct {
	Ok     bool                  `json:"ok"`
	Errors []validationErrorJSON `json:"errors"`
}

type validationErrorJSON struct {
	Rule    string `json:"rule"`
	Section string `json:"section,omitempty"`
	Phase   *int   `json:"phase,omitempty"`
	Line    int    `json:"line,omitempty"`
	Phases  []int  `json:"phases,omitempty"`
	Detail  string `json:"detail"`
}

type showSectionJSONOutput struct {
	Plan      string `json:"plan"`
	Directory string `json:"directory"`
	Section   string `json:"section"`
	Content   string `json:"content"`
	File      string `json:"file"`
}

type showAllJSONOutput struct {
	Plan         string            `json:"plan"`
	Directory    string            `json:"directory"`
	Status       string            `json:"status"`
	Description  string            `json:"description"`
	DependsOn    []string          `json:"depends_on"`
	Goals        string            `json:"goals"`
	Context      string            `json:"context"`
	PlanDocument string            `json:"plan_document"`
	Phases       []showJSONOutput  `json:"phases"`
	Documents    map[string]string `json:"documents"`
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

type initJSONOutput struct {
	ConfigFile     string   `json:"config_file"`
	RepositoryRoot string   `json:"repository_root"`
	Language       string   `json:"language"`
	PlansDirs      []string `json:"plans_dirs"`
	// Created and Existed are separate so a caller can tell a fresh setup from
	// a rerun without diffing the filesystem itself.
	Created []string `json:"created"`
	Existed []string `json:"existed"`
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

func makeStatusJSON(summaries []plan.Summary) statusJSONOutput {
	plans := make([]statusPlanJSON, 0, len(summaries))
	for _, summary := range summaries {
		done, total, _ := summary.Progress()
		remaining := make([]statusPhaseJSON, 0)
		for _, phase := range summary.Phases {
			if phase.Status == "done" {
				continue
			}
			remaining = append(remaining, statusPhaseJSON{
				PhaseNumber: phase.ID,
				Slug:        phase.Slug,
				Title:       phase.Title,
				Status:      phase.Status,
			})
		}
		plans = append(plans, statusPlanJSON{
			Name:        summary.Name,
			Directory:   filepath.ToSlash(summary.Label),
			Status:      summary.Status,
			DonePhases:  done,
			TotalPhases: total,
			Remaining:   remaining,
			Wait:        append([]string{}, summary.Wait...),
		})
	}
	return statusJSONOutput{Plans: plans}
}

func makeOverviewJSON(summaries []plan.Summary) overviewJSONOutput {
	plans := make([]overviewPlanJSON, 0, len(summaries))
	for _, summary := range summaries {
		status := summary.Status
		if status == "" {
			status = "unknown"
		}
		done, total, next := summary.Progress()
		plans = append(plans, overviewPlanJSON{
			Name:        summary.Name,
			Directory:   filepath.ToSlash(summary.Label),
			Status:      status,
			DonePhases:  done,
			TotalPhases: total,
			NextPhase:   next,
			Wait:        append([]string{}, summary.Wait...),
		})
	}
	return overviewJSONOutput{Plans: plans}
}

func makeNotesJSON(notes []notes.Note) notesJSONOutput {
	values := make([]noteJSON, 0, len(notes))
	for _, note := range notes {
		values = append(values, noteJSON{
			CompletedAt: note.At,
			Plan:        note.Plan,
			Event:       note.Event,
			Phase:       note.Phase,
			Commit:      note.Commit,
			ShortCommit: note.ShortHash,
			Subject:     note.Subject,
		})
	}
	return notesJSONOutput{Notes: values}
}

func makeShowJSON(phase plan.PhaseDetails) showJSONOutput {
	return showJSONOutput{
		Plan:        phase.Plan,
		Directory:   phase.Directory,
		PhaseNumber: phase.ID,
		Slug:        phase.Slug,
		Title:       phase.Title,
		Status:      phase.Status,
		PlannedWork: phase.PlannedWork,
		DoneWhen:    phase.DoneWhen,
		DependsOn:   append([]string{}, phase.Dependencies...),
		File:        phase.File,
	}
}

func makeTemplateJSON(kind, selector, template string) templateJSONOutput {
	return templateJSONOutput{Kind: kind, Selector: selector, Template: template}
}

func makeApplyJSON(operation applyOperation) applyJSONOutput {
	documents := operation.documents
	if documents == nil {
		documents = map[string]string{}
	}
	diffs := operation.diffs
	if diffs == nil {
		diffs = []applyDiffJSON{}
	}
	return applyJSONOutput{
		Ok:        true,
		Action:    operation.action,
		Selector:  operation.selector,
		DryRun:    operation.dryRun,
		Changed:   operation.changed,
		Documents: documents,
		Diff:      diffs,
	}
}

func makeValidationJSON(records []validation.Record) []validationErrorJSON {
	result := make([]validationErrorJSON, 0, len(records))
	for _, record := range records {
		result = append(result, validationErrorJSON{
			Rule:    record.Rule,
			Section: record.Section,
			Phase:   record.Phase,
			Line:    record.Line,
			Phases:  append([]int{}, record.Phases...),
			Detail:  record.Detail,
		})
	}
	return result
}

func makeInitJSON(target, root, language string, plansDirs, created, existed []string) initJSONOutput {
	return initJSONOutput{
		ConfigFile:     target,
		RepositoryRoot: root,
		Language:       language,
		PlansDirs:      append([]string{}, plansDirs...),
		// Non-nil slices: `created` and `existed` encode as [] rather than
		// null, so a consumer can iterate them without a nil check.
		Created: append([]string{}, created...),
		Existed: append([]string{}, existed...),
	}
}

func makeConfigJSON(settings config.Config, root string) configJSONOutput {
	var configFile *string
	if settings.Path != "" {
		value := settings.Path
		configFile = &value
	}
	return configJSONOutput{
		ConfigFile:     configFile,
		RepositoryRoot: root,
		Agent:          agentenv.CurrentDescription(),
		Language:       settings.Language,
		PlansDirs:      append([]string{}, settings.PlansDirs...),
		Ignore:         append([]string{}, settings.Ignore...),
		Hooks: configHooksJSON{
			Timeout: settings.Hooks.TimeoutDuration().String(),
			Before:  makeHookRulesJSON(settings.Hooks.Before),
			After:   makeHookRulesJSON(settings.Hooks.After),
		},
	}
}

func makeHookRulesJSON(rules []hooks.Rule) []hookRuleJSON {
	result := make([]hookRuleJSON, 0, len(rules))
	for _, rule := range rules {
		result = append(result, hookRuleJSON{On: append([]string{}, rule.On...), Run: rule.Run})
	}
	return result
}

func makeDoctorJSON(issues []doctor.Issue) doctorJSONOutput {
	result := make([]doctorIssueJSON, 0, len(issues))
	for _, issue := range issues {
		result = append(result, doctorIssueJSON{Location: issue.Location, Message: issue.Message})
	}
	return doctorJSONOutput{Issues: result}
}
