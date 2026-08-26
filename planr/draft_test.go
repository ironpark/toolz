package main

import (
	"fmt"
	"strings"
	"testing"
)

func phaseForTest(id int, dependsOn ...int) draftPhase {
	return draftPhase{
		Title: fmt.Sprintf("Phase %d", id),
		Meta: phaseMeta{
			Phase:     id,
			Slug:      fmt.Sprintf("phase-%d", id),
			Status:    "planned",
			DependsOn: dependsOn,
		},
		Planned:    "Implement the phase.",
		Completion: "The phase is verified.",
	}
}

func TestValidatePhaseDependencies(t *testing.T) {
	tests := []struct {
		name        string
		phases      []draftPhase
		wantErrText string
	}{
		{
			name:   "allows dependencies on other phases",
			phases: []draftPhase{phaseForTest(0), phaseForTest(1, 0), phaseForTest(2, 0, 1)},
		},
		{
			name:        "rejects self dependency",
			phases:      []draftPhase{phaseForTest(0, 0)},
			wantErrText: "cannot depend on itself",
		},
		{
			name:        "rejects undefined dependency",
			phases:      []draftPhase{phaseForTest(0), phaseForTest(1, 9)},
			wantErrText: "phase 9 is not defined",
		},
		{
			name:        "rejects duplicate dependency",
			phases:      []draftPhase{phaseForTest(0), phaseForTest(1, 0, 0)},
			wantErrText: "more than once",
		},
		{
			name:        "rejects dependency cycle",
			phases:      []draftPhase{phaseForTest(0, 1), phaseForTest(1, 0)},
			wantErrText: "cycle detected",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePhaseDependencies(test.phases)
			if test.wantErrText == "" {
				if err != nil {
					t.Fatalf("validatePhaseDependencies() unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErrText) {
				t.Fatalf("validatePhaseDependencies() error = %v, want text %q", err, test.wantErrText)
			}
		})
	}
}
