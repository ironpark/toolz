package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/apply"
	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/ironpark/toolz/cli/planr/internal/jsonout"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
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
	if err := hooks.RunTo(repoRoot, settings.Hooks, settings.SkipHooks, "before", hooks.EventNew, name, -1, "draft", progressWriter(cmd)); err != nil {
		return err
	}
	rendered, err := doc.RenderNewDraft(settings.Language, name, dependsOn, description)
	if err != nil {
		return err
	}
	if cmd.Bool("json") {
		if err := jsonout.Write(jsonout.Template("plan", name, rendered)); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(absOutput, []byte(rendered), 0644); err != nil {
			return err
		}
		fmt.Printf("Created %s\n", absOutput)
	}
	if err := hooks.RunTo(repoRoot, settings.Hooks, settings.SkipHooks, "after", hooks.EventNew, name, -1, "draft", progressWriter(cmd)); err != nil {
		return err
	}
	return nil
}

func newPhaseCommand(cmd *ucli.Command, selector string) error {
	planArg, title, ok := strings.Cut(selector, "#")
	if !ok || planArg == "" || title == "" || strings.Contains(title, "#") {
		return fmt.Errorf("new phase requires <plan-name>#<phase-name>")
	}
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("phase title must not be empty")
	}
	if strings.ContainsAny(title, "\r\n") {
		return fmt.Errorf("phase title must be a single line")
	}
	if len(cmd.StringSlice("depends-on")) > 0 || strings.TrimSpace(cmd.String("description")) != "" {
		return fmt.Errorf("phase draft fields belong in the draft; do not pass plan description or dependency flags")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	settings, repoRoot, err := config.Load(cwd)
	if err != nil {
		return err
	}
	settings = settings.WithSkipHooks(cmd.Bool("no-hooks"))
	planDirectories := settings.PlanDirs(repoRoot)
	planRoot, planDirectory, err := plan.FindDirectory(planDirectories, planArg)
	if err != nil {
		return err
	}
	done, err := plan.AlreadyDone(planRoot)
	if err != nil {
		return err
	}
	if done {
		return fmt.Errorf("plan %q is already done; new phase drafts are only allowed for open plans", planDirectory)
	}

	slug := plan.SlugifyTitle(strings.TrimSpace(title))
	if slug == "" {
		// The draft remains editable, and the author can replace this placeholder
		// with a valid ASCII slug before applying a non-ASCII title.
		slug = "phase"
	}
	output := cmd.String("output")
	if output == "" {
		output = draft.PlanName(planDirectory) + "-" + slug + ".md"
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
	rendered, err := doc.RenderNewPhaseDraft(settings.Language, draft.PlanName(planDirectory), strings.TrimSpace(title), slug)
	if err != nil {
		return err
	}
	if err := hooks.RunTo(repoRoot, settings.Hooks, settings.SkipHooks, "before", hooks.EventNew, planDirectory, -1, "draft", progressWriter(cmd)); err != nil {
		return err
	}
	if cmd.Bool("json") {
		if err := jsonout.Write(jsonout.Template(apply.KindPhase, draft.PlanName(planDirectory)+"#"+strings.TrimSpace(title), rendered)); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(absOutput, []byte(rendered), 0644); err != nil {
			return err
		}
		fmt.Printf("Created %s\n", absOutput)
	}
	return hooks.RunTo(repoRoot, settings.Hooks, settings.SkipHooks, "after", hooks.EventNew, planDirectory, -1, "draft", progressWriter(cmd))
}
