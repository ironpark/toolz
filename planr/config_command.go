package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
)

func configCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 0 {
		return fmt.Errorf("config does not accept positional arguments")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	settings, root, err := loadConfig(cwd)
	if err != nil {
		return err
	}

	if settings.configPath == "" {
		fmt.Println("config_file: none (using defaults)")
	} else {
		fmt.Printf("config_file: %s\n", settings.configPath)
	}
	fmt.Printf("repository_root: %s\n", root)
	fmt.Printf("language: %s\n", settings.Language)
	printStringValues("plans_dirs", settings.PlansDirs)
	printStringValues("ignore", settings.Ignore)
	fmt.Println("hooks:")
	fmt.Printf("  timeout: %s\n", settings.Hooks.timeoutDuration())
	printHookRules("before", settings.Hooks.Before)
	printHookRules("after", settings.Hooks.After)
	return nil
}

func printStringValues(name string, values []string) {
	if len(values) == 0 {
		fmt.Printf("%s: []\n", name)
		return
	}
	fmt.Printf("%s:\n", name)
	for _, value := range values {
		fmt.Printf("  - %s\n", value)
	}
}

func printHookRules(name string, rules []hookRule) {
	if len(rules) == 0 {
		fmt.Printf("  %s: []\n", name)
		return
	}
	fmt.Printf("  %s:\n", name)
	for _, rule := range rules {
		fmt.Printf("    - on: [%s]\n", strings.Join(rule.On, ", "))
		fmt.Printf("      run: %s\n", rule.Run)
	}
}
