package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func skipUnlessAvailable(t *testing.T) {
	t.Helper()
	if err := Available(); err != nil {
		t.Skipf("sandbox unavailable: %v", err)
	}
}

// trialDirs builds the shape a trial has: a base holding a writable workspace,
// with the profile beside it rather than inside it.
func trialDirs(t *testing.T) (base, workspace string) {
	t.Helper()
	base = t.TempDir()
	workspace = filepath.Join(base, "workspace")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	return base, workspace
}

func TestProfileOrdersTheBlanketAllowBeforeTheDenial(t *testing.T) {
	profile := Profile(Spec{Writable: []string{"/a"}, Network: true})
	allow := strings.Index(profile, "(allow default)")
	deny := strings.Index(profile, "(deny file-write*)")
	write := strings.Index(profile, `(allow file-write* (subpath "/a"))`)
	if allow < 0 || deny < 0 || write < 0 {
		t.Fatalf("profile is missing a rule:\n%s", profile)
	}
	// Seatbelt takes the last matching rule, so a writable path listed before
	// the denial would be overridden by it.
	if !(allow < deny && deny < write) {
		t.Errorf("rules are out of order:\n%s", profile)
	}
}

func TestProfileDeniesNetworkOnlyWhenAsked(t *testing.T) {
	if strings.Contains(Profile(Spec{Network: true}), "(deny network*)") {
		t.Error("network was denied despite being allowed")
	}
	if !strings.Contains(Profile(Spec{Network: false}), "(deny network*)") {
		t.Error("network was not denied")
	}
}

func TestProfilePutsHiddenPathsLastSoTheyOutrankTheAllow(t *testing.T) {
	profile := Profile(Spec{Writable: []string{"/a"}, DenyRead: []string{"/secret"}, Network: true})
	if at := strings.Index(profile, `(deny file-read* (subpath "/secret"))`); at < 0 {
		t.Fatalf("missing the read denial:\n%s", profile)
	} else if at < strings.Index(profile, "(allow default)") {
		t.Errorf("the read denial would be overridden by the blanket allow:\n%s", profile)
	}
}

func TestNewRefusesAProfileTheTrialCouldRewrite(t *testing.T) {
	skipUnlessAvailable(t)
	_, workspace := trialDirs(t)
	// Inside the writable workspace: the sandboxed process could rewrite it and
	// every command after the first would run under whatever it wrote.
	_, err := New(filepath.Join(workspace, "sandbox.sb"), Spec{Writable: []string{workspace}})
	if err == nil {
		t.Fatal("New() = nil error, want a refusal")
	}
	if !strings.Contains(err.Error(), "rewrite") {
		t.Errorf("error does not explain why: %v", err)
	}
}

func TestSandboxConfinesWritesToTheWorkspace(t *testing.T) {
	skipUnlessAvailable(t)
	base, workspace := trialDirs(t)
	confined, err := New(filepath.Join(base, "sandbox.sb"), Spec{Writable: []string{workspace}, Network: true})
	if err != nil {
		t.Fatal(err)
	}

	inside := confined.Command(context.Background(),
		[]string{"sh", "-c", "echo ok > " + filepath.Join(workspace, "wrote.txt")}, workspace, nil)
	if output, err := inside.CombinedOutput(); err != nil {
		t.Fatalf("writing inside the workspace failed: %v\n%s", err, output)
	}

	// The base holds the workspace but is not itself writable, so this is the
	// escape the sandbox exists to stop.
	outside := confined.Command(context.Background(),
		[]string{"sh", "-c", "echo no > " + filepath.Join(base, "escaped.txt")}, workspace, nil)
	if output, err := outside.CombinedOutput(); err == nil {
		t.Fatalf("writing outside the workspace was allowed:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(base, "escaped.txt")); !os.IsNotExist(err) {
		t.Errorf("the file was created outside the workspace: %v", err)
	}
}

func TestSandboxLeavesTheHostToolchainReadable(t *testing.T) {
	skipUnlessAvailable(t)
	base, workspace := trialDirs(t)
	confined, err := New(filepath.Join(base, "sandbox.sb"), Spec{Writable: []string{workspace}, Network: true})
	if err != nil {
		t.Fatal(err)
	}
	// The whole point: a sandboxed trial still uses what is installed here,
	// which is what makes it cheaper than an image that has to carry it.
	command := confined.Command(context.Background(), []string{"sh", "-c", "command -v sh && cat /etc/hosts > /dev/null"}, workspace, nil)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("reading the host failed: %v\n%s", err, output)
	}
}

