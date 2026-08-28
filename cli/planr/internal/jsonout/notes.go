package jsonout

import (
	"github.com/ironpark/toolz/cli/planr/internal/notes"
)

type NotesOutput struct {
	Notes []note `json:"notes"`
}

type note struct {
	CompletedAt string `json:"completed_at"`
	Plan        string `json:"plan"`
	Event       string `json:"event"`
	Phase       string `json:"phase"`
	Commit      string `json:"commit"`
	ShortCommit string `json:"short_commit"`
	Subject     string `json:"subject"`
}

func Notes(records []notes.Note) NotesOutput {
	values := make([]note, 0, len(records))
	for _, record := range records {
		values = append(values, note{
			CompletedAt: record.At,
			Plan:        record.Plan,
			Event:       record.Event,
			Phase:       record.Phase,
			Commit:      record.Commit,
			ShortCommit: record.ShortHash,
			Subject:     record.Subject,
		})
	}
	return NotesOutput{Notes: values}
}
