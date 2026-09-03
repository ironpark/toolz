package config

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/ironpark/toolz/cli/mohae/internal/sandbox"
)

// Sandbox scopes, spelled like the container's so a configuration that moves
// between the two does not have to learn a second vocabulary.
//
// setup confines everything mohae runs on the trial's behalf — the setup
// script, the hooks and the verification commands — while the agent runs
// unconfined. full adds the agent, which is what makes two agents measurable
// under one rule: codex bounds its own writes and claude-code does not, and
// this is the cheap way to close that gap.
const (
	SandboxScopeSetup = "setup"
	SandboxScopeFull  = "full"
)

// SandboxScopes is stable for diagnostics.
var SandboxScopes = []string{SandboxScopeSetup, SandboxScopeFull}

// SandboxConfig confines a trial's writes without moving it off this machine.
//
// It is deliberately not a cheaper container. A container decides which
// toolchain a trial is built and graded with, so a run of one configuration
// means the same thing on another machine; a sandbox leaves the toolchain as it
// found it and only bounds where the trial may write. Comparing two agents on
// one machine is what it is for. Comparing results across machines still wants
// a container.
type SandboxConfig struct {
	// Enabled turns the sandbox on. Unlike the container — which is implied by
	// naming an image — there is nothing else here that has to be set, so it
	// has to be said outright.
	Enabled bool `yaml:"enabled,omitempty"`
	// Scope decides whether the agent is confined too. See the scope constants.
	Scope string `yaml:"scope,omitempty"`
	// AllowWrite are extra paths the trial may write to, on top of the
	// workspace and the defaults below. A build cache shared with the rest of
	// the machine is the usual reason to set it.
	AllowWrite []string `yaml:"allow_write,omitempty"`
	// DenyRead hides paths from the trial. Nothing is hidden by default: the
	// sandbox exists so the trial can use what is installed here. This is for a
	// configuration that would rather an agent could not read the credentials
	// in the home directory it is running under.
	DenyRead []string `yaml:"deny_read,omitempty"`
	// Network allows the trial to reach the network, and defaults to allowing
	// it: an agent that cannot reach its API produces no result at all. Turning
	// it off with scope full does exactly that, so it is meant for scope setup,
	// where it makes a verification that quietly downloads something fail here
	// rather than depend on what it found.
	Network *bool `yaml:"network,omitempty"`
}

// defaultWritableHomeDirs are the directories under the trial's home that stay
// writable. An agent CLI keeps its session and its cache here, and a package
// manager invoked by a setup script keeps its downloads here; denying them
// would not confine the trial so much as stop it running. None of them is
// where an agent would leave the work it was asked to produce, which is the
// thing the sandbox is guarding against.
var defaultWritableHomeDirs = []string{
	".cache", ".config", ".local", ".npm", ".claude", ".codex",
}

// AgentInside reports whether the agent under test is confined as well.
func (s SandboxConfig) AgentInside() bool {
	return s.Enabled && s.Scope == SandboxScopeFull
}

// AllowsNetwork reports whether the trial may reach the network.
func (s SandboxConfig) AllowsNetwork() bool {
	return s.Network == nil || *s.Network
}

// SandboxSpec builds the sandbox for a trial whose own directories are under
// base. workspace, scratch and home are always writable; everything else the
// trial may write to has to be named.
func (c *Config) SandboxSpec(base, home string) sandbox.Spec {
	writable := []string{
		filepath.Join(base, "workspace"),
		filepath.Join(base, "scratch"),
		filepath.Join(base, "home"),
	}
	// The trial's own, not the machine's: on macOS the shared temporary
	// directory is the per-user one under /var/folders, which is also where
	// this trial's workspace lives, so allowing it would undo the sandbox.
	temporary := filepath.Join(base, "tmp")
	writable = append(writable, temporary)
	for _, name := range defaultWritableHomeDirs {
		writable = append(writable, filepath.Join(home, name))
	}
	for _, path := range c.Sandbox.AllowWrite {
		writable = append(writable, c.Resolve(expandHome(path)))
	}
	denied := make([]string, 0, len(c.Sandbox.DenyRead))
	for _, path := range c.Sandbox.DenyRead {
		denied = append(denied, c.Resolve(expandHome(path)))
	}
	return sandbox.Spec{
		Writable: writable,
		DenyRead: denied,
		Network:  c.Sandbox.AllowsNetwork(),
		TempDir:  temporary,
	}
}

func (c *Config) validateSandbox() error {
	s := c.Sandbox
	if !s.Enabled {
		// The other fields are only meaningful with the sandbox on, and a
		// configuration that set them and left it off is one whose author
		// expected confinement they are not getting.
		switch {
		case s.Scope != "", len(s.AllowWrite) > 0, len(s.DenyRead) > 0, s.Network != nil:
			return fmt.Errorf("sandbox: enabled is false, so the rest of the section has no effect")
		}
		return nil
	}
	if c.Container.Enabled() {
		// Both would want to be the trial's executor, and which one won would
		// decide what the run measured.
		return fmt.Errorf("sandbox and container cannot both be enabled: a container already bounds what the trial can reach")
	}
	if s.Scope != "" && !slices.Contains(SandboxScopes, s.Scope) {
		return fmt.Errorf("sandbox.scope: unknown scope %q (one of: %v)", s.Scope, SandboxScopes)
	}
	// Checked at load time rather than when the trial starts: a machine that
	// cannot sandbox should say so before the run spends anything.
	if err := sandbox.Available(); err != nil {
		return fmt.Errorf("sandbox: %w", err)
	}
	return nil
}

func (c *Config) applySandboxDefaults() {
	if c.Sandbox.Enabled && c.Sandbox.Scope == "" {
		c.Sandbox.Scope = SandboxScopeSetup
	}
}
