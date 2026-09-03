// Package sandbox confines what a trial may write to without moving it off
// this machine.
//
// It answers a different question from the container package, and the two are
// worth keeping apart. A container pins the toolchain: it decides which
// compiler and which test runner a trial is built and graded with, which is
// what makes a measurement comparable across machines. A sandbox pins nothing —
// the trial uses whatever is installed here — and only bounds where it may
// write. That is much cheaper (no image, no daemon, no root, microseconds
// rather than seconds) and it is enough for the thing a container is most often
// reached for: making two agents on one machine play by the same rules.
//
// The gap it closes is a real one. Left on the host an agent runs with
// permission prompts bypassed, and a prompt that does not name a directory has
// been seen to leave its work outside the workspace and fail verification for
// the wrong reason. A sandbox stops that without asking the configuration to
// carry an image and a mounted credentials directory.
package sandbox

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	processutil "github.com/ironpark/toolz/cli/mohae/internal/process"
)

// binary is the seatbelt front-end. It ships with macOS, so a sandboxed trial
// needs nothing installed.
const binary = "/usr/bin/sandbox-exec"

// Available reports whether this machine can sandbox a trial, and says why not
// when it cannot. A configuration that asked for a sandbox and did not get one
// must fail rather than quietly run unconfined: a trial that ran with the whole
// filesystem writable would otherwise be reported as one that did not.
func Available() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("sandbox is implemented for macOS only, and this is %s", runtime.GOOS)
	}
	if _, err := os.Stat(binary); err != nil {
		return fmt.Errorf("%s is not available on this machine", binary)
	}
	return nil
}

// Spec is what a sandbox allows. Everything on this machine stays readable —
// that is the whole point, since the trial is meant to use the toolchain
// installed here — so a spec only has to say what may be written and what must
// not be seen at all.
type Spec struct {
	// Writable are the host paths the trial may write to. They are resolved
	// before use: macOS reports a path through /tmp while the sandbox matches
	// on /private/tmp, and a rule written the other way silently matches
	// nothing.
	Writable []string
	// DenyRead are paths hidden from the trial entirely. Nothing is hidden by
	// default; this is for a configuration that would rather an agent could not
	// read the credentials sitting in the home directory it is running under.
	DenyRead []string
	// Network allows the trial to reach the network. An agent needs it to
	// answer at all, so it is on unless a configuration grading offline turns
	// it off.
	Network bool
	// TempDir is the trial's own temporary directory, exported to every
	// command as TMPDIR.
	//
	// The machine's shared temporary directory is deliberately not writable.
	// On macOS it is the per-user directory under /var/folders, which is also
	// where the trial's workspace lives — allowing it would hand back
	// everything the sandbox just took away, this profile included. A
	// directory of the trial's own keeps well-behaved tools working and is
	// removed with the workspace.
	TempDir string
}

// Sandbox is an Executor that runs commands under a seatbelt profile.
//
// The profile is written once, at the path given to New, and every command
// refers to it. That path must sit somewhere the trial cannot write: a profile
// the sandboxed process could rewrite would bound only the first command run
// under it.
type Sandbox struct {
	profile string
	spec    Spec
}

// New writes spec's profile to profilePath and returns the executor that uses
// it. profilePath must not be inside any of spec's writable paths.
func New(profilePath string, spec Spec) (*Sandbox, error) {
	if err := Available(); err != nil {
		return nil, err
	}
	resolved := Spec{
		Writable: resolveAll(spec.Writable),
		DenyRead: resolveAll(spec.DenyRead),
		Network:  spec.Network,
		TempDir:  spec.TempDir,
	}
	if inside := writableAncestor(resolved.Writable, profilePath); inside != "" {
		return nil, fmt.Errorf("sandbox profile %s would sit inside the writable path %s, where the trial could rewrite it", profilePath, inside)
	}
	if err := os.WriteFile(profilePath, []byte(Profile(resolved)), 0o444); err != nil {
		return nil, fmt.Errorf("writing sandbox profile: %w", err)
	}
	return &Sandbox{profile: profilePath, spec: resolved}, nil
}

// Command builds the process under the profile. The environment is this
// process's own with the trial's overlay on top, exactly as the host executor
// would build it: a sandbox changes what a command may write, not what it is.
func (s *Sandbox) Command(ctx context.Context, argv []string, dir string, env map[string]string) *exec.Cmd {
	args := append([]string{"-f", s.profile}, argv...)
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = dir
	command.Env = processutil.Env(s.tempEnv(env))
	processutil.Isolate(command)
	return command
}

