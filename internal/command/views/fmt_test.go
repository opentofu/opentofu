// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestFmtViews(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(get Fmt)
		wantStdout string
		wantStderr string
	}{
		// Diagnostics
		"warning": {
			viewCall: func(v Fmt) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning occurred", "foo bar"),
				}
				v.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A warning occurred\n\nfoo bar"),
			wantStderr: "",
		},
		"error": {
			viewCall: func(v Fmt) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "An error occurred", "foo bar"),
				}
				v.Diagnostics(diags)
			},
			wantStdout: "",
			wantStderr: withNewline("\nError: An error occurred\n\nfoo bar"),
		},
		"multiple_diagnostics": {
			viewCall: func(v Fmt) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning", "foo bar warning"),
					tfdiags.Sourceless(tfdiags.Error, "An error", "foo bar error"),
				}
				v.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A warning\n\nfoo bar warning"),
			wantStderr: withNewline("\nError: An error\n\nfoo bar error"),
		},
		"content writer": {
			viewCall: func(v Fmt) {
				in := `resource foo_instance foo {
  instance_type = "${var.instance_type}"
}
`
				_, _ = v.UserOutputWriter().Write([]byte(in))
			},
			// The new line at the end is from the printer. The one in the input has been trimmed out
			wantStdout: "resource foo_instance foo {\n  instance_type = \"${var.instance_type}\"\n}\n",
			wantStderr: "",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testFmtHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
		})
	}
}

func testFmtHuman(t *testing.T, call func(get Fmt), wantStdout, wantStderr string) {
	view, done := testView(t)
	v := NewFmt(view)
	call(v)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}
