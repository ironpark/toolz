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

// A draft produced by `planr new` must be registrable by `planr add` once the
// author fills in its placeholders, and must report every unfilled placeholder
// at once rather than one failed `add` at a time.
func newDraftForTest(t *testing.T) string {
	t.Helper()
	raw, err := renderNewDraft("demo", nil, "a demo plan")
	if err != nil {
		t.Fatalf("renderNewDraft() unexpected error: %v", err)
	}
	return raw
}

func TestNewDraftReportsEveryPlaceholderAtOnce(t *testing.T) {
	_, err := parseDraft([]byte(newDraftForTest(t)), "demo.md")
	if err == nil {
		t.Fatal("parseDraft() accepted an unfilled draft")
	}
	message := err.Error()
	if got := strings.Count(message, "\n  line "); got != 3 {
		t.Fatalf("parseDraft() reported %d placeholders, want 3; error: %v", got, err)
	}
}

func TestNewDraftRoundTripsOnceFilledIn(t *testing.T) {
	raw := newDraftForTest(t)
	lines := strings.Split(raw, "\n")
	for index, line := range lines {
		if strings.Contains(line, draftPlaceholder) {
			lines[index] = "- filled in"
		}
	}
	parsed, err := parseDraft([]byte(strings.Join(lines, "\n")), "demo.md")
	if err != nil {
		t.Fatalf("parseDraft() rejected a filled-in draft: %v", err)
	}
	if parsed.Name != "demo" || len(parsed.Phases) != 1 {
		t.Fatalf("parseDraft() = %+v, want plan demo with one phase", parsed)
	}
}

func TestPlaceholderGuidanceInCommentIsNotAPlaceholder(t *testing.T) {
	raw := "<!-- fill in every " + draftPlaceholder + " line -->\nreal content\n"
	if err := checkDraftPlaceholders(raw); err != nil {
		t.Fatalf("checkDraftPlaceholders() flagged commented guidance: %v", err)
	}
}

func TestPhaseDependsOnAcceptsNumbersAndSlugs(t *testing.T) {
	phases := []draftPhase{
		{Title: "First", Meta: phaseMeta{Phase: 0, Slug: "first"}},
		{Title: "Second", Meta: phaseMeta{Phase: 1, Slug: "second", DependsOnRefs: []phaseRef{{slug: "first"}}}},
		{Title: "Third", Meta: phaseMeta{Phase: 2, Slug: "third", DependsOnRefs: []phaseRef{{number: intPointer(0)}, {slug: "second"}}}},
	}
	if err := resolvePhaseRefs(phases); err != nil {
		t.Fatalf("resolvePhaseRefs() unexpected error: %v", err)
	}
	if got := phases[1].Meta.DependsOn; len(got) != 1 || got[0] != 0 {
		t.Fatalf("slug dependency resolved to %v, want [0]", got)
	}
	if got := phases[2].Meta.DependsOn; len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("mixed dependencies resolved to %v, want [0 1]", got)
	}
}

func TestPhaseDependsOnUnknownSlugListsAvailableSlugs(t *testing.T) {
	phases := []draftPhase{
		{Title: "First", Meta: phaseMeta{Phase: 0, Slug: "first"}},
		{Title: "Second", Meta: phaseMeta{Phase: 1, Slug: "second", DependsOnRefs: []phaseRef{{slug: "frist"}}}},
	}
	err := resolvePhaseRefs(phases)
	if err == nil {
		t.Fatal("resolvePhaseRefs() accepted an unknown slug")
	}
	for _, want := range []string{"frist", "first", "second"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("resolvePhaseRefs() error = %v, want it to mention %q", err, want)
		}
	}
}

func intPointer(value int) *int { return &value }
