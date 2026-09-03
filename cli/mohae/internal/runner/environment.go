package runner

import (
	"maps"

	agentdriver "github.com/ironpark/toolz/cli/mohae/internal/driver"
	processutil "github.com/ironpark/toolz/cli/mohae/internal/process"
)

// trialEnv is the common environment overlay used by setup, agents, hooks and
// verification. Agent-specific code receives the resolved map and never needs
// to depend on the runner's Config type.
//
// The executor is a parameter because $MOHAE_WORKSPACE has to name the
// workspace as the command that reads it sees it. A containerised verify
// command and a host agent are looking at the same files under two different
// paths, and one of them handed the other's would be looking at nothing.
func trialEnv(config *Config, workspace *Workspace, executor processutil.Executor) map[string]string {
	env := map[string]string{
		"MOHAE_WORKSPACE": executor.Path(workspace.Root),
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
		Workspace:  workspace.Agent().Path(workspace.Root),
		Version:    version,
		Env:        trialEnv(config, workspace, workspace.Agent()),
		MCPServers: servers,
		OnText:     onText,
		Exec:       workspace.Agent(),
	}
}
