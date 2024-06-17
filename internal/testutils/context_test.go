package testutils_test

import (
	"testing"

	"github.com/opentofu/opentofu/internal/testutils"
)

func TestContext(t *testing.T) {
	ctx := testutils.Context(t)
	if ctx == nil {
		t.Fatalf("No context returned from testutils.Context")
	}
	tDeadline, tOk := t.Deadline()
	ctxDeadline, ctxOk := ctx.Deadline()
	if tOk != ctxOk {
		t.Fatalf("The testutils.Context function does not correctly set up the deadline ('ok' value mismatch)")
	}
	if tDeadline != ctxDeadline {
		t.Fatalf(
			"The testutils.Context function does not correctly set up the deadline ('deadline' value mismatch)",
		)
	}
}
