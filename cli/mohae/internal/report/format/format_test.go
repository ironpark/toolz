package format

import "testing"

func TestAllReturnsAStableCopy(t *testing.T) {
	formats := All()
	if len(formats) != 4 {
		t.Fatalf("All() = %v, want four formats", formats)
	}
	formats[0] = "changed"
	if got := All()[0]; got != HTML {
		t.Fatalf("mutating the returned formats changed the registry to %q", got)
	}
}

func TestIsKnown(t *testing.T) {
	for _, name := range All() {
		if !IsKnown(name) {
			t.Errorf("IsKnown(%q) = false", name)
		}
	}
	if IsKnown("carrier-pigeon") {
		t.Error("an unknown format was accepted")
	}
}