func TestSandboxPointsCommandsAtTheTrialsOwnTempDir(t *testing.T) {
	skipUnlessAvailable(t)
	base, workspace := trialDirs(t)
	temporary := filepath.Join(base, "tmp")
	if err := os.Mkdir(temporary, 0o755); err != nil {
		t.Fatal(err)
	}
	confined, err := New(filepath.Join(base, "sandbox.sb"), Spec{
		Writable: []string{workspace, temporary},
		Network:  true,
		TempDir:  temporary,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := confined.Command(context.Background(), []string{"sh", "-c", `printf '%s' "$TMPDIR"`}, workspace, nil)
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	// Resolved rather than compared verbatim: the sandbox matches on the real
	// path, and t.TempDir hands back one that goes through a symlink.
	want, _ := filepath.EvalSymlinks(temporary)
	got, _ := filepath.EvalSymlinks(string(output))
	if got != want {
		t.Errorf("TMPDIR = %q, want %q", got, want)
	}
}

func TestSandboxKeepsAConfiguredTempDir(t *testing.T) {
	skipUnlessAvailable(t)
	base, workspace := trialDirs(t)
	confined, err := New(filepath.Join(base, "sandbox.sb"), Spec{
		Writable: []string{workspace},
		Network:  true,
		TempDir:  filepath.Join(base, "tmp"),
	})
	if err != nil {
		t.Fatal(err)
	}
	command := confined.Command(context.Background(),
		[]string{"sh", "-c", `printf '%s' "$TMPDIR"`}, workspace,
		map[string]string{"TMPDIR": workspace})
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != workspace {
		t.Errorf("TMPDIR = %q, want the configured %q", output, workspace)
	}
}

func TestSandboxIsIsolatedButNotContained(t *testing.T) {
	skipUnlessAvailable(t)
	base, workspace := trialDirs(t)
	confined, err := New(filepath.Join(base, "sandbox.sb"), Spec{Writable: []string{workspace}})
	if err != nil {
		t.Fatal(err)
	}
	// Not contained: the filesystem is this host's, so paths mean the same
	// thing on both sides and nothing has to be copied to be reachable.
	if confined.Contained() {
		t.Error("Contained() = true, but the sandbox shares the host's filesystem")
	}
	if got := confined.Path("/some/path"); got != "/some/path" {
		t.Errorf("Path() = %q, want the identity", got)
	}
	// Isolated: a command spawned around this executor would run unconfined,
	// which is what a driver has to know before letting its SDK spawn the CLI.
	if !confined.Isolated() {
		t.Error("Isolated() = false, so a driver would start the agent unconfined")
	}
}

func TestResolveAllDropsPathsThatDoNotExist(t *testing.T) {
	directory := t.TempDir()
	got := resolveAll([]string{directory, filepath.Join(directory, "missing"), ""})
	if len(got) != 1 {
		t.Fatalf("resolveAll() = %v, want only the existing directory", got)
	}
	// Resolved, because the sandbox matches on the real path: on macOS
	// t.TempDir is reached through /var, which is a symlink to /private/var.
	want, _ := filepath.EvalSymlinks(directory)
	if got[0] != want {
		t.Errorf("resolveAll() = %q, want the resolved %q", got[0], want)
	}
}
