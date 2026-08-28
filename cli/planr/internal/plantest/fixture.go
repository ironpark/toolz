package plantest

import (
	"embed"
	"fmt"
	"io/fs"

	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
)

// CheckoutFixture is the draft document the scenario test registers as several
// plans. It stays a real document rather than a Go literal because it is also
// what an author writes by hand, so a change to the draft format has to keep
// this file loadable.
const CheckoutFixture = "checkout-v2.md"

//go:embed fixtures
var fixtures embed.FS

// Fixtures returns the draft fixtures as an io/fs tree. Tests take an fs.FS
// rather than a directory path so they read the fixtures the same way whether
// they come from this package or from a tree the test builds itself.
func Fixtures() fs.FS {
	tree, err := fs.Sub(fixtures, "fixtures")
	if err != nil {
		// fs.Sub on an embedded directory can only fail on an invalid name,
		// which is fixed at compile time by the go:embed directive above.
		panic(err)
	}
	return tree
}

// DraftBody returns the named draft with its frontmatter removed, so one
// fixture body can be registered under several plan names and dependencies.
// A fixture that lost its frontmatter is an error rather than a whole-file
// body: it would otherwise be registered under the draft's own name.
func DraftBody(fsys fs.FS, name string) (string, error) {
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", err
	}
	front, body, err := mdoc.Split(string(raw))
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	if len(front) == 0 {
		return "", fmt.Errorf("%s: draft has no frontmatter", name)
	}
	return body, nil
}

// DraftDocument writes the frontmatter for one plan in front of a draft body,
// producing the document `planr apply` reads. An empty dependency list is
// pruned by mdoc.Render, which keeps the input identical to what planr writes
// back out.
func DraftDocument(planName string, dependsOn []string, body string) (string, error) {
	return mdoc.Render(map[string]any{"plan_name": planName, "depends_on": dependsOn}, body)
}
