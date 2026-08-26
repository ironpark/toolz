package main

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed templates/draft.md.tmpl
var templateFiles embed.FS

var draftTemplate = template.Must(template.ParseFS(templateFiles, "templates/draft.md.tmpl"))

func renderNewDraft(name string) (string, error) {
	var output bytes.Buffer
	if err := draftTemplate.ExecuteTemplate(&output, "draft.md.tmpl", struct{ Name string }{name}); err != nil {
		return "", fmt.Errorf("render draft template: %w", err)
	}
	return output.String(), nil
}
