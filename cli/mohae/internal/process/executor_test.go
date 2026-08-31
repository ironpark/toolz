package process

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestHostRunsCommandsWhereItIsTold(t *testing.T) {
	directory := t.TempDir()
	command := Shell(context.Background(), Host{}, "pwd; printf %s \"$MOHAE_TRIAL\"", directory, map[string]string{"MOHAE_TRIAL": "t"})
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	got := string(output)
	if !strings.Contains(got, "t") {
		t.Fatalf("the overlay did not reach the command: %q", got)
	}
	if !strings.Contains(got, directory) && !strings.Contains(got, os.TempDir()) {
		t.Fatalf("the command did not run in %q: %q", directory, got)
	}
}

func TestHostPathIsTheIdentity(t *testing.T) {
	// A host command already sees the host's filesystem, and a mapping here
	// would be a bug that only shows up as a missing file much later.
	host := Host{}
	if got := host.Path("/tmp/x"); got != "/tmp/x" {
		t.Fatalf("Path = %q", got)
	}
	if host.Contained() {
		t.Fatal("the host reports itself as contained")
	}
}

func TestOverlayKeepsOnlyWhatAnSDKAdded(t *testing.T) {
	// An SDK returns os.Environ() plus its own additions. Forwarding the whole
	// list into a container would carry this host's PATH and HOME, which name
	// directories that do not exist inside it.
	t.Setenv("MOHAE_OVERLAY_TEST", "inherited")
	built := append(os.Environ(), "MOHAE_ADDED=new", "MOHAE_OVERLAY_TEST=changed")

	overlay := Overlay(built)
	if got := overlay["MOHAE_ADDED"]; got != "new" {
		t.Errorf("added variable = %q", got)
	}
	if got := overlay["MOHAE_OVERLAY_TEST"]; got != "changed" {
		t.Errorf("changed variable = %q", got)
	}
	for key, value := range overlay {
		if current, ok := os.LookupEnv(key); ok && current == value {
			t.Errorf("%s was forwarded unchanged from this process", key)
		}
	}
}
