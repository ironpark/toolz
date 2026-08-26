package main

import (
	"bytes"
	"embed"
	"fmt"
	"strconv"
	"strings"
	"text/template"
)

//go:embed templates/*.md.tmpl
var templateFiles embed.FS

var draftTemplates = template.Must(template.New("draft").Funcs(template.FuncMap{"join": strings.Join, "quote": strconv.Quote}).ParseFS(templateFiles, "templates/*.md.tmpl"))

// renderNewDraft emits the draft skeleton for language, which selects which
// embedded template is used. An unknown language falls back to the default.
func renderNewDraft(language, name string, dependsOn []string, descriptions ...string) (string, error) {
	description := ""
	if len(descriptions) > 0 {
		description = descriptions[0]
	}
	var output bytes.Buffer
	if err := draftTemplates.ExecuteTemplate(&output, documentStringsFor(language).template, struct {
		Name        string
		Description string
		DependsOn   []string
	}{name, description, dependsOn}); err != nil {
		return "", fmt.Errorf("render draft template: %w", err)
	}
	return output.String(), nil
}
