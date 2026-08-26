package main

import (
	"bytes"
	"embed"
	"fmt"
	"strconv"
	"strings"
	"text/template"
)

//go:embed templates/draft.md.tmpl
var templateFiles embed.FS

var draftTemplate = template.Must(template.New("draft.md.tmpl").Funcs(template.FuncMap{"join": strings.Join, "quote": strconv.Quote}).ParseFS(templateFiles, "templates/draft.md.tmpl"))

func renderNewDraft(name string, dependsOn []string, descriptions ...string) (string, error) {
	description := ""
	if len(descriptions) > 0 {
		description = descriptions[0]
	}
	var output bytes.Buffer
	if err := draftTemplate.ExecuteTemplate(&output, "draft.md.tmpl", struct {
		Name        string
		Description string
		DependsOn   []string
	}{name, description, dependsOn}); err != nil {
		return "", fmt.Errorf("render draft template: %w", err)
	}
	return output.String(), nil
}
