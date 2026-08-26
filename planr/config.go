package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

type config struct {
	PlansDir  string   `yaml:"plans_dir"`
	PlansDirs []string `yaml:"plans_dirs"`
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
			if err := yaml.Unmarshal(contents, &value); err != nil {
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

func planPaths(start string) ([]string, error) {
	value, root, err := loadConfig(start)
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(value.PlansDirs))
	for index, directory := range value.PlansDirs {
		paths[index] = filepath.Join(root, directory)
	}
	return paths, nil
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
