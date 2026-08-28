package cli

import (
	"context"
	"fmt"
	"github.com/ironpark/toolz/cli/planr/internal/apply"
	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/jsonout"
	"github.com/ironpark/toolz/cli/planr/internal/validation"
	"github.com/ironpark/toolz/cli/planr/internal/vfs"
	ucli "github.com/urfave/cli/v3"
	"io"
	"os"
)

func applyCommand(_ context.Context, cmd *ucli.Command) error {
	if cmd.NArg() > 1 {
		return applyCommandError(cmd, fmt.Errorf("apply accepts one document file or --stdin"))
	}
	if cmd.Bool("stdin") && cmd.NArg() != 0 {
		return applyCommandError(cmd, fmt.Errorf("apply accepts either a document file or --stdin, not both"))
	}
	if !cmd.Bool("stdin") && cmd.NArg() == 0 {
		return applyCommandError(cmd, fmt.Errorf("apply requires <document-file> or --stdin"))
	}

	var (
		raw      []byte
		fallback string
		err      error
	)
	if cmd.Bool("stdin") {
		raw, err = io.ReadAll(os.Stdin)
		raw = apply.UnwrapJSONDocument(raw)
	} else {
		fallback = cmd.Args().First()
		raw, err = vfs.ReadFile(fallback)
	}
	if err != nil {
		return applyCommandError(cmd, err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return applyCommandError(cmd, err)
	}
	settings, repoRoot, err := config.Load(cwd)
	if err != nil {
		return applyCommandError(cmd, err)
	}
	settings = settings.WithSkipHooks(cmd.Bool("no-hooks"))

	kind, document, err := apply.Detect(raw, fallback)
	if err != nil {
		return applyCommandError(cmd, err)
	}
	var operation apply.Operation
	switch kind {
	case apply.KindPlan:
		operation, err = apply.Plan(document.(draft.Draft), settings, repoRoot, cmd.Bool("dry-run"), progressWriter(cmd))
	case apply.KindPhase:
		operation, err = apply.Phase(document.(apply.PhaseDraft), settings, repoRoot, cmd.Bool("dry-run"), progressWriter(cmd))
	case apply.KindEdit:
		operation, err = apply.Edit(raw, settings, repoRoot, cmd.Bool("dry-run"), progressWriter(cmd))
	default:
		err = fmt.Errorf("unsupported document kind %q", kind)
	}
	if err != nil {
		return applyCommandError(cmd, err)
	}
	if cmd.Bool("json") {
		return jsonout.Write(jsonout.Apply(operation))
	}
	if operation.DryRun {
		printApplyDryRun(operation)
	}
	return nil
}

func printApplyDryRun(operation apply.Operation) {
	if !operation.Changed {
		fmt.Printf("Would leave %s unchanged\n", operation.Selector)
		return
	}
	for _, diff := range operation.Diffs {
		fmt.Printf("Would update %s\n", diff.Path)
	}
}

func applyCommandError(cmd *ucli.Command, err error) error {
	if !cmd.Bool("json") {
		return err
	}
	records := validation.Records(err)
	if len(records) == 0 {
		records = []validation.Record{{Rule: "document", Detail: err.Error()}}
	}
	if writeErr := jsonout.Write(jsonout.ApplyFailureOutput{Ok: false, Errors: jsonout.Validation(records)}); writeErr != nil {
		return writeErr
	}
	return err
}
