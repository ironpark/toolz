package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/jsonout"
	ucli "github.com/urfave/cli/v3"
)

func newInitTestCommand() *ucli.Command {
	return &ucli.Command{
		Name: "init",
		Flags: []ucli.Flag{
			&ucli.StringFlag{Name: "language"},
			&ucli.StringSliceFlag{Name: "plans-dir"},
			&ucli.BoolFlag{Name: "force"},
			&ucli.BoolFlag{Name: "json"},
		},
		Action: initCommand,
	}
}

func runInit(t *testing.T, arguments ...string) error {
	t.Helper()
	return newInitTestCommand().Run(context.Background(), append([]string{"init"}, arguments...))
}

func initRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatalf("git init: %v", err)
	}
	return root
}

func TestInitWritesConfigAndPlansDirectory(t *testing.T) {
	root := initRepository(t)
	withWorkingDirectory(t, root)

	if err := runInit(t); err != nil {
		t.Fatalf("init() unexpected error: %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "plan")); err != nil || !info.IsDir() {
		t.Fatalf("plans directory was not created: %v", err)
	}
	// The written file has to be the one planr then reads back; a template that
	// does not round-trip would leave `init` followed by `config` disagreeing.
	settings, foundRoot, err := config.Load(root)
	if err != nil {
		t.Fatalf("config.Load() after init: %v", err)
	}
	if foundRoot != root {
		t.Fatalf("root = %q, want %q", foundRoot, root)
	}
	if settings.Path != filepath.Join(root, ".planr.yaml") {
		t.Fatalf("config path = %q", settings.Path)
	}
	if settings.Language != doc.DefaultLanguage {
		t.Fatalf("language = %q, want %q", settings.Language, doc.DefaultLanguage)
	}
	if len(settings.PlansDirs) != 1 || settings.PlansDirs[0] != "plan" {
		t.Fatalf("plans_dirs = %#v", settings.PlansDirs)
	}
}

func TestInitHonoursLanguageAndPlansDirectories(t *testing.T) {
	root := initRepository(t)
	withWorkingDirectory(t, root)

	if err := runInit(t, "--language", "ko", "--plans-dir", "plans-active", "--plans-dir", "plans-archive"); err != nil {
		t.Fatalf("init() unexpected error: %v", err)
	}
	settings, _, err := config.Load(root)
	if err != nil {
		t.Fatalf("config.Load() after init: %v", err)
	}
	if settings.Language != doc.Korean {
		t.Fatalf("language = %q, want %q", settings.Language, doc.Korean)
	}
	if len(settings.PlansDirs) != 2 || settings.PlansDirs[0] != "plans-active" || settings.PlansDirs[1] != "plans-archive" {
		t.Fatalf("plans_dirs = %#v", settings.PlansDirs)
	}
	for _, directory := range settings.PlansDirs {
		if info, err := os.Stat(filepath.Join(root, directory)); err != nil || !info.IsDir() {
			t.Fatalf("%s was not created: %v", directory, err)
		}
	}
}

func TestInitRejectsUnsupportedLanguageBeforeWriting(t *testing.T) {
	root := initRepository(t)
	withWorkingDirectory(t, root)

	if err := runInit(t, "--language", "fr"); err == nil {
		t.Fatal("init accepted an unsupported language")
	}
	if _, err := os.Stat(filepath.Join(root, ".planr.yaml")); !os.IsNotExist(err) {
		t.Fatal("a rejected init left a configuration behind")
	}
}

func TestInitRejectsAbsolutePlansDirectory(t *testing.T) {
	root := initRepository(t)
	withWorkingDirectory(t, root)

	if err := runInit(t, "--plans-dir", "/etc"); err == nil {
		t.Fatal("init accepted an absolute plans directory")
	}
}

func TestInitDoesNotOverwriteWithoutForce(t *testing.T) {
	root := initRepository(t)
	withWorkingDirectory(t, root)
	existing := "language: ko\nplans_dir: mine\n"
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	err := runInit(t)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("init() error = %v, want a refusal naming --force", err)
	}
	if got := readFileString(t, filepath.Join(root, ".planr.yaml")); got != existing {
		t.Fatalf("configuration was modified:\n%s", got)
	}
	if err := runInit(t, "--force"); err != nil {
		t.Fatalf("init --force: %v", err)
	}
	if got := readFileString(t, filepath.Join(root, ".planr.yaml")); got == existing {
		t.Fatal("--force did not rewrite the configuration")
	}
}

