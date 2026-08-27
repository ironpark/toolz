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

// A draft produced by `planr new` must be registrable by `planr apply` once the
// author fills in its placeholders, and must report every unfilled placeholder
// at once rather than one failed application at a time.
func newDraftForTest(t *testing.T, language string) string {
	t.Helper()
	raw, err := renderNewDraft(language, "demo", nil, "a demo plan")
	if err != nil {
		t.Fatalf("renderNewDraft(%q) unexpected error: %v", language, err)
	}
	return raw
}

func TestNewDraftReportsEveryPlaceholderAtOnce(t *testing.T) {
	for _, language := range sortedLanguages() {
		t.Run(language, func(t *testing.T) {
			_, err := parseDraft([]byte(newDraftForTest(t, language)), "demo.md")
			if err == nil {
				t.Fatal("parseDraft() accepted an unfilled draft")
			}
			message := err.Error()
			if got := strings.Count(message, "\n  line "); got != 3 {
				t.Fatalf("parseDraft() reported %d placeholders, want 3; error: %v", got, err)
			}
		})
	}
}

// A caller that never opens the document only learns what is wrong from the
// error, so a section mismatch must name the offending sections rather than
// restating what a correct draft looks like.
func TestSectionMismatchNamesTheOffendingSections(t *testing.T) {
	raw := newDraftForTest(t, languageEnglish)
	lines := []string{}
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "# CONTEXT") {
			continue
		}
		if strings.Contains(line, draftPlaceholder) {
			line = "- filled in"
		}
		lines = append(lines, line)
	}
	_, err := parseDraft([]byte(strings.Join(lines, "\n")), "demo.md")
	if err == nil {
		t.Fatal("parseDraft() accepted a draft with a missing section")
	}
	for _, want := range []string{"missing section(s): CONTEXT", "found sections:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	records := validationRecords(err)
	if len(records) != 1 || records[0].Rule != "sections" || !strings.Contains(records[0].Detail, "CONTEXT") {
		t.Fatalf("validation records = %#v, want one sections record naming CONTEXT", records)
	}
}

func TestDescribeSectionMismatch(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		found []string
		want  string
	}{
		{name: "missing", found: []string{"GOALS", "SCOPE", "PHASES"}, want: "missing section(s): CONTEXT, VERIFICATION, ORDERING, NEXT"},
		{name: "unexpected", found: append(append([]string{}, requiredSections...), "EXTRA"), want: "unexpected section(s): EXTRA"},
		{name: "duplicated", found: append(append([]string{}, requiredSections...), "GOALS"), want: "duplicated section(s): GOALS"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := describeSectionMismatch(testCase.found); !strings.Contains(got, testCase.want) {
				t.Fatalf("describeSectionMismatch() = %q, want it to contain %q", got, testCase.want)
			}
		})
	}
}

// Every language's skeleton must parse, so a draft written against one
// language is not silently unusable under another.
func TestNewDraftRoundTripsInEveryLanguage(t *testing.T) {
	for _, language := range sortedLanguages() {
		t.Run(language, func(t *testing.T) {
			raw := newDraftForTest(t, language)
			lines := strings.Split(raw, "\n")
			for index, line := range lines {
				if strings.Contains(line, draftPlaceholder) {
					lines[index] = "- filled in"
				}
			}
			if _, err := parseDraft([]byte(strings.Join(lines, "\n")), "demo.md"); err != nil {
				t.Fatalf("parseDraft() rejected a filled-in %s draft: %v", language, err)
			}
		})
	}
}

func TestNewDraftRoundTripsOnceFilledIn(t *testing.T) {
	raw := newDraftForTest(t, languageKorean)
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
