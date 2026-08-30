package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalConfig = `name: sample
agent:
  type: codex
workspace:
  source: ./fixture
prompts:
  - file: ./PROMPT.md
`

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), DefaultConfigName)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigAppliesDefaults(t *testing.T) {
	config, err := LoadConfig(writeConfig(t, minimalConfig))
	if err != nil {
		t.Fatal(err)
	}
	if config.Limits.TimeoutSeconds != DefaultTimeoutSeconds {
		t.Errorf("limits not defaulted: %+v", config.Limits)
	}
	if config.Report.Dir != DefaultReportDir {
		t.Errorf("report dir = %q", config.Report.Dir)
	}
}

func TestLoadConfigNamesItselfAfterTheFile(t *testing.T) {
	// An unnamed config still has to be distinguishable in a report.
	config, err := LoadConfig(writeConfig(t, strings.Replace(minimalConfig, "name: sample\n", "", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != "mohae" {
		t.Errorf("name = %q, want the file's own name", config.Name)
	}
}

func TestLoadConfigRejectsUnknownKeys(t *testing.T) {
	// A misspelled key would otherwise be discovered as a trial that silently
	// measured the defaults.
	_, err := LoadConfig(writeConfig(t, minimalConfig+"workspce: typo\n"))
	if err == nil {
		t.Fatal("expected an error for an unknown key")
	}
}

func TestLoadConfigReadsWorkspaceExcludesAndArtifacts(t *testing.T) {
	contents := strings.Replace(minimalConfig, "workspace:\n  source: ./fixture\n", `workspace:
  source: ./fixture
  exclude: [FIXTURE.*, tmp/**]
`, 1)
	config, err := LoadConfig(writeConfig(t, contents+`artifacts:
  - plans/**
  - .harness/events.log
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Workspace.Exclude) != 2 || config.Workspace.Exclude[0] != "FIXTURE.*" {
		t.Errorf("workspace.exclude = %v", config.Workspace.Exclude)
	}
	if len(config.Artifacts) != 2 || config.Artifacts[0] != "plans/**" {
		t.Errorf("artifacts = %v", config.Artifacts)
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("valid patterns were rejected: %v", err)
	}
}

func TestLoadConfigReadsAfterHooks(t *testing.T) {
	config, err := LoadConfig(writeConfig(t, minimalConfig+`hooks:
  after:
    - gofmt -w .
    - run: ./collect-state.sh
      scope: outside
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Hooks.After) != 2 || config.Hooks.After[0].Run != "gofmt -w ." ||
		config.Hooks.After[1].Run != "./collect-state.sh" || config.Hooks.After[1].Scope != HookScopeOutside {
		t.Errorf("hooks.after = %v", config.Hooks.After)
	}
}

func TestValidateRejectsUnsafeOrMalformedWorkspacePatterns(t *testing.T) {
	for name, section := range map[string]string{
		"absolute exclude": "workspace:\n  source: ./fixture\n  exclude: [/private/data]\n",
		"parent artifact":  "artifacts: [../outside]\n",
		"broken artifact":  "artifacts: ['logs/[']\n",
		"empty exclude":    "workspace:\n  source: ./fixture\n  exclude: ['']\n",
	} {
		t.Run(name, func(t *testing.T) {
			contents := minimalConfig
			if strings.HasPrefix(section, "workspace:") {
				contents = strings.Replace(contents, "workspace:\n  source: ./fixture\n", section, 1)
			} else {
				contents += section
			}
			if _, err := LoadConfig(writeConfig(t, contents)); err == nil {
				t.Fatal("expected the unsafe pattern to be rejected")
			}
		})
	}
}

func TestValidateRejectsIncompleteConfigurations(t *testing.T) {
	cases := map[string]string{
		"missing agent type":  strings.Replace(minimalConfig, "  type: codex\n", "", 1),
		"unknown agent type":  strings.Replace(minimalConfig, "codex", "not-an-agent", 1),
		"missing workspace":   strings.Replace(minimalConfig, "  source: ./fixture\n", "", 1),
		"missing prompt":      strings.Replace(minimalConfig, "  - file: ./PROMPT.md\n", "", 1),
		"two prompt sources":  minimalConfig + "    text: inline\n",
		"unknown prompt key":  minimalConfig + "    unles: turn > 1\n",
		"broken condition":    minimalConfig + "    when: turn >\n",
		"unknown condition":   minimalConfig + "    when: nonexistent_variable\n",
		"non-boolean when":    minimalConfig + "    when: turn\n",
		"unknown format":      minimalConfig + "report:\n  formats: [smoke-signal]\n",
		"custom without argv": strings.Replace(minimalConfig, "codex", "custom-cli", 1),
		"empty after hook":    minimalConfig + "hooks:\n  after: ['  ']\n",
		"invalid hook scope":  minimalConfig + "hooks:\n  after:\n    - run: true\n      scope: source\n",
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadConfig(writeConfig(t, contents)); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
}

func TestResolveIsRelativeToTheConfigFile(t *testing.T) {
	// Not to the working directory: a config has to survive being run from
	// anywhere.
	path := writeConfig(t, minimalConfig)
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(path), "fixture")
	if got := config.Resolve("./fixture"); got != want {
		t.Errorf("Resolve = %q, want %q", got, want)
	}
	if got := config.Resolve("/tmp/fixed"); got != "/tmp/fixed" {
		t.Errorf("absolute path was rewritten to %q", got)
	}
}

func TestReferencedPathsSkipUnsetFieldsAndAreAbsolute(t *testing.T) {
	config, err := LoadConfig(writeConfig(t, minimalConfig))
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]bool{}
	for _, referenced := range config.ReferencedPaths() {
		if !filepath.IsAbs(referenced.Path) {
			t.Errorf("%s is not absolute: %s", referenced.Field, referenced.Path)
		}
		fields[referenced.Field] = true
	}
	if !fields["workspace.source"] || !fields["prompts[0].file"] {
		t.Errorf("missing referenced paths: %v", fields)
	}
	if fields["verify.script"] {
		t.Error("an unset field was reported as a referenced path")
	}
}

func TestSkillsAndMCPAreScopedPerAgent(t *testing.T) {
	config, err := LoadConfig(writeConfig(t, minimalConfig+`skills:
  - path: ./skills/commit
    agents: [claude-code]
mcp:
  - name: context7
    config: ./mcp.json
`))
	if err != nil {
		t.Fatal(err)
	}
	skill := config.Skills[0]
	if !skill.EnabledFor("claude-code") || skill.EnabledFor("codex") {
		t.Errorf("skill scoping = %+v", skill)
	}
	// An empty agents list means the item applies to every driver, so the
	// common single-agent config never repeats the agent's name.
	server := config.MCP[0]
	if !server.EnabledFor("claude-code") || !server.EnabledFor("codex") {
		t.Errorf("server scoping = %+v", server)
	}

	fields := map[string]bool{}
	for _, referenced := range config.ReferencedPaths() {
		fields[referenced.Field] = true
	}
	if !fields["skills[0].path"] || !fields["mcp[0].config"] {
		t.Errorf("missing referenced paths: %v", fields)
	}
}

func TestSkillsAndMCPRejectUnknownAgentsAndMissingPaths(t *testing.T) {
	for name, section := range map[string]string{
		"unknown skill agent": "skills:\n  - path: ./s\n    agents: [gemini]\n",
		"missing skill path":  "skills:\n  - agents: [codex]\n",
		"unknown mcp agent":   "mcp:\n  - config: ./m.json\n    agents: [gemini]\n",
		"missing mcp config":  "mcp:\n  - name: unnamed\n",
	} {
		if _, err := LoadConfig(writeConfig(t, minimalConfig+section)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
