package lang

import "testing"

// TestAllowListFunctions ensures that only the allowed functions are present.
// It also ensures that assumptions made about how functions are registered (i.e. via makeBaseFunctionTable)
// are still valid. If the upstream function table changes, this test will fail.
func TestAllowListFunctions(t *testing.T) {
	allowedFunctions := []string{"jsondecode", "jsonencode", "keys", "length", "list", "trim", "trimprefix", "trimspace", "trimsuffix", "try"}
	allFunctions := makeBaseFunctionTable("")
	if len(allFunctions) != len(allowedFunctions) {
		t.Fatalf("expected 10 functions, got %d", len(allFunctions))
	}

	for _, allowed := range allowedFunctions {
		if _, ok := allFunctions[allowed]; !ok {
			t.Fatalf("expected function %q to be allowed, but it is not", allowed)
		}
	}
}
