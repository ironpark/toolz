package main

import (
	"strings"
	"testing"
)

func TestSplitPhaseDocumentSectionsUsesConfiguredLanguageTable(t *testing.T) {
	for _, language := range sortedLanguages() {
		t.Run(language, func(t *testing.T) {
			body := phaseDocumentBody(language, "API Contract", "- add the contract", "- contract tests pass")
			planned, done, err := splitPhaseDocumentSections("API Contract", body)
			if err != nil {
				t.Fatalf("splitPhaseDocumentSections() unexpected error: %v", err)
			}
			if planned != "- add the contract" || done != "- contract tests pass" {
				t.Fatalf("sections = %q / %q", planned, done)
			}
		})
	}
}

func TestSplitPhaseDocumentSectionsRejectsMissingSection(t *testing.T) {
	body := "> NEXT: none\n\n# API Contract\n\n## " + documentStringsFor(languageEnglish).plannedWork + "\n\nwork\n"
	_, _, err := splitPhaseDocumentSections("API Contract", body)
	if err == nil || !strings.Contains(err.Error(), "Done When") {
		t.Fatalf("splitPhaseDocumentSections() error = %v, want missing Done When", err)
	}
}

func TestSplitPhaseDocumentSectionsIgnoresMarkdownHeadingsInWorkBody(t *testing.T) {
	body := phaseDocumentBody(languageEnglish, "API Contract", "work\n\n## Implementation detail\n\nmore work", "done")
	planned, done, err := splitPhaseDocumentSections("API Contract", body)
	if err != nil {
		t.Fatalf("splitPhaseDocumentSections() unexpected error: %v", err)
	}
	if planned != "work\n\n## Implementation detail\n\nmore work" || done != "done" {
		t.Fatalf("sections = %q / %q", planned, done)
	}
}
