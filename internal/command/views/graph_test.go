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

func TestGraphView(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(graph Graph)
		wantStdout string
		wantStderr string
	}{
		"output graph": {
			viewCall: func(graph Graph) {
				graph.Output(`digraph {
	compound = "true"
	newrank = "true"
	subgraph "root" {
		"[root] provider.aws" [label = "provider.aws", shape = "diamond"]
	}
}`)
			},
			wantStdout: `digraph {
	compound = "true"
	newrank = "true"
	subgraph "root" {
		"[root] provider.aws" [label = "provider.aws", shape = "diamond"]
	}
}
`,
			wantStderr: "",
		},
		"error unsupported local op": {
			viewCall: func(graph Graph) {
				graph.ErrorUnsupportedLocalOp()
			},
			wantStdout: "",
			wantStderr: withNewline(`The configured backend doesn't support this operation.

The "backend" in OpenTofu defines how OpenTofu operates. The default
backend performs all operations locally on your machine. Your configuration
is configured to use a non-local backend. This backend doesn't support this
operation.
`),
		},
		"diagnostics warning": {
			viewCall: func(graph Graph) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning occurred", "This is a warning message"),
				}
				graph.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A warning occurred\n\nThis is a warning message"),
			wantStderr: "",
		},
		"diagnostics error": {
			viewCall: func(graph Graph) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "An error occurred", "This is an error message"),
				}
				graph.Diagnostics(diags)
			},
			wantStdout: "",
			wantStderr: withNewline("\nError: An error occurred\n\nThis is an error message"),
		},
		"diagnostics multiple": {
			viewCall: func(graph Graph) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "First warning", "This is the first warning"),
					tfdiags.Sourceless(tfdiags.Error, "An error", "This is an error"),
					tfdiags.Sourceless(tfdiags.Warning, "Second warning", "This is the second warning"),
				}
				graph.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: First warning\n\nThis is the first warning\n\nWarning: Second warning\n\nThis is the second warning"),
			wantStderr: withNewline("\nError: An error\n\nThis is an error"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testGraphHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
		})
	}
}

func testGraphHuman(t *testing.T, call func(graph Graph), wantStdout, wantStderr string) {
	view, done := testView(t)
	graphView := NewGraph(view)
	call(graphView)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}
