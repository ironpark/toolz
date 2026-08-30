package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/jsonout"
	"github.com/ironpark/toolz/cli/planr/internal/notes"
	ucli "github.com/urfave/cli/v3"
)

func newNotesCommand() *ucli.Command {
	return &ucli.Command{
		Name:          "notes",
		Usage:         "list plan and phase completions linked to commits",
		ArgsUsage:     "[plan-name]",
		Flags:         []ucli.Flag{jsonFlag()},
		ShellComplete: planNameShellComplete,
		Action:        runNotes,
	}
}

func runNotes(_ context.Context, cmd *ucli.Command) error {
	if cmd.NArg() > 1 {
		return fmt.Errorf("notes command takes at most one plan name")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	_, repoRoot, err := config.Load(cwd)
	if err != nil {
		return err
	}

	records, err := notes.Read(repoRoot, cmd.Args().First())
	if err != nil {
		return err
	}
	if cmd.Bool("json") {
		return jsonout.Write(jsonout.Notes(records))
	}
	if len(records) == 0 {
		fmt.Println("no completions recorded")
		return nil
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "COMPLETED\tPLAN\tEVENT\tCOMMIT\tSUBJECT")
	for _, note := range records {
		event := note.Event
		if note.Phase != "" {
			event = fmt.Sprintf("%s %s", event, note.Phase)
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", note.At, note.Plan, event, note.ShortHash, note.Subject)
	}
	return writer.Flush()
}
