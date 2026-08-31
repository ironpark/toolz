package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ironpark/toolz/cli/mohae/internal/container"
)

// Container scopes. The scope decides where the boundary falls, and the two
// answers are useful for different reasons.
//
// setup containerises everything mohae runs on the trial's behalf — the setup
// script, the hooks and the verification commands — while the agent stays on
// the host. That is what pins the toolchain a trial is built and graded with,
// and it needs nothing of the agent in the image.
//
// full adds the agent itself, which is the only way two agents are measured
// under the same rules: it bounds what the agent can reach as well as what it
// starts from. It costs more to set up, because the image must carry the agent
// CLI and the credentials it logs in with.
const (
	ContainerScopeSetup = "setup"
	ContainerScopeFull  = "full"
)

// ContainerScopes is stable for diagnostics.
var ContainerScopes = []string{ContainerScopeSetup, ContainerScopeFull}

// ContainerConfig runs a trial inside a container instead of on the host.
//
// It is off unless the configuration names an image or a Dockerfile: a trial
// that did not ask for a container should not need a runtime installed to run.
type ContainerConfig struct {
	// Image is the image to run. Exactly one of image and build is set.
	Image string `yaml:"image,omitempty"`
	// Build is a directory containing a Dockerfile, built before each trial.
	// The runtime's layer cache makes the repeat builds of a comparison run
	// cheap; nothing here caches on mohae's behalf.
	Build string `yaml:"build,omitempty"`
	// Runtime selects docker or podman. The default picks the first one
	// installed, which is what a configuration shared between machines wants.
	Runtime string `yaml:"runtime,omitempty"`
	// Scope decides whether the agent runs inside too. See the scope constants.
	Scope string `yaml:"scope,omitempty"`
	// Network is passed to the runtime unchanged. "none" is what a trial that
	// wants its grading to be reproducible asks for.
	Network string `yaml:"network,omitempty"`
	// User is who the trial runs as: "host" (the default) keeps the files the
	// trial writes owned by the user who started mohae, "root", or an explicit
	// uid:gid.
	User string `yaml:"user,omitempty"`
	// Env is set on the container, so every command run inside sees it. It is
	// separate from agent.env, which is the trial's own overlay and reaches
	// the agent whether or not there is a container.
	Env map[string]string `yaml:"env,omitempty"`
	// Mounts are host directories made visible inside. With scope full this is
	// how the agent's credentials get in.
	Mounts []ContainerMount `yaml:"mounts,omitempty"`
}

// ContainerMount is one host directory bound into the container.
type ContainerMount struct {
	// Source is a host path, resolved against the configuration file like
	// every other path, with a leading ~ expanded: a credentials directory is
	// named from the home directory far more often than from the config's.
	Source string `yaml:"source"`
	// Target is where it appears inside, and must be absolute.
	Target string `yaml:"target"`
	// ReadOnly is what a mount carrying credentials should be.
	ReadOnly bool `yaml:"read_only,omitempty"`
}

// Enabled reports whether this trial runs in a container at all.
func (c ContainerConfig) Enabled() bool { return c.Image != "" || c.Build != "" }

// AgentInside reports whether the agent under test runs in the container as
// well as the commands around it.
func (c ContainerConfig) AgentInside() bool {
	return c.Enabled() && c.Scope == ContainerScopeFull
}

// ContainerSpec resolves the configuration into what the container package
// needs. base is the trial's own directory, which is mounted inside; the
// caller owns it, so it is passed in rather than guessed at here.
func (c *Config) ContainerSpec(base string) container.Spec {
	spec := container.Spec{
		Image:   c.Container.Image,
		Base:    base,
		Network: c.Container.Network,
		User:    c.Container.User,
		Env:     c.Container.Env,
	}
	if c.Container.Build != "" {
		spec.Build = c.Resolve(c.Container.Build)
	}
	for _, mount := range c.Container.Mounts {
		spec.Mounts = append(spec.Mounts, container.Mount{
			Host:     c.Resolve(expandHome(mount.Source)),
			Target:   mount.Target,
			ReadOnly: mount.ReadOnly,
		})
	}
	return spec
}

func (c *ContainerConfig) applyDefaults() {
	if !c.Enabled() {
		return
	}
	if c.Runtime == "" {
		c.Runtime = container.Auto
	}
	if c.Scope == "" {
		c.Scope = ContainerScopeSetup
	}
	if c.User == "" {
		c.User = container.UserHost
	}
}

func (c ContainerConfig) validate() error {
	if c.Image != "" && c.Build != "" {
		// Which one won would decide what the trial actually measured, so the
		// file has to say.
		return fmt.Errorf("container: image and build cannot both be set")
	}
	if !c.Enabled() {
		// Every other field only means something once one of them is, and a
		// container section that named a scope but no image would otherwise
		// read as enabled.
		if c.Scope != "" || c.Runtime != "" || c.Network != "" || c.User != "" || len(c.Mounts) > 0 || len(c.Env) > 0 {
			return fmt.Errorf("container: one of image or build is required to run a trial in a container")
		}
		return nil
	}
	if !container.IsKnownRuntime(c.Runtime) {
		return fmt.Errorf("container.runtime: unknown runtime %q (one of: %s, %s)",
			c.Runtime, container.Auto, strings.Join(container.Runtimes, ", "))
	}
	if c.Scope != ContainerScopeSetup && c.Scope != ContainerScopeFull {
		return fmt.Errorf("container.scope must be %s", strings.Join(ContainerScopes, " or "))
	}
	for index, mount := range c.Mounts {
		if strings.TrimSpace(mount.Source) == "" {
			return fmt.Errorf("container.mounts[%d].source is required", index)
		}
		if strings.TrimSpace(mount.Target) == "" {
			return fmt.Errorf("container.mounts[%d].target is required", index)
		}
		if !path.IsAbs(filepath.ToSlash(mount.Target)) {
			return fmt.Errorf("container.mounts[%d].target must be an absolute path inside the container", index)
		}
		if withinMountPoint(mount.Target) {
			// The trial's own directory is already mounted there, and a mount
			// laid over it would hide the workspace mohae is about to grade.
			return fmt.Errorf("container.mounts[%d].target must not be inside %s, which holds the trial's workspace",
				index, container.MountPoint)
		}
	}
	return nil
}

func withinMountPoint(target string) bool {
	cleaned := path.Clean(filepath.ToSlash(target))
	return cleaned == container.MountPoint || strings.HasPrefix(cleaned, container.MountPoint+"/")
}

// expandHome turns a leading ~ into the user's home directory. It is applied
// to mount sources only: those are the paths a configuration writes from the
// home directory rather than from its own.
func expandHome(value string) string {
	if value != "~" && !strings.HasPrefix(value, "~/") {
		return value
	}
	home, err := userHomeDir()
	if err != nil || home == "" {
		// Left as written, so the failure is a mount the runtime rejects by
		// name rather than a silently different directory.
		return value
	}
	if value == "~" {
		return home
	}
	return filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(value, "~/")))
}

// defaultUserHomeDir is what userHomeDir is; tests replace the variable so ~
// expansion can be checked without depending on who is running them, and put
// this back afterwards.
var defaultUserHomeDir = os.UserHomeDir

var userHomeDir = defaultUserHomeDir
