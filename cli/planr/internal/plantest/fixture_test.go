package plantest

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestDraftBodyStripsOnlyTheFrontmatter(t *testing.T) {
	// A horizontal rule in Markdown looks exactly like a frontmatter
	// terminator, so only the first one may end the frontmatter.
	fixtures := fstest.MapFS{
		"plain.md": {Data: []byte("---\nplan_name: checkout-v2\n---\n# GOALS\n\nShip it.\n")},
		"rule.md":  {Data: []byte("---\nplan_name: x\n---\n# GOALS\n\n---\n\nmore\n")},
	}
	for _, test := range []struct{ name, want string }{
		{name: "plain.md", want: "# GOALS\n\nShip it.\n"},
		{name: "rule.md", want: "# GOALS\n\n---\n\nmore\n"},
	} {
		got, err := DraftBody(fixtures, test.name)
		if err != nil {
			t.Fatalf("DraftBody(%s) unexpected error: %v", test.name, err)
		}
		if got != test.want {
			t.Errorf("DraftBody(%s) = %q, want %q", test.name, got, test.want)
		}
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
	document := DraftDocument("checkout-v2", []string{"auth-foundation#2"}, "# GOALS\n")
	if !strings.HasPrefix(document, "---\nplan_name: checkout-v2\ndepends_on: [auth-foundation#2]\n---\n") {
		t.Fatalf("document frontmatter = %q", document)
	}
	if !strings.HasSuffix(document, "---\n# GOALS\n") {
		t.Fatalf("document body was rewritten: %q", document)
	}
	// An empty depends_on list would be written back as `depends_on: []`, which
	// planr prunes; leaving the key out keeps input and output alike.
	if strings.Contains(DraftDocument("auth-foundation", nil, "# GOALS\n"), "depends_on") {
		t.Fatal("a plan without dependencies still writes depends_on")
	}
}
