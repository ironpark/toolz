package truenas

import "testing"

func TestMetadataForMethod(t *testing.T) {
	if got := MetadataForMethod("pool.dataset.delete").Risk; got != MethodRiskDestructive {
		t.Fatalf("delete risk = %q", got)
	}
	if got := MetadataForMethod("pool.dataset.query").Risk; got != MethodRiskNormal {
		t.Fatalf("query risk = %q", got)
	}
}