func TestInitForceRepairsAnUnparseableConfig(t *testing.T) {
	// The one command that can fix a broken .planr.yaml must not be blocked by
	// it: detection reads the path, not the contents.
	root := initRepository(t)
	withWorkingDirectory(t, root)
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dirs: [\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := runInit(t, "--force"); err != nil {
		t.Fatalf("init --force on a broken config: %v", err)
	}
	if _, _, err := config.Load(root); err != nil {
		t.Fatalf("config still does not parse: %v", err)
	}
}

func TestInitFromSubdirectoryWritesAtTheRepositoryRoot(t *testing.T) {
	// Otherwise a stray config in a subdirectory would shadow the repository
	// for every command run below it.
	root := initRepository(t)
	nested := filepath.Join(root, "services", "api")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, nested)

	if err := runInit(t); err != nil {
		t.Fatalf("init() unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".planr.yaml")); err != nil {
		t.Fatalf("configuration was not written at the repository root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nested, ".planr.yaml")); !os.IsNotExist(err) {
		t.Fatal("configuration was written in the subdirectory")
	}
}

func TestInitRefusesWhenTheConfigLivesElsewhereInTheRepository(t *testing.T) {
	root := initRepository(t)
	nested := filepath.Join(root, "services")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, ".planr.yaml"), []byte("language: ko\n"), 0644); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, nested)

	err := runInit(t, "--force")
	if err == nil || !strings.Contains(err.Error(), filepath.Join(nested, ".planr.yaml")) {
		t.Fatalf("init() error = %v, want it to name the existing configuration", err)
	}
}

func TestInitReusesAnExistingPlansDirectory(t *testing.T) {
	root := initRepository(t)
	if err := os.MkdirAll(filepath.Join(root, "plan"), 0755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(root, "plan", "keep.md")
	if err := os.WriteFile(keep, []byte("# existing\n"), 0644); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, root)

	if err := runInit(t); err != nil {
		t.Fatalf("init() unexpected error: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("existing plans content was disturbed: %v", err)
	}
}

func TestInitWritesOutsideAGitRepositoryAndWarns(t *testing.T) {
	// A project is often configured before `git init`; refusing would make the
	// first command a user runs the one that fails.
	root := t.TempDir()
	withWorkingDirectory(t, root)

	stderr := captureStderr(t, func() {
		if err := runInit(t); err != nil {
			t.Fatalf("init() unexpected error: %v", err)
		}
	})
	if _, err := os.Stat(filepath.Join(root, ".planr.yaml")); err != nil {
		t.Fatalf("configuration was not written: %v", err)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "git") {
		t.Fatalf("stderr = %q, want a warning about the missing repository", stderr)
	}
}

func TestInitOutsideARepositoryIgnoresAnAncestorConfig(t *testing.T) {
	// Without a repository the upward search has no boundary, so an unrelated
	// .planr.yaml above the project must not be treated as this project's.
	parent := t.TempDir()
	if err := os.WriteFile(filepath.Join(parent, ".planr.yaml"), []byte("language: ko\n"), 0644); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(parent, "project")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, project)

	captureStderr(t, func() {
		if err := runInit(t); err != nil {
			t.Fatalf("init() unexpected error: %v", err)
		}
	})
	if _, err := os.Stat(filepath.Join(project, ".planr.yaml")); err != nil {
		t.Fatalf("configuration was not written in the project: %v", err)
	}
}

func TestInitDoesNotWarnInsideARepository(t *testing.T) {
	root := initRepository(t)
	withWorkingDirectory(t, root)

	stderr := captureStderr(t, func() {
		if err := runInit(t); err != nil {
			t.Fatalf("init() unexpected error: %v", err)
		}
	})
	if strings.Contains(stderr, "warning:") {
		t.Fatalf("stderr = %q, want no warning", stderr)
	}
}

func TestInitRefusalOutsideARepositoryDoesNotClaimToHaveWritten(t *testing.T) {
	root := t.TempDir()
	existing := "language: ko\n"
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, root)

	var err error
	stderr := captureStderr(t, func() { err = runInit(t) })
	if err == nil {
		t.Fatal("init overwrote an existing configuration without --force")
	}
	if strings.Contains(stderr, "anyway") {
		t.Fatalf("stderr = %q, want no claim that anything was written", stderr)
	}
	if got := readFileString(t, filepath.Join(root, ".planr.yaml")); got != existing {
		t.Fatalf("configuration was modified:\n%s", got)
	}
}

// captureStderr collects what fn writes to os.Stderr, which is where init warns
// so that --json output on stdout stays machine-readable.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = writer
	defer func() { os.Stderr = original }()

	done := make(chan string, 1)
	go func() {
		var builder strings.Builder
		buffer := make([]byte, 1024)
		for {
			read, readErr := reader.Read(buffer)
			builder.Write(buffer[:read])
			if readErr != nil {
				break
			}
		}
		done <- builder.String()
	}()

	fn()
	writer.Close()
	return <-done
}

func TestInitRejectsPositionalArguments(t *testing.T) {
	root := initRepository(t)
	withWorkingDirectory(t, root)

	if err := runInit(t, "somewhere"); err == nil {
		t.Fatal("init accepted a positional argument")
	}
}

func TestMakeInitJSONEncodesEmptyListsNotNull(t *testing.T) {
	output := jsonout.Init("/repo/.planr.yaml", "/repo", doc.English, []string{"plan"}, nil, nil)
	if output.Created == nil || output.Existed == nil {
		t.Fatalf("created/existed must encode as [], got %#v", output)
	}
}
