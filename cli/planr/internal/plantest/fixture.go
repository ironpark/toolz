package plantest

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
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
//
// Only the first `---` line terminates the frontmatter: a horizontal rule in
// the body looks exactly the same and must not truncate the document.
func DraftBody(fsys fs.FS, name string) (string, error) {
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", err
	}
	contents := string(raw)
	if !strings.HasPrefix(contents, "---\n") {
		return "", fmt.Errorf("%s: draft has no frontmatter", name)
	}
	end := strings.Index(contents[3:], "\n---\n")
	if end < 0 {
		return "", fmt.Errorf("%s: draft frontmatter is unterminated", name)
	}
	return contents[3+end+len("\n---\n"):], nil
}

// DraftDocument writes the frontmatter for one plan in front of a draft body,
// producing the document `planr apply` reads.
func DraftDocument(planName string, dependsOn []string, body string) string {
	front := "plan_name: " + planName + "\n"
	if len(dependsOn) > 0 {
		// An empty depends_on list would be pruned back out of the generated
		// plan, so the key is written only when it carries something.
		front += "depends_on: [" + strings.Join(dependsOn, ", ") + "]\n"
	}
	return "---\n" + front + "---\n" + body
}
