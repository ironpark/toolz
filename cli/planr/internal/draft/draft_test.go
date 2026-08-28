package draft_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/jsonout"
	"github.com/ironpark/toolz/cli/planr/internal/validation"
)

func phaseForTest(id int, dependsOn ...int) draft.Phase {
	return draft.Phase{
		Title: fmt.Sprintf("Phase %d", id),
		Meta: draft.Meta{
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
		phases      []draft.Phase
		wantErrText string
	}{
		{
			name:   "allows dependencies on other phases",
			phases: []draft.Phase{phaseForTest(0), phaseForTest(1, 0), phaseForTest(2, 0, 1)},
		},
		{
			name:        "rejects self dependency",
			phases:      []draft.Phase{phaseForTest(0, 0)},
			wantErrText: "cannot depend on itself",
		},
		{
			name:        "rejects undefined dependency",
			phases:      []draft.Phase{phaseForTest(0), phaseForTest(1, 9)},
			wantErrText: "phase 9 is not defined",
		},
		{
			name:        "rejects duplicate dependency",
			phases:      []draft.Phase{phaseForTest(0), phaseForTest(1, 0, 0)},
			wantErrText: "more than once",
		},
		{
			name:        "rejects dependency cycle",
			phases:      []draft.Phase{phaseForTest(0, 1), phaseForTest(1, 0)},
			wantErrText: "cycle detected",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := draft.ValidatePhaseDependencies(test.phases)
			if test.wantErrText == "" {
				if err != nil {
					t.Fatalf("draft.ValidatePhaseDependencies() unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErrText) {
				t.Fatalf("draft.ValidatePhaseDependencies() error = %v, want text %q", err, test.wantErrText)
			}
		})
	}
}

// A draft produced by `planr new` must be registrable by `planr apply` once the
// author fills in its placeholders, and must report every unfilled placeholder
// at once rather than one failed application at a time.
func newDraftForTest(t *testing.T, language string) string {
	t.Helper()
	raw, err := doc.RenderNewDraft(language, "demo", nil, "a demo plan")
	if err != nil {
		t.Fatalf("doc.RenderNewDraft(%q) unexpected error: %v", language, err)
	}
	return raw
}

func TestNewDraftReportsEveryPlaceholderAtOnce(t *testing.T) {
	for _, language := range doc.SortedLanguages() {
		t.Run(language, func(t *testing.T) {
			_, err := draft.Parse([]byte(newDraftForTest(t, language)), "demo.md")
			if err == nil {
				t.Fatal("draft.Parse() accepted an unfilled draft")
			}
			message := err.Error()
			if got := strings.Count(message, "\n  line "); got != 3 {
				t.Fatalf("draft.Parse() reported %d placeholders, want 3; error: %v", got, err)
			}
		})
	}
}

// A caller that never opens the document only learns what is wrong from the
// error, so a section mismatch must name the offending sections rather than
// restating what a correct draft looks like.
func TestSectionMismatchNamesTheOffendingSections(t *testing.T) {
	raw := newDraftForTest(t, doc.English)
	lines := []string{}
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "# CONTEXT") {
			continue
		}
		if strings.Contains(line, draft.Placeholder) {
			line = "- filled in"
		}
		lines = append(lines, line)
	}
	_, err := draft.Parse([]byte(strings.Join(lines, "\n")), "demo.md")
	if err == nil {
		t.Fatal("draft.Parse() accepted a draft with a missing section")
	}
	for _, want := range []string{"missing section(s): CONTEXT", "found sections:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	records := validation.Records(err)
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
		{name: "unexpected", found: append(append([]string{}, draft.RequiredSections...), "EXTRA"), want: "unexpected section(s): EXTRA"},
		{name: "duplicated", found: append(append([]string{}, draft.RequiredSections...), "GOALS"), want: "duplicated section(s): GOALS"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := draft.DescribeSectionMismatch(testCase.found); !strings.Contains(got, testCase.want) {
				t.Fatalf("draft.DescribeSectionMismatch() = %q, want it to contain %q", got, testCase.want)
			}
		})
	}
}

// Every language's skeleton must parse, so a draft written against one
// language is not silently unusable under another.
func TestNewDraftRoundTripsInEveryLanguage(t *testing.T) {
	for _, language := range doc.SortedLanguages() {
		t.Run(language, func(t *testing.T) {
			raw := newDraftForTest(t, language)
			lines := strings.Split(raw, "\n")
			for index, line := range lines {
				if strings.Contains(line, draft.Placeholder) {
					lines[index] = "- filled in"
				}
			}
			if _, err := draft.Parse([]byte(strings.Join(lines, "\n")), "demo.md"); err != nil {
				t.Fatalf("draft.Parse() rejected a filled-in %s draft: %v", language, err)
			}
		})
	}
}

