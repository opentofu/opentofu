// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func assertDiagnosticsMatch(t *testing.T, got, want tfdiags.Diagnostics) {
	// We'll use the "for RPC" representation as a common baseline here
	// so that we're comparing the diagnostics just semantically rather
	// than by their implementation details. Note however that this
	// normalization doesn't cover EvalContext and Expression because
	// those are not RPC-friendly.
	for _, diag := range want {
		fromExpr := diag.FromExpr()
		if fromExpr != nil && fromExpr.Expression != nil {
			t.Fatal("assertDiagnosticsMatch cannot compare diagnostics with Expression")
		}
		if fromExpr != nil && fromExpr.EvalContext != nil {
			t.Fatal("assertDiagnosticsMatch cannot compare diagnostics with EvalContext")
		}
	}
	gotNorm := got.ForRPC()
	wantNorm := want.ForRPC()
	if diff := cmp.Diff(wantNorm, gotNorm); diff != "" {
		t.Fatal("wrong diagnostics:\n" + diff)
	}
}
