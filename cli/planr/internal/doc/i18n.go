package doc

import (
	"fmt"
	"sort"
	"strings"
)

// planr writes plan documents in the language configured by `language` in
// .planr.yaml. Only document text is localized: command output, flags, and
// error messages stay in English so scripts and hooks read the same everywhere.
const (
	English         = "en"
	Korean          = "ko"
	DefaultLanguage = English
)

// Strings is the set of headings and markers planr writes into a plan.
// Both variants must stay parseable by the draft parser in internal/draft, so
// any heading added here also belongs in PhaseSectionHeadings.
type Strings struct {
	// Template is the embedded draft skeleton emitted by `planr new`.
	Template string
	// PhaseTemplate is the embedded phase draft emitted by `planr new plan#phase`.
	PhaseTemplate string
	// PlannedWork and DoneWhen title the two required blocks of a phase.
	PlannedWork, DoneWhen string
	// NoNext marks a phase document that does not point at a follow-up.
	NoNext string
	// Verification, Ordering, and NextTarget title the PLAN.md body sections.
	Verification, Ordering, NextTarget string
}

var languageStrings = map[string]Strings{
	English: {
		Template:      "draft.en.md.tmpl",
		PhaseTemplate: "phase.en.md.tmpl",
		PlannedWork:   "Planned Work",
		DoneWhen:      "Done When",
		NoNext:        "none",
		Verification:  "Shared Verification",
		Ordering:      "Decisions That Constrain Ordering",
		NextTarget:    "Next Implementation Target",
	},
	Korean: {
		Template:      "draft.ko.md.tmpl",
		PhaseTemplate: "phase.ko.md.tmpl",
		PlannedWork:   "계획된 작업",
		DoneWhen:      "완료 조건",
		NoNext:        "없음",
		Verification:  "공통 검증",
		Ordering:      "구현 순서를 제한하는 결정",
		NextTarget:    "다음 구현 대상",
	},
}

// PhaseSectionHeadings pairs the planned-work and done-when headings of every
// supported language. A draft is accepted when it uses any one pair, regardless
// of the configured language: a plan written by someone else must still parse.
func PhaseSectionHeadings() [][2]string {
	pairs := make([][2]string, 0, len(languageStrings))
	for _, language := range SortedLanguages() {
		set := languageStrings[language]
		pairs = append(pairs, [2]string{set.PlannedWork, set.DoneWhen})
	}
	return pairs
}

func StringsFor(language string) Strings {
	if set, ok := languageStrings[NormalizeLanguage(language)]; ok {
		return set
	}
	return languageStrings[DefaultLanguage]
}

func NormalizeLanguage(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return DefaultLanguage
	}
	return language
}

func ValidateLanguage(language string) error {
	if _, ok := languageStrings[NormalizeLanguage(language)]; !ok {
		return fmt.Errorf("language %q is not supported; use one of: %s", language, strings.Join(SortedLanguages(), ", "))
	}
	return nil
}

func SortedLanguages() []string {
	languages := make([]string, 0, len(languageStrings))
	for language := range languageStrings {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages
}
