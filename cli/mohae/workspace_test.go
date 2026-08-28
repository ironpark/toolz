package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFile writes one file, creating its parents, and fails the test if it
// cannot: a fixture that was not written would show up as a confusing
// assertion failure much later.
func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

// fixtureConfig builds a loadable config rooted at directory, with a source
// tree already in place, so a test only has to say what it is varying.
func fixtureConfig(t *testing.T, directory string) *Config {
	t.Helper()
	writeFile(t, filepath.Join(directory, "fixture", "README.md"), "hello\n", 0o644)
	config := &Config{
		Path:      filepath.Join(directory, DefaultConfigName),
		Name:      "trial",
		Agent:     AgentConfig{Type: "custom-cli", Command: []string{"true"}},
		Workspace: WorkspaceConfig{Source: "./fixture"},
		Prompts:   []Prompt{{Text: "do the thing"}},
	}
	config.applyDefaults()
	return config
}

func TestPrepareWorkspaceCopiesTheSourceAndLeavesItAlone(t *testing.T) {
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	writeFile(t, filepath.Join(directory, "fixture", "nested", "run.sh"), "#!/bin/sh\n", 0o755)

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()

	if data, err := os.ReadFile(filepath.Join(workspace.Root, "README.md")); err != nil || string(data) != "hello\n" {
		t.Fatalf("README.md = %q, %v", data, err)
	}
	// A fixture that builds itself needs its executable bits to survive the copy.
	info, err := os.Stat(filepath.Join(workspace.Root, "nested", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o100 == 0 {
		t.Errorf("nested/run.sh lost its executable bit: %v", info.Mode())
	}
	// The scratch directory has to sit outside the workspace, or verification
	// would leave files that look like the agent's work.
	if strings.HasPrefix(workspace.Scratch, workspace.Root+string(os.PathSeparator)) {
		t.Errorf("scratch %q is inside the workspace %q", workspace.Scratch, workspace.Root)
	}

	// The agent writes in the copy; the source must not change.
	writeFile(t, filepath.Join(workspace.Root, "written-by-agent.txt"), "x", 0o644)
	if _, err := os.Stat(filepath.Join(directory, "fixture", "written-by-agent.txt")); err == nil {
		t.Error("the trial wrote into workspace.source")
	}
}

func TestPrepareWorkspaceSkipsTheSourceGitDirectory(t *testing.T) {
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	writeFile(t, filepath.Join(directory, "fixture", ".git", "HEAD"), "ref: refs/heads/main\n", 0o644)

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()
	if _, err := os.Stat(filepath.Join(workspace.Root, ".git")); err == nil {
		t.Error("the source repository's history was copied into the trial")
	}
}

func TestPrepareWorkspaceExcludesEvaluationFilesAndMatchedDirectories(t *testing.T) {
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	config.Workspace.Exclude = []string{"FIXTURE.*", "generated/**", "**/*.secret"}
	writeFile(t, filepath.Join(directory, "fixture", "FIXTURE.PROMPT.ko.md"), "hidden\n", 0o644)
	writeFile(t, filepath.Join(directory, "fixture", "nested", "FIXTURE.TEST.sh"), "hidden\n", 0o755)
	writeFile(t, filepath.Join(directory, "fixture", "generated", "deep", "output.txt"), "hidden\n", 0o644)
	writeFile(t, filepath.Join(directory, "fixture", "nested", "token.secret"), "hidden\n", 0o644)
	writeFile(t, filepath.Join(directory, "fixture", "nested", "keep.txt"), "visible\n", 0o644)

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()

	for _, excluded := range []string{
		"FIXTURE.PROMPT.ko.md",
		filepath.Join("nested", "FIXTURE.TEST.sh"),
		"generated",
		filepath.Join("nested", "token.secret"),
	} {
		if _, err := os.Lstat(filepath.Join(workspace.Root, excluded)); !os.IsNotExist(err) {
			t.Errorf("excluded path %q survived: %v", excluded, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(workspace.Root, "nested", "keep.txt")); err != nil || string(data) != "visible\n" {
		t.Fatalf("kept file = %q, %v", data, err)
	}
}

func TestWorkspacePatternGlobstarCrossesDirectories(t *testing.T) {
	cases := map[string]bool{
		"plans/**|plans":                     true,
		"plans/**|plans/one/PLAN.md":         true,
		"**/*.log|events.log":                true,
		"**/*.log|nested/events.log":         true,
		"FIXTURE.*|nested/FIXTURE.TEST.sh":   true,
		"plans/*.md|plans/one/PLAN.md":       false,
		"plans/**/*.md|plans/one/PLAN.md":    true,
		"plans/**/*.md|plans/one/state.json": false,
	}
	for value, want := range cases {
		pattern, candidate, _ := strings.Cut(value, "|")
		if got := matchWorkspacePattern(pattern, candidate); got != want {
			t.Errorf("matchWorkspacePattern(%q, %q) = %v, want %v", pattern, candidate, got, want)
		}
	}
}

func TestPrepareWorkspaceRunsTheInitScriptInsideTheCopy(t *testing.T) {
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	writeFile(t, filepath.Join(directory, "init.sh"), "#!/bin/sh\npwd > pwd.txt\n", 0o755)
	config.Workspace.InitScript = "./init.sh"

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()
	data, err := os.ReadFile(filepath.Join(workspace.Root, "pwd.txt"))
	if err != nil {
		t.Fatal(err)
	}
	// Resolved on both sides: macOS hands out symlinked temp paths.
	got, err := filepath.EvalSymlinks(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(workspace.Root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("init script ran in %q, want %q", got, want)
	}
}

func TestPrepareWorkspaceReportsInitScriptFailureWithItsOutput(t *testing.T) {
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	writeFile(t, filepath.Join(directory, "init.sh"), "#!/bin/sh\necho no dependency\nexit 3\n", 0o755)
	config.Workspace.InitScript = "./init.sh"

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli")
	if err == nil {
		workspace.Cleanup()
		t.Fatal("expected the trial to stop when setup failed")
	}
	// The workspace is gone by then, so the output has to travel with the error.
	if !strings.Contains(err.Error(), "no dependency") {
		t.Errorf("err = %v, want it to carry the script's output", err)
	}
}

func TestPrepareWorkspaceInstallsAgentMarkdownUnderTheExpectedName(t *testing.T) {
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	writeFile(t, filepath.Join(directory, "instructions.md"), "# Rules\n", 0o644)
	config.Workspace.AgentMD = "./instructions.md"

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()
	if data, err := os.ReadFile(filepath.Join(workspace.Root, "AGENTS.md")); err != nil || string(data) != "# Rules\n" {
		t.Fatalf("AGENTS.md = %q, %v", data, err)
	}
}

func TestPrepareWorkspaceInstallsOnlyTheSkillsScopedToTheAgent(t *testing.T) {
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	writeFile(t, filepath.Join(directory, "skills", "shared", "SKILL.md"), "shared\n", 0o644)
	writeFile(t, filepath.Join(directory, "skills", "claude-only", "SKILL.md"), "claude\n", 0o644)
	config.Skills = []SkillConfig{
		{Path: "./skills/shared"},
		{Path: "./skills/claude-only", Agents: []string{"claude-code"}},
	}

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()
	root := filepath.Join(workspace.Root, filepath.FromSlash(agentTypes["custom-cli"].skillDir))
	if _, err := os.Stat(filepath.Join(root, "shared", "SKILL.md")); err != nil {
		t.Errorf("the unscoped skill was not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "claude-only")); err == nil {
		// Installing it anyway would measure custom-cli with a skill the
		// configuration said only claude-code gets.
		t.Error("a skill scoped to claude-code was installed for custom-cli")
	}
}

func TestPrepareWorkspaceMakesTheBaselineCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	config.Workspace.Git = true

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()
	if _, err := os.Stat(filepath.Join(workspace.Root, ".git")); err != nil {
		t.Fatalf("no repository was created: %v", err)
	}
	// A baseline that did not include the fixture would show the whole tree as
	// the agent's work.
	status := NewPromptEnv(workspace.Root).Sh("test -z \"$(git status --porcelain)\"")
	if status != 0 {
		t.Errorf("the baseline commit left changes uncommitted (git status = %d)", status)
	}
}

func TestCleanupRemovesEverythingAndIsRepeatable(t *testing.T) {
	directory := t.TempDir()
	workspace, err := PrepareWorkspace(context.Background(), fixtureConfig(t, directory), "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspace.Root); err == nil {
		t.Error("the workspace survived cleanup")
	}
	if err := workspace.Cleanup(); err != nil {
		t.Errorf("a second cleanup should be a no-op: %v", err)
	}
}

func TestPruneStaleWorkspacesRemovesOnlyOldTrialDirectories(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	old := time.Now().Add(-48 * time.Hour)
	for name, modTime := range map[string]time.Time{
		"mohae-old-1":   old,
		"mohae-fresh-1": time.Now(),
		"unrelated-1":   old,
	} {
		path := filepath.Join(os.TempDir(), name)
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatal(err)
		}
	}

	if removed := PruneStaleWorkspaces(24 * time.Hour); removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(filepath.Join(os.TempDir(), "mohae-old-1")); err == nil {
		t.Error("a stale workspace survived the prune")
	}
	for _, kept := range []string{"mohae-fresh-1", "unrelated-1"} {
		if _, err := os.Stat(filepath.Join(os.TempDir(), kept)); err != nil {
			t.Errorf("%s should have been left alone: %v", kept, err)
		}
	}
}
