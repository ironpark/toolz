// Package apply turns planr's authoring documents — plan drafts, new-phase
// drafts and edit checkouts — into on-disk plan documents. The command layer
// only decides where the raw bytes come from and how the result is printed.
package apply

import (
	"path/filepath"
	"sort"
)

// Document kinds recognised by Detect.
const (
	KindPlan  = "plan"
	KindPhase = "phase"
	KindEdit  = "edit"
)

// Diff records the before/after contents of one document an operation touches.
// Before is empty for documents the operation creates.
type Diff struct {
	Path   string
	Before string
	After  string
}

// Operation is the result of applying a document: what it did, and which
// documents it wrote (or would write, for a dry run).
type Operation struct {
	Action    string
	Selector  string
	DryRun    bool
	Changed   bool
	Documents map[string]string
	Diffs     []Diff
}

func makeOperation(action, selector string, dryRun bool, documents map[string]string, diffs []Diff) Operation {
	changed := false
	for _, diff := range diffs {
		if diff.Before != diff.After {
			changed = true
			break
		}
	}
	return Operation{Action: action, Selector: selector, DryRun: dryRun, Changed: changed, Documents: documents, Diffs: diffs}
}

func documentsWithRoot(root string, documents map[string]string) map[string]string {
	result := make(map[string]string, len(documents))
	for path, contents := range documents {
		result[absolutePath(filepath.Join(root, filepath.FromSlash(path)))] = contents
	}
	return result
}

func newDocumentDiffs(root string, documents map[string]string) []Diff {
	paths := make([]string, 0, len(documents))
	for path := range documents {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]Diff, 0, len(paths))
	for _, path := range paths {
		result = append(result, Diff{Path: absolutePath(filepath.Join(root, filepath.FromSlash(path))), After: documents[path]})
	}
	return result
}

func absolutePath(path string) string {
	value, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return value
}
