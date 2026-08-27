package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
)

func TestPlanNameCompletionValuesDiscoversAndSortsNames(t *testing.T) {
	active := t.TempDir()
	archive := t.TempDir()
	for _, value := range []struct {
		root string
		name string
	}{
		{active, "02-zeta"},
		{active, "00-alpha"},
		{archive, "01-alpha"},
		{archive, "not-a-plan"},
	} {
		if err := os.Mkdir(filepath.Join(value.root, value.name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := planNameCompletionValues([]string{active, archive}, "a")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "alpha" {
		t.Fatalf("completion values = %#v, want [alpha]", got)
	}
}

func TestRootEnablesShellCompletionAndCompletesPlanNames(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dir: plans\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "plans", "00-checkout-v2"), 0755); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	if !newRootCommand().EnableShellCompletion {
		t.Fatal("root command did not enable shell completion")
	}
	command := newRootCommand()
	var output bytes.Buffer
	command.Writer = &output
	if err := command.Run(context.Background(), []string{"planr", "status", "check", "--generate-shell-completion"}); err != nil {
		t.Fatalf("completion run failed: %v", err)
	}
	if got, want := output.String(), "checkout-v2\n"; got != want {
		t.Fatalf("completion output = %q, want %q", got, want)
	}
}
