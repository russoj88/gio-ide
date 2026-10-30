package packagesinternal

import "testing"

// TestP3ModeBits fails if the pinned go/packages LoadMode bits move.
func TestP3ModeBits(t *testing.T) {
	if TypecheckCgo != 4096 {
		t.Fatalf("TypecheckCgo = %d, want 4096 (typecheckCgo bit)", TypecheckCgo)
	}
	if DepsErrors != 1024 {
		t.Fatalf("DepsErrors = %d, want 1024 (needInternalDepsErrors bit)", DepsErrors)
	}
}
