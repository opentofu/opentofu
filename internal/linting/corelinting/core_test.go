// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package corelinting

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func compareDiagnostics(t *testing.T, want, got tfdiags.Diagnostics) {
	if len(want) != len(got) {
		t.Fatalf("cannot compare the diagnostic slices. want len = %d; got len = %d", len(want), len(got))
	}

	for i := range want {
		compareDiagnostic(t, i, want[i], got[i])
	}
}

func compareDiagnostic(t *testing.T, i int, want, got tfdiags.Diagnostic) {
	var prefix string
	if i >= 0 {
		prefix = fmt.Sprintf("[idx %d] ", i)
	}
	if wv, gv := want.Severity(), got.Severity(); wv != gv {
		t.Errorf("%sinvalid severity. want: %q but got %q", prefix, wv.String(), gv.String())
	}
	if wv, gv := want.Description().Summary, got.Description().Summary; wv != gv {
		t.Errorf("%sinvalid summary. want: %q but got %q", prefix, wv, gv)
	}
	if wv, gv := want.Description().Detail, got.Description().Detail; wv != gv {
		t.Errorf("%sinvalid detail. want: %q but got %q", prefix, wv, gv)
	}
	if wv, gv := want.Description().Address, got.Description().Address; wv != gv {
		t.Errorf("%sinvalid address. want: %q but got %q", prefix, wv, gv)
	}
	if wv, gv := want.Source(), got.Source(); !wv.Equal(gv) {
		t.Errorf("%sinvalid source. want: %#v but got %#v", prefix, wv, gv)
	}
	wei, gei := want.ExtraInfo(), got.ExtraInfo()
	if diff := cmp.Diff(wei, gei); diff != "" {
		t.Errorf("%sinvalid extra info (-want,+got):\n%s", prefix, diff)
	}
	wexpr, gexpr := want.FromExpr(), got.FromExpr()
	if diff := cmp.Diff(wexpr, gexpr); diff != "" {
		t.Errorf("%sinvalid fromExpr (-want,+got):\n%s", prefix, diff)
	}
}
