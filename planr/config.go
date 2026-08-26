package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
	"github.com/goccy/go-yaml"
)

type config struct {
	PlansDir  string     `yaml:"plans_dir"`
	PlansDirs []string   `yaml:"plans_dirs"`
	Ignore    []string   `yaml:"ignore"`
	Hooks     hookConfig `yaml:"hooks"`
}

type hookConfig struct {
	Before []hookRule `yaml:"before"`
	After  []hookRule `yaml:"after"`
}

type hookRule struct {
	On  []string `yaml:"on"`
	Run string   `yaml:"run"`
}

func loadConfig(start string) (config, string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return config{}, "", err
	}
	for {
		path := filepath.Join(current, ".planr.yaml")
		contents, readErr := os.ReadFile(path)
		if readErr == nil {
			var value config
			if err := yaml.UnmarshalWithOptions(contents, &value, yaml.Strict()); err != nil {
				return config{}, "", fmt.Errorf("parse %s: %w", path, err)
			}
			if len(value.PlansDirs) == 0 && value.PlansDir != "" {
				value.PlansDirs = []string{value.PlansDir}
			}
			if len(value.PlansDirs) == 0 {
				value.PlansDirs = []string{"plan"}
			}
			if err := validatePlanDirs(value.PlansDirs); err != nil {
				return config{}, "", err
			}
			if err := validateHooks(value.Hooks); err != nil {
				return config{}, "", err
			}
			return value, current, nil
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return config{}, "", readErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return config{PlansDirs: []string{"plan"}}, start, nil
		}
		current = parent
	}
}

// planDirs resolves the configured plans directories against the repository root.
func (c config) planDirs(root string) []string {
	paths := make([]string, len(c.PlansDirs))
	for index, directory := range c.PlansDirs {
		paths[index] = filepath.Join(root, directory)
	}
	return paths
}

func planPaths(start string) ([]string, error) {
	value, root, err := loadConfig(start)
	if err != nil {
		return nil, err
	}
	return value.planDirs(root), nil
}

func validatePlanDirs(directories []string) error {
	seen := map[string]bool{}
	for _, directory := range directories {
		if directory == "" || filepath.IsAbs(directory) {
			return errors.New("plans_dirs entries must be non-empty relative paths")
		}
		if seen[directory] {
			return fmt.Errorf("plans_dirs contains duplicate path %q", directory)
		}
		seen[directory] = true
	}
	return nil
}

// ensureGitRepository fails fast when planr is run outside a git repository.
// Completion records are stored as git notes and the done-check reads the
// worktree status, so every plan operation assumes a repository is present.
func ensureGitRepository(start string) error {
	_, err := git.PlainOpenWithOptions(start, &git.PlainOpenOptions{DetectDotGit: true})
	if err == nil {
		return nil
	}
	if errors.Is(err, git.ErrRepositoryNotExists) {
		return fmt.Errorf("planr requires a git repository, but %s is not inside one; run `git init` at your project root first", start)
	}
	return fmt.Errorf("cannot open the git repository for %s: %w", start, err)
}
