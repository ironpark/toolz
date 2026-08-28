package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/gitrepo"
	"github.com/urfave/cli/v3"
)

// initCommand writes .planr.yaml and the configured plans directories so a
// repository is ready for `planr new`. Every setting it writes is already a
// default, so the file exists to be edited and to mark the repository root,
// not to change behaviour on the day it is created.
func initCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 0 {
		return fmt.Errorf("init does not accept positional arguments")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, existing, repositoryErr, err := initTarget(cwd)
	if err != nil {
		return err
	}
	target := filepath.Join(root, ".planr.yaml")

	if existing != "" && !cmd.Bool("force") {
		return fmt.Errorf("configuration already exists: %s; pass --force to overwrite it", existing)
	}
	// The search walks upwards, so a config in a parent directory is found from
	// a subdirectory. Writing there would silently reconfigure the parent
	// project instead of this one.
	if existing != "" && existing != target {
		return fmt.Errorf("configuration for this repository lives at %s, not %s", existing, target)
	}

	language := doc.NormalizeLanguage(cmd.String("language"))
	if err := doc.ValidateLanguage(language); err != nil {
		return err
	}
	plansDirs := cmd.StringSlice("plans-dir")
	if len(plansDirs) == 0 {
		plansDirs = config.Default().PlansDirs
	}
	if err := config.ValidatePlanDirs(plansDirs); err != nil {
		return err
	}

	// Warned here rather than on entry: a run that goes on to refuse for some
	// other reason writes nothing, and "writing it anyway" would be false.
	if repositoryErr != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", repositoryErr)
		fmt.Fprintf(os.Stderr, "warning: writing the configuration anyway; every other planr command will fail until this is a git repository\n")
	}

	created := make([]string, 0, len(plansDirs)+1)
	existed := make([]string, 0, len(plansDirs))
	for _, directory := range plansDirs {
		path := filepath.Join(root, directory)
		switch _, statErr := os.Stat(path); {
		case statErr == nil:
			existed = append(existed, path)
			continue
		case !errors.Is(statErr, os.ErrNotExist):
			return statErr
		}
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}
		created = append(created, path)
	}
	if err := os.WriteFile(target, []byte(configTemplate(language, plansDirs)), 0644); err != nil {
		return err
	}
	created = append(created, target)

	if cmd.Bool("json") {
		return writeJSON(makeInitJSON(target, root, language, plansDirs, created, existed))
	}
	for _, path := range created {
		fmt.Printf("created %s\n", path)
	}
	for _, path := range existed {
		fmt.Printf("exists  %s\n", path)
	}
	fmt.Printf("\nWrite your first plan with:\n  planr new <plan-name> \"<description>\"\n")
	return nil
}

// initTarget decides which directory this repository is configured from and
// reports the configuration file already in effect there, if any. A non-nil
// repositoryErr means there is no git repository yet; init still writes the
// file, because a project is often configured before `git init`, but the caller
// has to say so -- planr keeps completion records as git notes, so every other
// command needs one.
func initTarget(cwd string) (root string, existing string, repositoryErr error, err error) {
	if repositoryErr = gitrepo.EnsureRepository(cwd); repositoryErr != nil {
		// Only this directory's own file counts: without a repository the
		// upward search has no boundary, and an unrelated .planr.yaml in some
		// ancestor must not decide anything here.
		path := filepath.Join(cwd, ".planr.yaml")
		if _, statErr := os.Stat(path); statErr == nil {
			return cwd, path, repositoryErr, nil
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", "", repositoryErr, statErr
		}
		return cwd, "", repositoryErr, nil
	}
	// config.Discover rather than config.Load: an existing file that no longer
	// parses still has to be detected here, otherwise `init --force` -- the one
	// command that could repair it -- would fail on the file it is replacing.
	location, err := config.Discover(cwd)
	if err != nil {
		return "", "", nil, err
	}
	return location.BaseRoot, location.Path, nil, nil
}

// configTemplate renders .planr.yaml with the settings init was asked for and
// comments for the ones it leaves out. The commented-out blocks are the
// settings people reach for next; discovering them requires the documentation
// otherwise.
func configTemplate(language string, plansDirs []string) string {
	var builder strings.Builder
	builder.WriteString("# planr configuration. See `planr config` for the values in effect.\n\n")
	builder.WriteString("# Language of the plan documents planr writes. Command output and errors\n")
	fmt.Fprintf(&builder, "# stay in English. Supported: %s\n", strings.Join(doc.SortedLanguages(), ", "))
	fmt.Fprintf(&builder, "language: %s\n\n", language)
	builder.WriteString("# Where plans are stored, relative to this file. List more than one to keep\n")
	builder.WriteString("# active and archived plans apart.\n")
	builder.WriteString("plans_dirs:\n")
	for _, directory := range plansDirs {
		fmt.Fprintf(&builder, "  - %s\n", directory)
	}
	builder.WriteString("\n# Paths the done-check ignores, so build output does not make a phase look\n")
	builder.WriteString("# unfinished.\n")
	builder.WriteString("# ignore:\n")
	builder.WriteString("#   - bin/**\n")
	builder.WriteString("\n# Commands to run around planr operations.\n")
	builder.WriteString("# hooks:\n")
	builder.WriteString("#   timeout: 10m\n")
	builder.WriteString("#   after:\n")
	builder.WriteString("#     - on: [done]\n")
	builder.WriteString("#       run: go test ./...\n")
	return builder.String()
}
