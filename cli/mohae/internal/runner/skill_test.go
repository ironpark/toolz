package runner

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	skillsrc "github.com/ironpark/toolz/cli/mohae/internal/skill"
)

const fetchedCommit = "0123456789abcdef0123456789abcdef01234567"

// skillHost serves a repository holding two skills the way GitHub does: a ref
// endpoint that answers with a commit, and that commit's tarball.
func skillHost(t *testing.T) *skillsrc.Resolver {
	t.Helper()
	archive := filepath.Join(t.TempDir(), "repo.tar.gz")
	writeTarball(t, archive, map[string]string{
		"repo-abc/skills/commit/SKILL.md": "---\nname: commit\n---\ncommit instructions\n",
		"repo-abc/skills/review/SKILL.md": "---\nname: review\n---\nreview instructions\n",
	})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.Contains(request.URL.Path, "/commits/"):
			writer.Write([]byte(fetchedCommit))
		case strings.Contains(request.URL.Path, "/tar.gz/"):
			http.ServeFile(writer, request, archive)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	return &skillsrc.Resolver{
		Root: t.TempDir(),
		HTTP: &http.Client{Transport: toHost{base: server.URL}},
	}
}

// toHost points the resolver's two GitHub hosts at the stub, so the fetching
// path runs without the test reaching the network.
type toHost struct{ base string }

func (h toHost) RoundTrip(request *http.Request) (*http.Response, error) {
	redirected, err := http.NewRequestWithContext(request.Context(), request.Method, h.base+request.URL.Path, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultTransport.RoundTrip(redirected)
}

func writeTarball(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	zipped := gzip.NewWriter(file)
	writer := tar.NewWriter(zipped)
	for name, content := range entries {
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zipped.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareWorkspaceInstallsEverySkillARemoteSourcePublishes(t *testing.T) {
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	config.Skills = []SkillConfig{{Source: "owner/repo", Ref: "main"}}

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli", skillHost(t))
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()

	// A source naming no subpath installs the set, so a repository of skills
	// does not have to be listed one entry at a time.
	for _, name := range []string{"commit", "review"} {
		manifest := filepath.Join(workspace.Root, ".agent", "skills", name, "SKILL.md")
		if _, err := os.Stat(manifest); err != nil {
			t.Errorf("%s was not installed: %v", name, err)
		}
	}
	if len(workspace.Skills) != 2 {
		t.Fatalf("recorded skills = %+v", workspace.Skills)
	}
	// The commit is the point: it is the only thing that says which revision of
	// the instructions the agent was actually given.
	for _, installed := range workspace.Skills {
		if installed.Commit != fetchedCommit {
			t.Errorf("%s commit = %q, want %q", installed.Name, installed.Commit, fetchedCommit)
		}
		if installed.Source != "owner/repo@main" {
			t.Errorf("%s source = %q", installed.Name, installed.Source)
		}
	}
}

func TestPrepareWorkspaceInstallsOnlyTheNamedSubpath(t *testing.T) {
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	config.Skills = []SkillConfig{{Source: "owner/repo", Ref: "main", Subpath: "skills/commit"}}

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli", skillHost(t))
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()

	if _, err := os.Stat(filepath.Join(workspace.Root, ".agent", "skills", "commit")); err != nil {
		t.Errorf("commit was not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace.Root, ".agent", "skills", "review")); !os.IsNotExist(err) {
		t.Errorf("review was installed despite the subpath: %v", err)
	}
}

func TestPrepareWorkspaceKeepsRemoteSkillsScopedPerAgent(t *testing.T) {
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	config.Skills = []SkillConfig{{Source: "owner/repo", Ref: "main", Agents: []string{"claude-code"}}}

	// A skill scoped to another agent must not be fetched at all: a download
	// for a trial that would never use it is wasted network either way.
	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli", &skillsrc.Resolver{Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()

	if len(workspace.Skills) != 0 {
		t.Errorf("recorded skills = %+v, want none", workspace.Skills)
	}
}

func TestPrepareWorkspaceFailsWhenARemoteSkillCannotBeFetched(t *testing.T) {
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	config.Skills = []SkillConfig{{Source: "owner/repo", Ref: "main"}}

	// Offline with an empty cache: the trial cannot be set up as configured,
	// and running it anyway would measure an agent missing its instructions.
	_, err := PrepareWorkspace(context.Background(), config, "custom-cli", &skillsrc.Resolver{
		Root:    t.TempDir(),
		Offline: true,
	})
	if err == nil {
		t.Fatal("PrepareWorkspace() = nil error, want the fetch failure")
	}
	if !strings.Contains(err.Error(), "skills[0]") {
		t.Errorf("error does not name the entry: %v", err)
	}
}
