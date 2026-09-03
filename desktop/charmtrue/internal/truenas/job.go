package truenas

import (
	"context"
	"encoding/json"
	"strings"
)

// Job is the common shape returned by core.get_jobs.
type Job struct {
	ID        int             `json:"id"`
	Method    string          `json:"method"`
	Arguments []any           `json:"arguments"`
	State     string          `json:"state"`
	Progress  json.RawMessage `json:"progress"`
	Result    json.RawMessage `json:"result"`
	Error     string          `json:"error"`
	Exception string          `json:"exception"`
}

// Failure converts a failed or aborted job to a structured error.
func (j Job) Failure() error {
	state := strings.ToUpper(j.State)
	if state != "FAILED" && state != "ABORTED" {
		return nil
	}
	return &JobError{ID: j.ID, State: state, Message: j.Error, Exception: j.Exception}
}

// Jobs queries core.get_jobs.
func (c *Client) Jobs(ctx context.Context, filters []Filter, options QueryOptions) ([]Job, error) {
	return Query[Job](ctx, c, "core.get_jobs", filters, options)
}

// WaitJob waits for a job and decodes its final result.
func (c *Client) WaitJob(ctx context.Context, id int, result any) error {
	if id < 1 {
		return &ValidationError{Field: "job id", Message: "must be positive"}
	}
	return c.Call(ctx, "core.job_wait", []any{id}, result)
}

// AbortJob requests cancellation of a running job.
func (c *Client) AbortJob(ctx context.Context, id int) error {
	if id < 1 {
		return &ValidationError{Field: "job id", Message: "must be positive"}
	}
	return c.Call(ctx, "core.job_abort", []any{id}, nil)
}
