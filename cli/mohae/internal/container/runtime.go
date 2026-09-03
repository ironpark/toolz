package container

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// The runtimes mohae knows how to drive, plus the value that asks it to pick.
const (
	Auto   = "auto"
	Docker = "docker"
	Podman = "podman"
)

// Runtimes is the probe order for Auto and the list diagnostics quote. Docker
// comes first because a machine with both installed is usually a Docker
// machine with Podman alongside it, not the other way round.
var Runtimes = []string{Docker, Podman}

// IsKnownRuntime reports whether name selects a runtime mohae can drive.
// Auto is accepted here: it is a valid thing to write in a configuration.
func IsKnownRuntime(name string) bool {
	return name == Auto || name == Docker || name == Podman
}

// Runtime is a resolved container CLI.
type Runtime struct {
	// Name is docker or podman — never auto, which is a request rather than
	// an answer.
	Name string
	// Path is the resolved executable, looked up once so every command in a
	// trial runs the same binary even if PATH changes under it.
	Path string
}

// Detect resolves the configured runtime, or picks the first one installed
// when the configuration did not choose. An unavailable runtime is an error
// rather than a fallback: a trial that asked for Podman and silently got
// Docker would be a different measurement under the same name.
func Detect(preferred string) (*Runtime, error) {
	if preferred == "" {
		preferred = Auto
	}
	if preferred == Auto {
		for _, name := range Runtimes {
			if path, err := exec.LookPath(name); err == nil {
				return &Runtime{Name: name, Path: path}, nil
			}
		}
		return nil, fmt.Errorf("no container runtime found on PATH (looked for %s)", strings.Join(Runtimes, " and "))
	}
	if !IsKnownRuntime(preferred) {
		return nil, fmt.Errorf("unknown container runtime %q (one of: %s, %s)", preferred, Auto, strings.Join(Runtimes, ", "))
	}
	path, err := exec.LookPath(preferred)
	if err != nil {
		return nil, fmt.Errorf("container runtime %q is not on PATH: %w", preferred, err)
	}
	return &Runtime{Name: preferred, Path: path}, nil
}

// Podman reports whether this runtime is Podman, which differs from Docker in
// the two places that matter here: how a bind mount is labelled for SELinux,
// and how the host's user id is mapped into a rootless container.
func (r *Runtime) Podman() bool { return r != nil && r.Name == Podman }

// run executes one runtime command and returns its standard output, folding
// standard error into the error so a failure says what the runtime said.
func (r *Runtime) run(ctx context.Context, args ...string) (string, error) {
	command := exec.CommandContext(ctx, r.Path, args...)
	stderr := &strings.Builder{}
	command.Stderr = stderr
	output, err := command.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			return "", fmt.Errorf("%s %s: %w", r.Name, args[0], err)
		}
		return "", fmt.Errorf("%s %s: %w\n%s", r.Name, args[0], err, detail)
	}
	return strings.TrimSpace(string(output)), nil
}
