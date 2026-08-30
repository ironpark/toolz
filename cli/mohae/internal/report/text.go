package report

import (
	"strings"
	"unicode"
)

func verdictWord(passed bool) string {
	if passed {
		return "pass"
	}
	return "fail"
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit-1] + "…"
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "trial"
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)
}
