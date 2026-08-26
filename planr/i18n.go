package main

import (
	"fmt"
	"sort"
	"strings"
)

// planr writes plan documents in the language configured by `language` in
// .planr.yaml. Only document text is localized: command output, flags, and
// error messages stay in English so scripts and hooks read the same everywhere.
const (
	languageEnglish = "en"
	languageKorean  = "ko"
	defaultLanguage = languageEnglish
)

// documentStrings is the set of headings and markers planr writes into a plan.
// Both variants must stay parseable by parsePhases, so any heading added here
// also belongs in phaseSectionHeadings.
type documentStrings struct {
	// template is the embedded draft skeleton emitted by `planr new`.
	template string
	// plannedWork and doneWhen title the two required blocks of a phase.
	plannedWork, doneWhen string
	// noNext marks a phase document that does not point at a follow-up.
	noNext string
	// verification, ordering, and nextTarget title the PLAN.md body sections.
	verification, ordering, nextTarget string
}

var documentLanguages = map[string]documentStrings{
	languageEnglish: {
		template:     "draft.en.md.tmpl",
		plannedWork:  "Planned Work",
		doneWhen:     "Done When",
		noNext:       "none",
		verification: "Shared Verification",
		ordering:     "Decisions That Constrain Ordering",
		nextTarget:   "Next Implementation Target",
	},
	languageKorean: {
		template:     "draft.ko.md.tmpl",
		plannedWork:  "계획된 작업",
		doneWhen:     "완료 조건",
		noNext:       "없음",
		verification: "공통 검증",
		ordering:     "구현 순서를 제한하는 결정",
		nextTarget:   "다음 구현 대상",
	},
}

// phaseSectionHeadings pairs the planned-work and done-when headings of every
// supported language. A draft is accepted when it uses any one pair, regardless
// of the configured language: a plan written by someone else must still parse.
func phaseSectionHeadings() [][2]string {
	pairs := make([][2]string, 0, len(documentLanguages))
	for _, language := range sortedLanguages() {
		set := documentLanguages[language]
		pairs = append(pairs, [2]string{set.plannedWork, set.doneWhen})
	}
	return pairs
}

func documentStringsFor(language string) documentStrings {
	if set, ok := documentLanguages[normalizeLanguage(language)]; ok {
		return set
	}
	return documentLanguages[defaultLanguage]
}

func normalizeLanguage(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return defaultLanguage
	}
	return language
}

func validateLanguage(language string) error {
	if _, ok := documentLanguages[normalizeLanguage(language)]; !ok {
		return fmt.Errorf("language %q is not supported; use one of: %s", language, strings.Join(sortedLanguages(), ", "))
	}
	return nil
}

func sortedLanguages() []string {
	languages := make([]string, 0, len(documentLanguages))
	for language := range documentLanguages {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages
}
