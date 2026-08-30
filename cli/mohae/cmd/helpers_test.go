package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ironpark/toolz/cli/mohae/internal/config"
	"github.com/ironpark/toolz/cli/mohae/internal/report"
	"github.com/ironpark/toolz/cli/mohae/internal/runner"
	"github.com/ironpark/toolz/cli/mohae/internal/scaffold"
)

type Config = config.Config
type Prompt = config.Prompt
type ReportConfig = config.ReportConfig
type TrialResult = runner.TrialResult
type ReportOptions = report.ReportOptions

const DefaultConfigName = config.DefaultConfigName

var Templates = scaffold.Templates
var LoadConfig = config.LoadConfig

type reportDocument struct {
	Passed int `json:"passed"`
	Total  int `json:"total"`
}

const minimalConfig = `name: sample
agent:
  type: codex
workspace:
  source: ./fixture
prompts:
  - file: ./PROMPT.md
`

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func stubAgent(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub-agent")
	writeFile(t, path, "#!/bin/sh\n"+script, 0o755)
	return path
}
