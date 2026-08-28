package plantest

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
)

func TestDraftBodyStripsTheFrontmatter(t *testing.T) {
	fixtures := fstest.MapFS{
		"plain.md": {Data: []byte("---\nplan_name: checkout-v2\n---\n# GOALS\n\nShip it.\n")},
	}
	got, err := DraftBody(fixtures, "plain.md")
	if err != nil {
		t.Fatalf("DraftBody() unexpected error: %v", err)
	}
	if got != "# GOALS\n\nShip it.\n" {
		t.Errorf("DraftBody() = %q", got)
	}
}

func TestDraftBodyRejectsUnusableFrontmatter(t *testing.T) {
	// Callers build every plan from this body, so a fixture that lost its
	// frontmatter has to fail rather than register plans named after the file.
	fixtures := fstest.MapFS{
		"missing.md":      {Data: []byte("# GOALS\n")},
		"unterminated.md": {Data: []byte("---\nplan_name: checkout-v2\n")},
	}
	for _, name := range []string{"missing.md", "unterminated.md", "absent.md"} {
		if _, err := DraftBody(fixtures, name); err == nil {
			t.Errorf("DraftBody(%s) succeeded, want an error", name)
		}
	}
}

func TestCheckoutFixtureIsAPlanDraft(t *testing.T) {
	body, err := DraftBody(Fixtures(), CheckoutFixture)
	if err != nil {
		t.Fatalf("DraftBody(%s) unexpected error: %v", CheckoutFixture, err)
	}
	if !strings.HasPrefix(body, "# GOALS") || !strings.Contains(body, "# PHASES") {
		t.Fatalf("%s does not read as a plan draft:\n%s", CheckoutFixture, body)
	}
}

func TestDraftDocumentWritesDependenciesOnlyWhenPresent(t *testing.T) {
	document, err := DraftDocument("checkout-v2", []string{"auth-foundation#2"}, "# GOALS\n")
	if err != nil {
		t.Fatalf("DraftDocument() unexpected error: %v", err)
	}
	front, body, err := mdoc.Split(document)
	if err != nil {
		t.Fatalf("mdoc.Split() unexpected error: %v", err)
	}
	if front["plan_name"] != "checkout-v2" || len(mdoc.Strings(front["depends_on"])) != 1 {
		t.Fatalf("document frontmatter = %#v", front)
	}
	if body != "# GOALS\n" {
		t.Fatalf("document body = %q", body)
	}
	// An empty depends_on list would be written back as `depends_on: []`, which
	// planr prunes; leaving the key out keeps input and output alike.
	document, err = DraftDocument("auth-foundation", nil, "# GOALS\n")
	if err != nil {
		t.Fatalf("DraftDocument() unexpected error: %v", err)
	}
	if strings.Contains(document, "depends_on") {
		t.Fatal("a plan without dependencies still writes depends_on")
	}
}