// tempEnv points the command at the trial's own temporary directory. A
// configuration that set TMPDIR itself is left alone: it named a directory it
// presumably also made writable.
func (s *Sandbox) tempEnv(env map[string]string) map[string]string {
	if s.spec.TempDir == "" {
		return env
	}
	if _, named := env["TMPDIR"]; named {
		return env
	}
	combined := make(map[string]string, len(env)+1)
	maps.Copy(combined, env)
	combined["TMPDIR"] = s.spec.TempDir
	return combined
}

// Path is the identity: a sandboxed command sees this machine's filesystem
// under the names it already has. This is the difference from a container,
// which shows the workspace at a path of its own, and it is why a sandboxed
// trial needs none of the copying a containerised one does.
func (s *Sandbox) Path(host string) string { return host }

// Contained is false: the filesystem is this host's. The scripts a
// configuration names are readable where they already are, and a path in the
// environment means the same thing inside and out.
func (s *Sandbox) Contained() bool { return false }

// Isolated is true: a command started outside this executor would run without
// the profile, which is to say unconfined. It is what a driver reads to decide
// whether it may let its SDK spawn the agent CLI itself.
func (s *Sandbox) Isolated() bool { return true }

// Writable is what the trial was allowed to write to, for the report.
func (s *Sandbox) Writable() []string { return slices.Clone(s.spec.Writable) }

// Profile renders spec as a seatbelt policy.
//
// Order matters: seatbelt takes the last rule that matches, so the blanket
// allow comes first, the write denial after it, and the writable paths after
// that. Anything hidden comes last, so it outranks the allow it contradicts.
func Profile(spec Spec) string {
	var out strings.Builder
	out.WriteString("(version 1)\n\n")
	out.WriteString(";; Readable by default: the trial is meant to use the toolchain\n")
	out.WriteString(";; installed on this machine, and guessing which parts of it a\n")
	out.WriteString(";; compiler needs is a game that cannot be won.\n")
	out.WriteString("(allow default)\n\n")

	out.WriteString(";; Writes are the boundary. Everything the trial produces has to land\n")
	out.WriteString(";; somewhere the report can account for.\n")
	out.WriteString("(deny file-write*)\n")
	for _, path := range spec.Writable {
		fmt.Fprintf(&out, "(allow file-write* (subpath %s))\n", quote(path))
	}
	out.WriteString("\n;; Device nodes: writing here persists nothing, and denying it breaks\n")
	out.WriteString(";; any program that opens a terminal or /dev/null.\n")
	out.WriteString("(allow file-write* (subpath \"/dev\"))\n")

	if !spec.Network {
		out.WriteString("\n;; Graded offline, so a verification that reaches the network fails\n")
		out.WriteString(";; here rather than depending on what it found there.\n")
		out.WriteString("(deny network*)\n")
	}
	if len(spec.DenyRead) > 0 {
		out.WriteString("\n;; Hidden outright, last so it outranks the blanket allow above.\n")
		for _, path := range spec.DenyRead {
			fmt.Fprintf(&out, "(deny file-read* (subpath %s))\n", quote(path))
		}
	}
	return out.String()
}

// resolveAll turns paths into the form the sandbox matches on, dropping any
// that do not exist: a rule naming a missing directory matches nothing, and
// keeping it would only make the profile harder to read.
func resolveAll(paths []string) []string {
	resolved := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		actual, err := filepath.EvalSymlinks(path)
		if err != nil {
			continue
		}
		if absolute, err := filepath.Abs(actual); err == nil {
			actual = absolute
		}
		if !slices.Contains(resolved, actual) {
			resolved = append(resolved, actual)
		}
	}
	return resolved
}

// writableAncestor reports which writable path contains candidate, if any.
func writableAncestor(writable []string, candidate string) string {
	resolved := candidate
	// The parent is resolved rather than the file, which need not exist yet.
	if parent, err := filepath.EvalSymlinks(filepath.Dir(candidate)); err == nil {
		resolved = filepath.Join(parent, filepath.Base(candidate))
	}
	for _, path := range writable {
		if resolved == path || strings.HasPrefix(resolved, path+string(os.PathSeparator)) {
			return path
		}
	}
	return ""
}

// quote renders a path as a seatbelt string literal.
func quote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}
