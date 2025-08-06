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

func TestUnmarkDeepWithPathsDeprecated(t *testing.T) {
	tests := []struct {
		name                      string
		input                     cty.Value
		wantDeprecationPathsCount int
	}{
		{
			name:                      "no marks",
			input:                     cty.StringVal("test"),
			wantDeprecationPathsCount: 0,
		},
		{
			name: "single deprecation mark",
			input: Deprecated(cty.StringVal("test"), DeprecationCause{
				By:      addrs.InputVariable{Name: "var1"},
				Key:     "var1",
				Message: "deprecated",
			}),
			wantDeprecationPathsCount: 1,
		},
		{
			name: "mixed marks",
			input: Deprecated(cty.StringVal("test").Mark(Sensitive), DeprecationCause{
				By:      addrs.InputVariable{Name: "var1"},
				Key:     "var1",
				Message: "deprecated",
			}),
			wantDeprecationPathsCount: 1,
		},
		{
			name: "multiple fields all of which have only deprecation marks",
			input: cty.ObjectVal(map[string]cty.Value{
				"field1": Deprecated(cty.StringVal("test1"), DeprecationCause{
					By:      addrs.InputVariable{Name: "var1"},
					Key:     "var1",
					Message: "deprecated1",
				}),
				"field2": Deprecated(cty.StringVal("test2"), DeprecationCause{
					By:      addrs.InputVariable{Name: "var2"},
					Key:     "var2",
					Message: "deprecated2",
				}),
				"field3": Deprecated(cty.StringVal("test3"), DeprecationCause{
					By:      addrs.InputVariable{Name: "var3"},
					Key:     "var3",
					Message: "deprecated3",
				}),
			}),
			wantDeprecationPathsCount: 3,
		},
		{
			name: "nested object with deprecation",
			input: cty.ObjectVal(map[string]cty.Value{
				"outer": cty.ObjectVal(map[string]cty.Value{
					"inner": Deprecated(cty.StringVal("nested"), DeprecationCause{
						By:      addrs.InputVariable{Name: "var1"},
						Key:     "var1",
						Message: "deprecated",
					}),
				}),
			}),
			wantDeprecationPathsCount: 1,
		},
		{
			name: "only non-deprecation marks",
			input: cty.ObjectVal(map[string]cty.Value{
				"sensitive": cty.StringVal("secret").Mark(Sensitive),
				"ephemeral": cty.StringVal("temp").Mark(Ephemeral),
			}),
			wantDeprecationPathsCount: 0,
		},
		{
			name: "nested with other marks too",
			input: cty.ObjectVal(map[string]cty.Value{
				"outer": cty.ObjectVal(map[string]cty.Value{
					"deprecated": Deprecated(cty.StringVal("dep"), DeprecationCause{
						By:      addrs.InputVariable{Name: "var1"},
						Key:     "var1",
						Message: "deprecated",
					}),
					"sensitive": cty.StringVal("secret").Mark(Sensitive),
				}),
			}),
			wantDeprecationPathsCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUnmarked, gotDeprecationMarks := unmarkDeepWithPathsDeprecated(tt.input)

			if len(gotDeprecationMarks) != tt.wantDeprecationPathsCount {
				t.Errorf("deprecation marks count mismatch\ngot:  %d\nwant: %d", len(gotDeprecationMarks), tt.wantDeprecationPathsCount)
			}

			// Verify that the returned value has NO deprecation marks
			if HasDeprecated(gotUnmarked) {
				t.Error("returned value still contains deprecation marks")
			}

			// Verify all deprecation marks returned only contain deprecation marks
			for _, pm := range gotDeprecationMarks {
				for m := range pm.Marks {
					if _, ok := m.(deprecationMark); !ok {
						t.Errorf("found non-deprecation mark in deprecation marks: %T", m)
					}
				}
			}
		})
	}
}
