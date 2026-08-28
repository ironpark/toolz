package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/agentenv"
	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
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
	settings, root, err := config.Load(cwd)
	if err != nil {
		return err
	}
	if cmd.Bool("json") {
		return writeJSON(makeConfigJSON(settings, root))
	}

	if settings.Path == "" {
		fmt.Println("config_file: none (using defaults)")
	} else {
		fmt.Printf("config_file: %s\n", settings.Path)
	}
	fmt.Printf("repository_root: %s\n", root)
	fmt.Printf("agent: %s\n", agentenv.CurrentDescription())
	fmt.Printf("language: %s\n", settings.Language)
	printStringValues("plans_dirs", settings.PlansDirs)
	printStringValues("ignore", settings.Ignore)
	fmt.Println("hooks:")
	fmt.Printf("  timeout: %s\n", settings.Hooks.TimeoutDuration())
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

func printHookRules(name string, rules []hooks.Rule) {
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
