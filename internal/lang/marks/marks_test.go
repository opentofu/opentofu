// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package marks

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Integration test for the marks to see if consolidate warnings is
// checking by the key of the mark together with the address
func TestMarkConsolidateWarnings(t *testing.T) {
	var diags tfdiags.Diagnostics

	for i := 0; i < 2; i++ {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			// Summary is the same for both diagnostics, but key is different
			Summary: "Output deprecated",
			Detail:  fmt.Sprintf("This one has an output %d", i),
			Subject: &hcl.Range{
				Filename: "foo.tf",
				Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
				End:      hcl.Pos{Line: 1, Column: 1, Byte: 0},
			},
			Extra: DeprecationCause{
				By: addrs.OutputValue{
					Name: "output",
				},
				Key:     fmt.Sprintf("output%d", i),
				Message: "output deprecate",
			},
		})
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Summary:  "Variable deprecated",
			Detail:   fmt.Sprintf("This one has a var %d", i),
			Subject: &hcl.Range{
				Filename: "foo.tf",
				Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
				End:      hcl.Pos{Line: 1, Column: 1, Byte: 0},
			},
			Extra: DeprecationCause{
				By: addrs.InputVariable{
					Name: "variable",
				},
				Key:     fmt.Sprintf("variable%d", i),
				Message: "variable deprecate",
			},
		})
	}

	// Adding an extra diagnostic with the same key to be consolidated
	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "Variable deprecated",
		Detail:   "This one has a var 1",
		Subject: &hcl.Range{
			Filename: "foo.tf",
			Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
			End:      hcl.Pos{Line: 1, Column: 1, Byte: 0},
		},
		Extra: DeprecationCause{
			By: addrs.InputVariable{
				Name: "variable",
			},
			Key:     "variable1",
			Message: "variable deprecate",
		},
	})

	// Adding an extra diagnostic with a variable in a module
	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "Variable deprecated",
		Detail:   "This one has a var 1 in a module",
		Subject: &hcl.Range{
			Filename: "foo.tf",
			Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
			End:      hcl.Pos{Line: 1, Column: 1, Byte: 0},
		},
		Extra: DeprecationCause{
			By: addrs.InputVariable{
				Name: "mod1.variable",
			},
			Key:     "mod1.variable1",
			Message: "variable deprecate",
		},
	})

	consolidatedDiags := diags.Consolidate(1, tfdiags.Warning)
	expectedDescriptions := [][2]string{
		{"Output deprecated", "This one has an output 0"},
		{"Variable deprecated", "This one has a var 0"},
		{"Output deprecated", "This one has an output 1"},
		{"Variable deprecated", "This one has a var 1\n\n(and one more similar warning elsewhere)"},
		{"Variable deprecated", "This one has a var 1 in a module"},
	}

	// We created 5 diagnostics, but the last one is consolidated
	expectedLen := len(expectedDescriptions)
	if len(consolidatedDiags) != expectedLen {
		t.Errorf("len %d is expected, got %d", expectedLen, len(consolidatedDiags))
	}

	for i, vals := range expectedDescriptions {
		if diff := cmp.Diff(vals[0], consolidatedDiags[i].Description().Summary); diff != "" {
			t.Errorf("%d: wrong summary: %s", i, diff)
		}
		if diff := cmp.Diff(vals[1], consolidatedDiags[i].Description().Detail); diff != "" {
			t.Errorf("%d: wrong detail msg: %s", i, diff)
		}
	}
}
