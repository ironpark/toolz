package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/urfave/cli/v3"
)

func TestArchiveMovesCompletedPlanAndPreservesNumbering(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dirs: [plans-active, plans-archive]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(root, "plans-active")
	archive := filepath.Join(root, "plans-archive")
	if err := os.MkdirAll(archive, 0755); err != nil {
		t.Fatal(err)
	}
	planRoot := filepath.Join(active, "00-checkout-v2")
	if err := plan.Write(planRoot, testDraft(), "00-checkout-v2", doc.English); err != nil {
		t.Fatal(err)
	}
	if _, _, err := updatePhaseStatus([]string{active}, "checkout-v2", 0, "done"); err != nil {
		t.Fatalf("complete plan: %v", err)
	}

	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	command := &cli.Command{Name: "archive", Action: archiveCommand}
	if err := command.Run(context.Background(), []string{"archive", "checkout-v2"}); err != nil {
		t.Fatalf("archive failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(active, "00-checkout-v2")); !os.IsNotExist(err) {
		t.Fatalf("active plan still exists; stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(archive, "00-checkout-v2")); err != nil {
		t.Fatalf("archived plan missing: %v", err)
	}
	next, err := plan.NextDirectory([]string{active, archive}, "follow-up")
	if err != nil {
		t.Fatal(err)
	}
	if next != "01-follow-up" {
		t.Fatalf("next plan directory = %q, want 01-follow-up", next)
	}
}

func TestArchiveRefusesOpenPlan(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dirs: [active, archive]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "active", "00-open"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "active", "00-open", "PLAN.md"), []byte("---\nplan_status: in-progress\n---\n"), 0644); err != nil {
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
	err = (&cli.Command{Name: "archive", Action: archiveCommand}).Run(context.Background(), []string{"archive", "open"})
	if err == nil || !strings.Contains(err.Error(), "not done") {
		t.Fatalf("archive open plan error = %v, want not done", err)
	}
}
