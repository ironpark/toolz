package jsonout

import (
	"github.com/ironpark/toolz/cli/planr/internal/apply"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
)

type ShowOutput struct {
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

type ShowSectionOutput struct {
	Plan      string `json:"plan"`
	Directory string `json:"directory"`
	Section   string `json:"section"`
	Content   string `json:"content"`
	File      string `json:"file"`
}

type ShowAllOutput struct {
	Plan         string            `json:"plan"`
	Directory    string            `json:"directory"`
	Status       string            `json:"status"`
	Description  string            `json:"description"`
	DependsOn    []string          `json:"depends_on"`
	Goals        string            `json:"goals"`
	Context      string            `json:"context"`
	PlanDocument string            `json:"plan_document"`
	Phases       []ShowOutput      `json:"phases"`
	Documents    map[string]string `json:"documents"`
}

type TemplateOutput struct {
	Kind     string `json:"kind"`
	Selector string `json:"selector"`
	Template string `json:"template"`
}

type EditOutput struct {
	Kind     string `json:"kind"`
	Selector string `json:"selector"`
	Section  string `json:"section,omitempty"`
	Target   string `json:"target"`
	Base     string `json:"base"`
	Document string `json:"document"`
}

func Show(phase plan.PhaseDetails) ShowOutput {
	return ShowOutput{
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

func ShowSection(details plan.SectionDetails) ShowSectionOutput {
	return ShowSectionOutput{
		Plan:      details.Plan,
		Directory: details.Directory,
		Section:   details.Section,
		Content:   details.Content,
		File:      details.File,
	}
}

func Edit(checkout apply.CheckoutDocument) EditOutput {
	return EditOutput{
		Kind:     checkout.Kind,
		Selector: checkout.Selector,
		Section:  checkout.Section,
		Target:   checkout.Target,
		Base:     checkout.Base,
		Document: checkout.Document,
	}
}

func Template(kind, selector, template string) TemplateOutput {
	return TemplateOutput{Kind: kind, Selector: selector, Template: template}
}

func ShowAll(details plan.Details) ShowAllOutput {
	phases := make([]ShowOutput, 0, len(details.Phases))
	for _, phase := range details.Phases {
		phases = append(phases, Show(phase))
	}
	return ShowAllOutput{
		Plan:         details.Plan,
		Directory:    details.Directory,
		Status:       details.Status,
		Description:  details.Description,
		DependsOn:    details.DependsOn,
		Goals:        details.Documents["GOALS.md"],
		Context:      details.Documents["CONTEXT.md"],
		PlanDocument: details.Documents["PLAN.md"],
		Phases:       phases,
		Documents:    details.Documents,
	}
}
