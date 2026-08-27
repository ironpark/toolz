package main

import (
	"fmt"
	"strings"

	"github.com/ironpark/toolz/cli/planr/agentenv"
)

// planr reports the AI coding agent it is running under in two places: as
// environment variables handed to repository hooks, and as a diagnostic line in
// `config` and `doctor`. Both derive from the same detection so a hook and the
// diagnostics can never disagree about the environment.

// agentEnvironment returns the agent variables exported to hook commands. They
// are always present and empty when planr was invoked from a plain shell, so a
// hook can test them without worrying about unset variables.
func agentEnvironment() []string {
	detection := agentenv.Detect()
	return []string{
		"PLANR_AGENT=" + string(detection.Agent),
		"PLANR_AGENT_SESSION=" + detection.SessionID,
		"PLANR_AGENT_LEVEL=" + detection.Level.String(),
	}
}

// currentAgentDescription renders the detected environment for `config` and
// `doctor`.
func currentAgentDescription() string {
	return describeAgent(agentenv.Detect())
}

// describeAgent names the agent, the variable that gave it away, and the
// session it belongs to. The signal is included because it is what a reader
// needs to reproduce or suppress the detection; the session id ties the run
// back to the agent transcript that produced it.
func describeAgent(detection agentenv.Detection) string {
	if !detection.Detected() {
		return "none (no AI agent environment detected)"
	}
	fields := []string{
		"signal=" + detection.Signal,
		"level=" + detection.Level.String(),
	}
	if detection.SessionID != "" {
		fields = append(fields, "session="+detection.SessionID)
	}
	name := string(detection.Agent)
	if name == "" {
		name = "unknown"
	}
	return fmt.Sprintf("%s (%s)", name, strings.Join(fields, ", "))
}
