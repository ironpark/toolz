package config

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestSandboxRejectsWhatCannotBeMeant(t *testing.T) {
	cases := map[string]string{
		// Both want to be the trial's executor, and which one won would decide
		// what the run measured.
		"with a container":    "container:\n  image: alpine\nsandbox:\n  enabled: true\n",
		"settings but off":    "sandbox:\n  scope: full\n",
		"allow_write but off": "sandbox:\n  allow_write: [./cache]\n",
		"unknown scope":       "sandbox:\n  enabled: true\n  scope: everything\n",
	}
	for name, section := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadConfig(writeConfig(t, minimalConfig+section)); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestSandboxDefaultsToConfiningOnlyTheSetup(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox is macOS-only")
	}
	config, err := LoadConfig(writeConfig(t, minimalConfig+"sandbox:\n  enabled: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	// setup rather than full: confining the agent changes what is being
	// measured, so it is asked for rather than assumed.
	if config.Sandbox.Scope != SandboxScopeSetup {
		t.Errorf("scope = %q, want %q", config.Sandbox.Scope, SandboxScopeSetup)
	}
	if config.Sandbox.AgentInside() {
		t.Error("AgentInside() = true at the default scope")
	}
	// An agent that cannot reach its API produces no result at all.
	if !config.Sandbox.AllowsNetwork() {
		t.Error("AllowsNetwork() = false by default")
	}
}

func TestSandboxSpecMakesTheTrialsOwnDirectoriesWritable(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox is macOS-only")
	}
	config, err := LoadConfig(writeConfig(t, minimalConfig+
		"sandbox:\n  enabled: true\n  allow_write: [./shared-cache]\n  deny_read: [~/.ssh]\n"))
	if err != nil {
		t.Fatal(err)
	}
	base, home := filepath.FromSlash("/tmp/base"), filepath.FromSlash("/home/tester")
	spec := config.SandboxSpec(base, home)

	for _, want := range []string{
		filepath.Join(base, "workspace"),
		filepath.Join(base, "scratch"),
		filepath.Join(base, "home"),
		filepath.Join(base, "tmp"),
		filepath.Join(home, ".cache"),
	} {
		if !slices.Contains(spec.Writable, want) {
			t.Errorf("%s is not writable: %v", want, spec.Writable)
		}
	}
	// The trial's own temporary directory, not the machine's: on macOS the
	// shared one contains the workspace, so allowing it would undo the sandbox.
	if spec.TempDir != filepath.Join(base, "tmp") {
		t.Errorf("TempDir = %q", spec.TempDir)
	}
	// allow_write is resolved against the configuration file like every other
	// configured path.
	if !slices.ContainsFunc(spec.Writable, func(path string) bool {
		return strings.HasSuffix(path, "shared-cache")
	}) {
		t.Errorf("allow_write was not applied: %v", spec.Writable)
	}
	if len(spec.DenyRead) != 1 || !strings.HasSuffix(spec.DenyRead[0], ".ssh") {
		t.Errorf("deny_read = %v", spec.DenyRead)
	}
}
