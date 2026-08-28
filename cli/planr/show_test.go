package main

import (
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
)

func TestSplitPhaseDocumentSectionsUsesConfiguredLanguageTable(t *testing.T) {
	for _, language := range doc.SortedLanguages() {
		t.Run(language, func(t *testing.T) {
			body := phaseDocumentBody(language, "API Contract", "- add the contract", "- contract tests pass")
			planned, done, err := draft.SplitPhaseDocumentSections("API Contract", body)
			if err != nil {
				t.Fatalf("draft.SplitPhaseDocumentSections() unexpected error: %v", err)
			}
			if planned != "- add the contract" || done != "- contract tests pass" {
				t.Fatalf("sections = %q / %q", planned, done)
			}
		})
	}
}

func TestSplitPhaseDocumentSectionsRejectsMissingSection(t *testing.T) {
	body := "> NEXT: none\n\n# API Contract\n\n## " + doc.StringsFor(doc.English).PlannedWork + "\n\nwork\n"
	_, _, err := draft.SplitPhaseDocumentSections("API Contract", body)
	if err == nil || !strings.Contains(err.Error(), "Done When") {
		t.Fatalf("draft.SplitPhaseDocumentSections() error = %v, want missing Done When", err)
	}
}

func TestSplitPhaseDocumentSectionsIgnoresMarkdownHeadingsInWorkBody(t *testing.T) {
	body := phaseDocumentBody(doc.English, "API Contract", "work\n\n## Implementation detail\n\nmore work", "done")
	planned, done, err := draft.SplitPhaseDocumentSections("API Contract", body)
	if err != nil {
		t.Fatalf("draft.SplitPhaseDocumentSections() unexpected error: %v", err)
	}
	if planned != "work\n\n## Implementation detail\n\nmore work" || done != "done" {
		t.Fatalf("sections = %q / %q", planned, done)
	}
}
