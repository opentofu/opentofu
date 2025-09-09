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
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"

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

func TestHasDeprecated(t *testing.T) {
	tests := []struct {
		name  string
		input cty.Value
		want  bool
	}{
		{
			name:  "no marks",
			input: cty.StringVal("test"),
			want:  false,
		},
		{
			name:  "only sensitive mark",
			input: cty.StringVal("test").Mark(Sensitive),
			want:  false,
		},
		{
			name: "has deprecation mark",
			input: Deprecated(cty.StringVal("test"), DeprecationCause{
				By:      addrs.InputVariable{Name: "var1"},
				Key:     "var1",
				Message: "deprecated",
			}),
			want: true,
		},
		{
			name: "mixed marks with deprecation",
			input: Deprecated(cty.StringVal("test").Mark(Sensitive), DeprecationCause{
				By:      addrs.InputVariable{Name: "var1"},
				Key:     "var1",
				Message: "deprecated",
			}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasDeprecated(tt.input)
			if got != tt.want {
				t.Errorf("HasDeprecated() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractDeprecatedDiagnosticsWithExpr(t *testing.T) {
	// This is a small unit test focused just on how we detect a deprecation
	// mark and translate it into a diagnostic message. The main tests for
	// the overall dynamic deprecation handling are in
	// [tofu.TestContext2Apply_deprecationWarnings].

	input := cty.ObjectVal(map[string]cty.Value{
		"okay": cty.StringVal("not deprecated").Mark(Sensitive),
		"warn": DeprecatedOutput(
			cty.StringVal("deprecated"),
			addrs.OutputValue{Name: "foo"}.Absolute(addrs.RootModuleInstance.Child("child", addrs.StringKey("beep"))),
			"Blah blah blah don't use this!",
			false,
		),
	})
	got, gotDiags := ExtractDeprecatedDiagnosticsWithExpr(
		input,
		// This expression is used just for its source location information.
		hcl.StaticExpr(cty.DynamicVal, hcl.Range{Filename: "test.tofu"}),
	)
	want := cty.ObjectVal(map[string]cty.Value{
		"okay": cty.StringVal("not deprecated").Mark(Sensitive), // non-deprecation marks should be preserved
		"warn": cty.StringVal("deprecated"),                     // deprecation marks are removed
	})
	if diff := cmp.Diff(want, got, ctydebug.CmpOptions); diff != "" {
		t.Error("wrong result value\n" + diff)
	}

	// We'll use the "ForRPC" representation of diagnostics just because it
	// compares well with cmp. We don't actually care what type of diagnostic
	// is returned here, only that it has the expected content.
	gotDiags = gotDiags.ForRPC()
	var wantDiags tfdiags.Diagnostics
	wantDiags = wantDiags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "Value derived from a deprecated source",
		Detail:   "This value's attribute warn is derived from module.child[\"beep\"].foo, which is deprecated with the following message:\n\nBlah blah blah don't use this!",
		Subject:  &hcl.Range{Filename: "test.tofu"}, // source location should come from the given expression
	})
	wantDiags = wantDiags.ForRPC()
	if diff := cmp.Diff(wantDiags, gotDiags); diff != "" {
		t.Error("wrong diagnostics\n" + diff)
	}

}
