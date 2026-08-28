package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run([]string{"--version"}, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got, want := stdout.String(), "skpt dev\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunWithoutCommandShowsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("run() error = nil, want an error")
	}
	if !strings.Contains(stderr.String(), "사용법: skpt") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}
