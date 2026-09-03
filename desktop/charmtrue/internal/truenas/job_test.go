package truenas

import (
	"errors"
	"testing"
)

func TestJobFailure(t *testing.T) {
	job := Job{ID: 42, State: "FAILED", Error: "dataset is busy", Exception: "trace"}
	err := job.Failure()
	var jobErr *JobError
	if !errors.As(err, &jobErr) || jobErr.ID != 42 || jobErr.Message != "dataset is busy" {
		t.Fatalf("Failure() = %#v", err)
	}
}

func TestSuccessfulJobHasNoFailure(t *testing.T) {
	if err := (Job{ID: 42, State: "SUCCESS"}).Failure(); err != nil {
		t.Fatalf("Failure() = %v", err)
	}
}
