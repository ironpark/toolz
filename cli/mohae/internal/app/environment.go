package app

import (
	"maps"

	agentdriver "github.com/ironpark/toolz/cli/mohae/internal/driver"
)

// trialEnv is the common environment overlay used by setup, agents, hooks and
// verification. Agent-specific code receives the resolved map and never needs
// to depend on the runner's Config type.
func trialEnv(config *Config, workspace *Workspace) map[string]string {
	env := map[string]string{
		"MOHAE_WORKSPACE": workspace.Root,
		"MOHAE_TRIAL":     config.Name,
	}
	if config.Agent.Model != "" {
		env["MOHAE_MODEL"] = config.Agent.Model
	}
	if config.Agent.Effort != "" {
		env["MOHAE_EFFORT"] = config.Agent.Effort
	}
	maps.Copy(env, config.Agent.Env)
	return env
}

func newDriverOptions(config *Config, workspace *Workspace, servers []MCPServerSpec, onText func(string), version string) agentdriver.Options {
	return agentdriver.Options{
		Type:       config.Agent.Type,
		Model:      config.Agent.Model,
		Effort:     config.Agent.Effort,
		Command:    config.Agent.Command,
		Workspace:  workspace.Root,
		Version:    version,
		Env:        trialEnv(config, workspace),
		MCPServers: driverMCPServers(servers),
		OnText:     onText,
	}
}
