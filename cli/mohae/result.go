package main

import "time"

// TrialResult is everything one trial produced. It is the only thing the
// reports render and the only thing `compare` will have to read, so it carries
// the whole story: what was sent, what came back, what it cost, and what the
// verification made of the workspace afterwards.
//
// The JSON names are part of the file format written into report.dir, so they
// are spelled out rather than left to the field names.
type TrialResult struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ConfigPath  string `json:"config_path"`
	Agent       string `json:"agent"`
	Model       string `json:"model,omitempty"`

	StartedAt time.Time `json:"started_at"`
	// DurationSeconds covers the whole trial, setup and verification included:
	// what a run of this configuration actually costs in wall time.
	DurationSeconds float64 `json:"duration_seconds"`

	Turns  []TurnResult   `json:"turns"`
	Verify []VerifyResult `json:"verify,omitempty"`
	MCP    []MCPProbe     `json:"mcp,omitempty"`
	// ArtifactDir is the persistent copy made before the temporary workspace
	// was removed. Artifacts records what each configured pattern matched.
	ArtifactDir string           `json:"artifact_dir,omitempty"`
	Artifacts   []ArtifactResult `json:"artifacts,omitempty"`
	// ArtifactError is kept apart from Error for the same reason TimedOut is:
	// a trial whose conversation and verification both succeeded but whose
	// artifact copy failed did not fail the same way as one whose agent
	// errored.
	ArtifactError string `json:"artifact_error,omitempty"`

	Usage TokenUsage `json:"usage"`

	Passed bool `json:"passed"`
	// TimedOut reports that the trial's own limit stopped it. It is kept apart
	// from Error because a trial that ran out of time did not fail the same way
	// as one whose agent errored.
	TimedOut bool   `json:"timed_out,omitempty"`
	Error    string `json:"error,omitempty"`
	// Workspace is where the trial's directory was left. It is set only when
	// the trial failed: a passing trial's copy is deleted, and a failing one is
	// kept because there is no other way to see what the agent actually did.
	Workspace string `json:"workspace,omitempty"`
}

// ArtifactResult records the workspace-relative paths selected by one
// configured artifact pattern. An empty Paths list means the pattern matched
// nothing; artifact capture itself is observational and does not grade it.
type ArtifactResult struct {
	Pattern string   `json:"pattern"`
	Paths   []string `json:"paths,omitempty"`
}

// TurnResult is one prompt and what it produced. A prompt that was never sent
// is recorded too, with the reason: a conversation that silently shrank would
// make two runs of the same configuration look identical when they were not.
type TurnResult struct {
	Index  int    `json:"index"`
	Name   string `json:"name,omitempty"`
	Prompt string `json:"prompt"`
	Sent   bool   `json:"sent"`
	// Skipped says why an unsent prompt was skipped: its condition was false,
	// or a prompt it came after never ran.
	Skipped string `json:"skipped,omitempty"`

	Response        string     `json:"response,omitempty"`
	Model           string     `json:"model,omitempty"`
	Usage           TokenUsage `json:"usage,omitempty"`
	DurationSeconds float64    `json:"duration_seconds,omitempty"`
	Error           string     `json:"error,omitempty"`
}

// VerifyResult is one grading command's verdict. The exit status is the verdict
// and the output is kept verbatim: mohae records what the command printed but
// imposes no format on it.
type VerifyResult struct {
	Command         string  `json:"command"`
	ExitCode        int     `json:"exit_code"`
	Passed          bool    `json:"passed"`
	Output          string  `json:"output,omitempty"`
	DurationSeconds float64 `json:"duration_seconds"`
}

// Sent counts the turns that actually ran.
func (r TrialResult) Sent() int {
	sent := 0
	for _, turn := range r.Turns {
		if turn.Sent {
			sent++
		}
	}
	return sent
}

// VerifyPassed counts the grading commands that passed.
func (r TrialResult) VerifyPassed() int {
	passed := 0
	for _, check := range r.Verify {
		if check.Passed {
			passed++
		}
	}
	return passed
}

// EffectiveModel is the model a report should name: what actually answered —
// a fallback model is exactly the kind of thing a benchmark has to notice —
// falling back to what was configured when the agent reported nothing. It is
// derived from the turns rather than stored, so it reads the same from a fresh
// run and from a report loaded back off disk.
func (r TrialResult) EffectiveModel() string {
	for _, turn := range r.Turns {
		if turn.Model != "" {
			return turn.Model
		}
	}
	return r.Model
}

// Verdict is the one-word outcome a report leads with.
//
// A trial with no verify command is "ungraded", not "pass": nothing measured
// it, and calling that a pass would read as a task the agent completed. It is
// still counted as passing for the run's exit status, because a configuration
// that deliberately grades nothing has not failed either.
func (r TrialResult) Verdict() string {
	switch {
	case r.TimedOut:
		return "timeout"
	case !r.Passed:
		return "fail"
	case len(r.Verify) == 0:
		return "ungraded"
	default:
		return "pass"
	}
}
