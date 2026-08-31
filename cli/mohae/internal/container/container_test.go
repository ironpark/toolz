package container

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The command lines are checked rather than executed: what mohae asks a
// runtime for is the part that decides whether a trial is isolated, and it has
// to be reviewable on a machine that has neither runtime installed.

func TestRunArgsMountsTheTrialDirectoryAtTheFixedPoint(t *testing.T) {
	args, err := Spec{Image: "golang:1.26", Base: "/tmp/mohae-x"}.runArgs("golang:1.26", false)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFlagValue(args, "--volume", "/tmp/mohae-x:"+MountPoint) {
		t.Fatalf("trial directory is not mounted at %s: %v", MountPoint, args)
	}
	if !hasFlagValue(args, "--workdir", MountPoint) {
		t.Fatalf("--workdir missing: %v", args)
	}
	// The image's own entrypoint would exit, and mohae needs something to exec
	// into for the length of the trial.
	if !hasFlagValue(args, "--entrypoint", "sh") || !slices.Contains(args, idleScript) {
		t.Fatalf("container is not kept alive: %v", args)
	}
	if index := slices.Index(args, "golang:1.26"); index < 0 || index > len(args)-2 {
		t.Fatalf("image is not followed by its arguments: %v", args)
	}
}

func TestRunArgsKeepsFilesOwnedByTheInvokingUser(t *testing.T) {
	if os.Getuid() < 0 {
		t.Skip("no host user id to map")
	}
	// The workspace is a bind mount: whatever writes to it inside owns the
	// files outside, and a run as root leaves a directory mohae cannot delete.
	for _, podman := range []bool{false, true} {
		args, err := Spec{Image: "img", Base: "/tmp/b", User: UserHost}.runArgs("img", podman)
		if err != nil {
			t.Fatal(err)
		}
		if !hasFlag(args, "--user") {
			t.Fatalf("podman=%v: --user missing: %v", podman, args)
		}
		// Rootless podman would otherwise remap the user, and the files would
		// come back owned by a subordinate id.
		if got := slices.Contains(args, "--userns=keep-id"); got != podman {
			t.Fatalf("podman=%v: keep-id = %v", podman, got)
		}
	}
}

func TestRunArgsPassesUserAndNetworkThrough(t *testing.T) {
	args, err := Spec{Image: "img", Base: "/tmp/b", User: "1000:1000", Network: "none"}.runArgs("img", false)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFlagValue(args, "--user", "1000:1000") {
		t.Fatalf("explicit user not passed through: %v", args)
	}
	if !hasFlagValue(args, "--network", "none") {
		t.Fatalf("network not passed through: %v", args)
	}
}

func TestRunArgsOrdersEnvironmentSoTwoRunsMatch(t *testing.T) {
	spec := Spec{Image: "img", Base: "/tmp/b", Env: map[string]string{"B": "2", "A": "1", "C": "3"}}
	first, err := spec.runArgs("img", false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := spec.runArgs("img", false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(first, second) {
		t.Fatalf("two runs of one spec differ:\n%v\n%v", first, second)
	}
	if !hasFlagValue(first, "--env", "A=1") || !hasFlagValue(first, "--env", "C=3") {
		t.Fatalf("environment missing: %v", first)
	}
}

func TestMountArgRelabelsOnlyWhatMohaeOwns(t *testing.T) {
	// :Z relabels the host directory, so it is applied to the trial's own
	// temporary directory and never to a path the configuration named.
	if got := mountArg(Mount{Host: "/a", Target: "/b", Relabel: true}, true); got != "/a:/b:Z" {
		t.Fatalf("podman base mount = %q", got)
	}
	if got := mountArg(Mount{Host: "/a", Target: "/b", Relabel: true}, false); got != "/a:/b" {
		t.Fatalf("docker base mount = %q", got)
	}
	if got := mountArg(Mount{Host: "/h/.claude", Target: "/root/.claude", ReadOnly: true}, true); got != "/h/.claude:/root/.claude:ro" {
		t.Fatalf("configured mount = %q", got)
	}
}

func TestPathMapsTheTrialDirectoryAndNothingElse(t *testing.T) {
	base := filepath.FromSlash("/tmp/mohae-run")
	c := &Container{base: base}
	cases := map[string]string{
		base:                                  MountPoint,
		filepath.Join(base, "workspace"):      MountPoint + "/workspace",
		filepath.Join(base, "scratch", "out"): MountPoint + "/scratch/out",
		// Outside the mount the container cannot see it, and inventing a path
		// would silently point a command at a different file.
		filepath.FromSlash("/etc/passwd"): filepath.FromSlash("/etc/passwd"),
		filepath.FromSlash("/tmp"):        filepath.FromSlash("/tmp"),
	}
	for host, want := range cases {
		if got := c.Path(host); got != want {
			t.Errorf("Path(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestKillScriptMatchesTheMarkerExactly(t *testing.T) {
	// A prefix match would let exec 1 kill exec 10, which on a multi-turn
	// trial means one timed-out turn stopping the next one's work.
	if !strings.Contains(killScript, "grep -qxF") {
		t.Fatalf("kill script does not match whole lines: %s", killScript)
	}
}

func TestDetectRejectsAnUnknownRuntime(t *testing.T) {
	// Falling back would report a trial as isolated by something other than
	// what it asked for.
	if _, err := Detect("containerd"); err == nil {
		t.Fatal("an unknown runtime was accepted")
	}
}

func TestLastLineIsTheRuntimesAnswer(t *testing.T) {
	if got := lastLine("Sending build context\nsha256:abc\n"); got != "sha256:abc" {
		t.Fatalf("lastLine = %q", got)
	}
}

func hasFlag(args []string, flag string) bool { return slices.Contains(args, flag) }

func hasFlagValue(args []string, flag, value string) bool {
	for index, argument := range args {
		if argument == flag && index+1 < len(args) && args[index+1] == value {
			return true
		}
	}
	return false
}
