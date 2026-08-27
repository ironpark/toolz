package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const profiledConfig = minimalConfig + `profiles:
  claude:
    agent:
      type: claude-code
      model: claude-opus-5
  quick:
    limits:
      timeout_seconds: 60
`

func TestProfilesOverwriteSectionsWholesale(t *testing.T) {
	config, err := LoadConfig(writeConfig(t, profiledConfig))
	if err != nil {
		t.Fatal(err)
	}
	if err := config.ApplyProfile("claude"); err != nil {
		t.Fatal(err)
	}
	// The whole section is replaced, not merged: the base agent's fields do
	// not leak into a profile that redefined the section.
	if config.Agent.Type != "claude-code" || config.Agent.Model != "claude-opus-5" {
		t.Errorf("agent = %+v", config.Agent)
	}
	// Sections the profile does not declare keep their base values.
	if config.Workspace.Source != "./fixture" {
		t.Errorf("workspace = %+v", config.Workspace)
	}
	if config.Limits.TimeoutSeconds != DefaultTimeoutSeconds {
		t.Errorf("limits = %+v", config.Limits)
	}
}

func TestProfilesLayerInOrder(t *testing.T) {
	config, err := LoadConfig(writeConfig(t, profiledConfig))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"claude", "quick"} {
		if err := config.ApplyProfile(name); err != nil {
			t.Fatal(err)
		}
	}
	if config.Agent.Type != "claude-code" || config.Limits.TimeoutSeconds != 60 {
		t.Errorf("layered config = agent %+v limits %+v", config.Agent, config.Limits)
	}
}

func TestApplyProfileRejectsUnknownNames(t *testing.T) {
	config, err := LoadConfig(writeConfig(t, profiledConfig))
	if err != nil {
		t.Fatal(err)
	}
	// The error lists what exists so a typo is a one-round-trip fix.
	if err := config.ApplyProfile("caude"); err == nil || !strings.Contains(err.Error(), "claude") {
		t.Fatalf("err = %v, want one naming the known profiles", err)
	}

	bare, err := LoadConfig(writeConfig(t, minimalConfig))
	if err != nil {
		t.Fatal(err)
	}
	if err := bare.ApplyProfile("claude"); err == nil || !strings.Contains(err.Error(), "no profiles") {
		t.Fatalf("err = %v, want one saying no profiles are defined", err)
	}
}

func TestRunAppliesProfilesBeforeFlagOverrides(t *testing.T) {
	directory := chdir(t)
	if err := os.WriteFile(filepath.Join(directory, DefaultConfigName), []byte(profiledConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(DefaultConfigName)
	if err != nil {
		t.Fatal(err)
	}
	command := flagsOnly(newRunCommand())
	arguments := []string{"run", "--profile", "claude", "--timeout", "42"}
	if err := command.Run(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}
	if err := applyRunOverrides(command, []*Config{config}); err != nil {
		t.Fatal(err)
	}
	// The profile selected the agent; the flag still fine-tuned the limits on
	// top of it.
	if config.Agent.Type != "claude-code" || config.Limits.TimeoutSeconds != 42 {
		t.Errorf("config = agent %+v limits %+v", config.Agent, config.Limits)
	}
}
