package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/ppwk/cmd"
)

func TestRunVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run(context.Background(), []string{"ppwk", "--version"}, &stdout, &stderr); code != cmd.ExitOK {
		t.Fatalf("exit code = %d, want %d (stderr %q)", code, cmd.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "dev") {
		t.Fatalf("stdout = %q, want the version", stdout.String())
	}
}

func TestRunWithoutCommandShowsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer

	run(context.Background(), []string{"ppwk"}, &stdout, &stderr)

	if !strings.Contains(stdout.String(), "USAGE") {
		t.Fatalf("stdout = %q, want usage", stdout.String())
	}
}

func TestUnknownFlagIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run(context.Background(), []string{"ppwk", "--nope"}, &stdout, &stderr); code != cmd.ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, cmd.ExitUsage)
	}
}

func TestUnarchiveIsRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run(context.Background(), []string{"ppwk", "unarchive", "T001"}, &stdout, &stderr); code != cmd.ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, cmd.ExitUsage)
	}
	if !strings.Contains(stderr.String(), "v1") {
		t.Fatalf("stderr = %q, want the v1 notice", stderr.String())
	}
}
