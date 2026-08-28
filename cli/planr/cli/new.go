package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	ucli "github.com/urfave/cli/v3"
)

func newCommand(_ context.Context, cmd *ucli.Command) error {
	if cmd.NArg() < 1 || cmd.NArg() > 2 {
		return fmt.Errorf("new requires <plan-name> and a short description")
	}
	selector := cmd.Args().First()
	if strings.Contains(selector, "#") {
		if cmd.NArg() != 1 {
			return fmt.Errorf("phase draft selector must be the only positional argument")
		}
		return newPhaseCommand(cmd, selector)
	}
	return newPlanCommand(cmd)
}

func newPlanCommand(cmd *ucli.Command) error {
	name := cmd.Args().First()
	if !draft.KebabPattern.MatchString(name) {
		return fmt.Errorf("plan name %q must be lowercase kebab-case", name)
	}
	descriptionInput := cmd.String("description")
	if cmd.NArg() == 2 {
		if descriptionInput != "" {
			return fmt.Errorf("description must be provided either as the second argument or with --description, not both")
		}
		descriptionInput = cmd.Args().Get(1)
	}
	description, err := draft.RequireDescription(descriptionInput)
	if err != nil {
		return err
	}
	output := cmd.String("output")
	if output == "" {
		output = name + ".md"
	}
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if !cmd.Bool("json") {
		if _, err := os.Stat(absOutput); err == nil {
			return fmt.Errorf("draft file already exists: %s", absOutput)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	dependsOn, err := draft.NormalizeDependencies(cmd.StringSlice("depends-on"), name)
	if err != nil {
		return fmt.Errorf("invalid dependencies for plan %q: %w", name, err)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	settings, repoRoot, err := config.Load(workingDirectory)
	if err != nil {
		return err
	}
	settings = settings.WithSkipHooks(cmd.Bool("no-hooks"))
	if err := runDocumentHooks(repoRoot, settings, "before", hooks.EventNew, name, -1, "draft", cmd.Bool("json")); err != nil {
		return err
	}
	rendered, err := doc.RenderNewDraft(settings.Language, name, dependsOn, description)
	if err != nil {
		return err
	}
	if cmd.Bool("json") {
		if err := writeJSON(makeTemplateJSON("plan", name, rendered)); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(absOutput, []byte(rendered), 0644); err != nil {
			return err
		}
		fmt.Printf("Created %s\n", absOutput)
	}
	if err := runDocumentHooks(repoRoot, settings, "after", hooks.EventNew, name, -1, "draft", cmd.Bool("json")); err != nil {
		return err
	}
	return nil
}