func TestNewDraftRoundTripsOnceFilledIn(t *testing.T) {
	raw := newDraftForTest(t, doc.Korean)
	lines := strings.Split(raw, "\n")
	for index, line := range lines {
		if strings.Contains(line, draft.Placeholder) {
			lines[index] = "- filled in"
		}
	}
	parsed, err := draft.Parse([]byte(strings.Join(lines, "\n")), "demo.md")
	if err != nil {
		t.Fatalf("draft.Parse() rejected a filled-in draft: %v", err)
	}
	if parsed.Name != "demo" || len(parsed.Phases) != 1 {
		t.Fatalf("draft.Parse() = %+v, want plan demo with one phase", parsed)
	}
}

func TestPlaceholderGuidanceInCommentIsNotAPlaceholder(t *testing.T) {
	raw := "<!-- fill in every " + draft.Placeholder + " line -->\nreal content\n"
	if err := draft.CheckPlaceholders(raw); err != nil {
		t.Fatalf("draft.CheckPlaceholders() flagged commented guidance: %v", err)
	}
}

func TestPhaseDependsOnAcceptsNumbersAndSlugs(t *testing.T) {
	phases := []draft.Phase{
		{Title: "First", Meta: draft.Meta{Phase: 0, Slug: "first"}},
		{Title: "Second", Meta: draft.Meta{Phase: 1, Slug: "second", DependsOnRefs: []draft.Ref{{Slug: "first"}}}},
		{Title: "Third", Meta: draft.Meta{Phase: 2, Slug: "third", DependsOnRefs: []draft.Ref{{Number: intPointer(0)}, {Slug: "second"}}}},
	}
	if err := draft.ResolveRefs(phases); err != nil {
		t.Fatalf("draft.ResolveRefs() unexpected error: %v", err)
	}
	if got := phases[1].Meta.DependsOn; len(got) != 1 || got[0] != 0 {
		t.Fatalf("slug dependency resolved to %v, want [0]", got)
	}
	if got := phases[2].Meta.DependsOn; len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("mixed dependencies resolved to %v, want [0 1]", got)
	}
}

func TestPhaseDependsOnUnknownSlugListsAvailableSlugs(t *testing.T) {
	phases := []draft.Phase{
		{Title: "First", Meta: draft.Meta{Phase: 0, Slug: "first"}},
		{Title: "Second", Meta: draft.Meta{Phase: 1, Slug: "second", DependsOnRefs: []draft.Ref{{Slug: "frist"}}}},
	}
	err := draft.ResolveRefs(phases)
	if err == nil {
		t.Fatal("draft.ResolveRefs() accepted an unknown slug")
	}
	for _, want := range []string{"frist", "first", "second"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("draft.ResolveRefs() error = %v, want it to mention %q", err, want)
		}
	}
}

func intPointer(value int) *int { return &value }

func TestStructuredValidationIncludesPlaceholderLocationAndCycle(t *testing.T) {
	raw := renderPlaceholderDraftForTest(t)
	if err := draft.CheckPlaceholders(raw); err == nil {
		t.Fatal("placeholder draft unexpectedly passed")
	} else {
		records := validation.Records(err)
		if len(records) != 3 || records[0].Rule != "placeholder" || records[0].Section != "PHASES" || records[0].Phase == nil || records[0].Line == 0 {
			t.Fatalf("placeholder records = %#v", records)
		}
		encoded := jsonout.Validation(records)
		if encoded[0].Rule != "placeholder" || encoded[0].Phase == nil || encoded[0].Line == 0 {
			t.Fatalf("placeholder JSON = %#v", encoded[0])
		}
	}

	err := draft.ValidatePhaseDependencies([]draft.Phase{phaseForTest(1, 3), phaseForTest(3, 1)})
	if err == nil {
		t.Fatal("cycle unexpectedly passed")
	}
	records := validation.Records(err)
	if len(records) != 1 || records[0].Rule != "dependency_cycle" || len(records[0].Phases) != 2 {
		t.Fatalf("cycle records = %#v", records)
	}
}

func renderPlaceholderDraftForTest(t *testing.T) string {
	t.Helper()
	raw, err := doc.RenderNewDraft(doc.English, "demo", nil, "a demo plan")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
