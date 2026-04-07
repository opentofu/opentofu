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

func TestRootView(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(view *Root)
		wantStdout string
		wantStderr string
	}{
		"custom error": {
			viewCall: func(v *Root) {
				v.Error("custom error")
			},
			wantStderr: `custom error
`,
		},
		"warning": {
			viewCall: func(v *Root) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning occurred", "foo bar"),
				}
				v.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A warning occurred\n\nfoo bar"),
			wantStderr: "",
		},
		"error": {
			viewCall: func(v *Root) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "An error occurred", "foo bar"),
				}
				v.Diagnostics(diags)
			},
			wantStdout: "",
			wantStderr: withNewline("\nError: An error occurred\n\nfoo bar"),
		},
		"multiple diagnostics": {
			viewCall: func(v *Root) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning", "foo bar warning"),
					tfdiags.Sourceless(tfdiags.Error, "An error", "foo bar error"),
				}
				v.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A warning\n\nfoo bar warning"),
			wantStderr: withNewline("\nError: An error\n\nfoo bar error"),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			view, done := testView(t)
			v := NewRoot(view)
			tc.viewCall(v)
			output := done(t)
			if diff := cmp.Diff(tc.wantStderr, output.Stderr()); diff != "" {
				t.Errorf("invalid stderr (-want, +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantStdout, output.Stdout()); diff != "" {
				t.Errorf("invalid stdout (-want, +got):\n%s", diff)
			}
		})
	}
}
