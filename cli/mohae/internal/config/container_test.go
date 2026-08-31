package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/mohae/internal/container"
)

func containerConfig(t *testing.T, section ContainerConfig) *Config {
	t.Helper()
	config := &Config{
		Path:      filepath.Join(t.TempDir(), "mohae.config.yaml"),
		Name:      "trial",
		Agent:     AgentConfig{Type: "claude-code"},
		Workspace: WorkspaceConfig{Source: "./fixture"},
		Prompts:   []Prompt{{Text: "go"}},
		Container: section,
	}
	config.ApplyDefaults()
	return config
}

func TestContainerIsOffUntilAnImageIsNamed(t *testing.T) {
	// A configuration that never asked for a container must not need a
	// runtime installed to run.
	config := containerConfig(t, ContainerConfig{})
	if config.Container.Enabled() {
		t.Fatal("an empty container section reads as enabled")
	}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestContainerSettingsWithoutAnImageAreRejected(t *testing.T) {
	// Otherwise a config that names a scope but forgot the image would run on
	// the host and report nothing unusual.
	config := containerConfig(t, ContainerConfig{Scope: ContainerScopeFull})
	if err := config.Validate(); err == nil {
		t.Fatal("container settings without an image were accepted")
	}
}

func TestContainerRejectsBothImageAndBuild(t *testing.T) {
	config := containerConfig(t, ContainerConfig{Image: "golang:1.26", Build: "./docker"})
	if err := config.Validate(); err == nil {
		t.Fatal("image and build were accepted together")
	}
}

func TestContainerDefaultsKeepTheAgentOnTheHost(t *testing.T) {
	// Pinning the toolchain a trial is built and graded with is useful on its
	// own, and must not silently require the agent CLI to be in the image.
	config := containerConfig(t, ContainerConfig{Image: "golang:1.26"})
	if config.Container.Scope != ContainerScopeSetup {
		t.Fatalf("scope = %q, want %q", config.Container.Scope, ContainerScopeSetup)
	}
	if config.Container.AgentInside() {
		t.Fatal("the default scope puts the agent inside the container")
	}
	if config.Container.Runtime != container.Auto {
		t.Fatalf("runtime = %q, want %q", config.Container.Runtime, container.Auto)
	}
	if config.Container.User != container.UserHost {
		t.Fatalf("user = %q, want %q", config.Container.User, container.UserHost)
	}
}

func TestContainerRejectsUnknownScopeAndRuntime(t *testing.T) {
	for name, section := range map[string]ContainerConfig{
		"scope":   {Image: "img", Scope: "sandbox"},
		"runtime": {Image: "img", Runtime: "containerd"},
	} {
		config := containerConfig(t, section)
		if err := config.Validate(); err == nil {
			t.Errorf("%s: an unknown value was accepted", name)
		}
	}
}

func TestContainerRejectsAMountOverTheWorkspace(t *testing.T) {
	// The trial's own directory lives there, and a mount laid over it would
	// hide the workspace mohae is about to grade.
	config := containerConfig(t, ContainerConfig{
		Image:  "img",
		Mounts: []ContainerMount{{Source: "./cache", Target: container.MountPoint + "/workspace"}},
	})
	err := config.Validate()
	if err == nil || !strings.Contains(err.Error(), container.MountPoint) {
		t.Fatalf("a mount over the workspace was accepted: %v", err)
	}
}

func TestContainerRejectsARelativeMountTarget(t *testing.T) {
	config := containerConfig(t, ContainerConfig{
		Image:  "img",
		Mounts: []ContainerMount{{Source: "./cache", Target: "cache"}},
	})
	if err := config.Validate(); err == nil {
		t.Fatal("a relative mount target was accepted")
	}
}

func TestContainerSpecResolvesPathsAgainstTheConfiguration(t *testing.T) {
	userHomeDir = func() (string, error) { return filepath.FromSlash("/home/tester"), nil }
	t.Cleanup(func() { userHomeDir = defaultUserHomeDir })

	config := containerConfig(t, ContainerConfig{
		Build: "./docker",
		Mounts: []ContainerMount{
			{Source: "~/.claude", Target: "/agent/.claude", ReadOnly: true},
			{Source: "./cache", Target: "/cache"},
		},
	})
	directory := filepath.Dir(config.Path)
	spec := config.ContainerSpec("/tmp/mohae-run")

	if want := filepath.Join(directory, "docker"); spec.Build != want {
		t.Errorf("build = %q, want %q", spec.Build, want)
	}
	if spec.Base != "/tmp/mohae-run" {
		t.Errorf("base = %q", spec.Base)
	}
	// A credentials directory is named from the home directory far more often
	// than from the configuration's own.
	if want := filepath.FromSlash("/home/tester/.claude"); spec.Mounts[0].Host != want {
		t.Errorf("mounts[0].host = %q, want %q", spec.Mounts[0].Host, want)
	}
	if !spec.Mounts[0].ReadOnly {
		t.Error("mounts[0] is not read-only")
	}
	if want := filepath.Join(directory, "cache"); spec.Mounts[1].Host != want {
		t.Errorf("mounts[1].host = %q, want %q", spec.Mounts[1].Host, want)
	}
	// Relabelling changes SELinux labels on the host, so it is never applied
	// to a directory the configuration named.
	for index, mount := range spec.Mounts {
		if mount.Relabel {
			t.Errorf("mounts[%d] asks the runtime to relabel a configured path", index)
		}
	}
}

func TestContainerBuildAndMountsAreReferencedPaths(t *testing.T) {
	// `verify` checks what a configuration points at, and a Dockerfile that
	// is not there fails the trial as surely as a missing fixture.
	config := containerConfig(t, ContainerConfig{
		Build:  "./docker",
		Mounts: []ContainerMount{{Source: "./cache", Target: "/cache"}},
	})
	fields := map[string]bool{}
	for _, path := range config.ReferencedPaths() {
		fields[path.Field] = true
	}
	for _, field := range []string{"container.build", "container.mounts[0].source"} {
		if !fields[field] {
			t.Errorf("%s is not reported as a referenced path", field)
		}
	}
}
