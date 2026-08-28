package jsonout

import (
	"github.com/ironpark/toolz/cli/planr/internal/apply"
	"github.com/ironpark/toolz/cli/planr/internal/validation"
)

type ApplyOutput struct {
	Ok        bool              `json:"ok"`
	Action    string            `json:"action"`
	Selector  string            `json:"selector"`
	DryRun    bool              `json:"dry_run"`
	Changed   bool              `json:"changed"`
	Documents map[string]string `json:"documents"`
	Diff      []applyDiff       `json:"diff"`
}

type applyDiff struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type ApplyFailureOutput struct {
	Ok     bool              `json:"ok"`
	Errors []ValidationError `json:"errors"`
}

type ValidationError struct {
	Rule    string `json:"rule"`
	Section string `json:"section,omitempty"`
	Phase   *int   `json:"phase,omitempty"`
	Line    int    `json:"line,omitempty"`
	Phases  []int  `json:"phases,omitempty"`
	Detail  string `json:"detail"`
}

func Apply(operation apply.Operation) ApplyOutput {
	documents := operation.Documents
	if documents == nil {
		documents = map[string]string{}
	}
	diffs := make([]applyDiff, 0, len(operation.Diffs))
	for _, diff := range operation.Diffs {
		diffs = append(diffs, applyDiff{Path: diff.Path, Before: diff.Before, After: diff.After})
	}
	return ApplyOutput{
		Ok:        true,
		Action:    operation.Action,
		Selector:  operation.Selector,
		DryRun:    operation.DryRun,
		Changed:   operation.Changed,
		Documents: documents,
		Diff:      diffs,
	}
}

func Validation(records []validation.Record) []ValidationError {
	result := make([]ValidationError, 0, len(records))
	for _, record := range records {
		result = append(result, ValidationError{
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
