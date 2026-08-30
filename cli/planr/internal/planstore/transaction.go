// Package planstore applies coordinated changes to the documents that make up
// one plan. A plan is stored in several files, so a failed later write must
// restore every earlier write before the caller releases its plan lock.
package planstore

import (
	"errors"
	"fmt"
	"os"

	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/vfs"
)

type Change struct {
	Path   string
	before *string
	after  *string
}

func Create(path, contents string) Change {
	return Change{Path: path, after: &contents}
}

func Update(path, before, after string) Change {
	return Change{Path: path, before: &before, after: &after}
}

func Delete(path, contents string) Change {
	return Change{Path: path, before: &contents}
}

func Apply(changes ...Change) error {
	for index, change := range changes {
		if err := apply(change); err != nil {
			if rollbackErr := rollback(changes[:index]); rollbackErr != nil {
				return fmt.Errorf("apply %s: %w; rollback: %v", change.Path, err, rollbackErr)
			}
			return fmt.Errorf("apply %s: %w", change.Path, err)
		}
	}
	return nil
}

func apply(change Change) error {
	if change.after == nil {
		return vfs.Remove(change.Path)
	}
	return mdoc.WriteAtomically(change.Path, *change.after)
}

func rollback(changes []Change) error {
	for index := len(changes) - 1; index >= 0; index-- {
		change := changes[index]
		if change.before == nil {
			if err := vfs.Remove(change.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove %s: %w", change.Path, err)
			}
			continue
		}
		if err := mdoc.WriteAtomically(change.Path, *change.before); err != nil {
			return fmt.Errorf("restore %s: %w", change.Path, err)
		}
	}
	return nil
}
