// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestCompileDependsOn(t *testing.T) {
	tests := map[string]struct {
		Input                    []hcl.Traversal
		MainScope, InstanceScope exprs.Scope
		ExtraMarks               cty.ValueMarks

		// Our mechanism for representing dependencies is cty marks, so at this
		// layer we're mainly just working generically with arbitrary marks
		// rather than with dependency marks in particular, though some current
		// special cases for referring to module calls violate that and end up
		// depending on the configgraph package's types directly.
		WantSharedMarks   cty.ValueMarks
		WantSharedDiags   tfdiags.Diagnostics
		WantInstanceMarks cty.ValueMarks
		WantInstanceDiags tfdiags.Diagnostics
	}{
		"empty": {
			Input:           nil,
			MainScope:       exprs.FlatScopeForTesting(nil),
			WantSharedMarks: nil,
			WantSharedDiags: nil,
		},
		"reference to unmarked value": {
			Input: []hcl.Traversal{
				{
					hcl.TraverseRoot{Name: "local"},
					hcl.TraverseAttr{Name: "foo"},
				},
			},
			MainScope: exprs.FlatScopeForTesting(map[string]cty.Value{
				"local": cty.ObjectVal(map[string]cty.Value{
					"foo": cty.StringVal("no marks here"),
				}),
			}),
			WantSharedMarks: nil,
			WantSharedDiags: nil,
		},
		"reference to marked value": {
			Input: []hcl.Traversal{
				{
					hcl.TraverseRoot{Name: "local"},
					hcl.TraverseAttr{Name: "foo"},
				},
			},
			MainScope: exprs.FlatScopeForTesting(map[string]cty.Value{
				"local": cty.ObjectVal(map[string]cty.Value{
					"foo": cty.StringVal("marked").Mark("precious"),
				}),
			}),
			WantSharedMarks: cty.NewValueMarks("precious"),
			WantSharedDiags: nil,
		},
		"references to two marked values and one unmarked": {
			Input: []hcl.Traversal{
				{
					hcl.TraverseRoot{Name: "local"},
					hcl.TraverseAttr{Name: "marked1"},
				},
				{
					hcl.TraverseRoot{Name: "local"},
					hcl.TraverseAttr{Name: "unmarked"},
				},
				{
					hcl.TraverseRoot{Name: "local"},
					hcl.TraverseAttr{Name: "marked2"},
				},
			},
			MainScope: exprs.FlatScopeForTesting(map[string]cty.Value{
				"local": cty.ObjectVal(map[string]cty.Value{
					"marked1":  cty.StringVal("...").Mark("precious"),
					"marked2":  cty.StringVal("...").Mark("magic"),
					"unmarked": cty.StringVal("..."),
				}),
			}),
			WantSharedMarks: cty.NewValueMarks("precious", "magic"),
			WantSharedDiags: nil,
		},
		"invalid reference": {
			Input: []hcl.Traversal{
				{
					hcl.TraverseRoot{
						Name: "local",
						SrcRange: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.InitialPos,
							End:      hcl.InitialPos,
						},
					},
				},
			},
			MainScope: exprs.FlatScopeForTesting(nil),
			// This is just one example of having the addrs.ParseRef diagnostics
			// get passed through the depends_on evaluation, to make sure that's
			// working at all. We don't exhaustively test every possible invalid
			// reference because that's a job for the addrs.ParseRef tests.
			WantSharedMarks: cty.NewValueMarks(exprs.EvalError),
			WantSharedDiags: tfdiags.New(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid reference",
				Detail:   `The "local" object cannot be accessed directly. Instead, access one of its attributes.`,
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.InitialPos,
					End:      hcl.InitialPos,
				},
			}),
		},

		// For now our only tests for instance-specific symbols are to test
		// that we refuse them completely, but this test structure is set up
		// to allow for a planned future extension that allows them.
		"refers to count.index": {
			Input: []hcl.Traversal{
				{
					hcl.TraverseRoot{
						Name: "count",
						SrcRange: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
							End:      hcl.Pos{Line: 1, Column: 6, Byte: 5},
						},
					},
					hcl.TraverseAttr{
						Name: "index",
						SrcRange: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 7, Byte: 6},
							End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
						},
					},
				},
			},
			MainScope: exprs.FlatScopeForTesting(nil),
			InstanceScope: exprs.FlatScopeForTesting(map[string]cty.Value{
				"count": cty.ObjectVal(map[string]cty.Value{
					"index": cty.NumberIntVal(0),
				}),
			}),

			WantSharedMarks: cty.NewValueMarks(exprs.EvalError),
			WantSharedDiags: tfdiags.New(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid explicit dependency",
				Detail:   `The depends_on argument must not include references to the per-instance symbols each.key, each.value, or count.index.`,
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
			}),
		},
		"refers to each.key": {
			Input: []hcl.Traversal{
				{
					hcl.TraverseRoot{
						Name: "each",
						SrcRange: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
							End:      hcl.Pos{Line: 1, Column: 5, Byte: 4},
						},
					},
					hcl.TraverseAttr{
						Name: "key",
						SrcRange: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
							End:      hcl.Pos{Line: 1, Column: 9, Byte: 8},
						},
					},
				},
			},
			MainScope: exprs.FlatScopeForTesting(nil),
			InstanceScope: exprs.FlatScopeForTesting(map[string]cty.Value{
				"each": cty.ObjectVal(map[string]cty.Value{
					"key": cty.StringVal("a"),
				}),
			}),

			WantSharedMarks: cty.NewValueMarks(exprs.EvalError),
			WantSharedDiags: tfdiags.New(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid explicit dependency",
				Detail:   `The depends_on argument must not include references to the per-instance symbols each.key, each.value, or count.index.`,
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 1, Column: 9, Byte: 8},
				},
			}),
		},
		"refers to each.value": {
			Input: []hcl.Traversal{
				{
					hcl.TraverseRoot{
						Name: "each",
						SrcRange: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
							End:      hcl.Pos{Line: 1, Column: 5, Byte: 4},
						},
					},
					hcl.TraverseAttr{
						Name: "value",
						SrcRange: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
							End:      hcl.Pos{Line: 1, Column: 11, Byte: 10},
						},
					},
				},
			},
			MainScope: exprs.FlatScopeForTesting(nil),
			InstanceScope: exprs.FlatScopeForTesting(map[string]cty.Value{
				"each": cty.ObjectVal(map[string]cty.Value{
					"key": cty.StringVal("a"),
				}),
			}),

			WantSharedMarks: cty.NewValueMarks(exprs.EvalError),
			WantSharedDiags: tfdiags.New(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid explicit dependency",
				Detail:   `The depends_on argument must not include references to the per-instance symbols each.key, each.value, or count.index.`,
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 1, Column: 11, Byte: 10},
				},
			}),
		},

		// TODO: Unfortunately references to module calls are currently treated in
		// a very special way so we can better match how the old runtime
		// would've treated them, and that special behavior depends directly
		// on concrete implementation details of this package in ways that
		// the other behavior doesn't and so it isn't practical to write unit
		// tests for those cases. For now we rely on the context tests in
		// package tofu for testing the behavior of module calls in depends_on.
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := grapheval.ContextWithNewWorker(t.Context())

			sharedDeps, compileInstanceDeps := compileDependsOn(test.Input, test.MainScope, test.ExtraMarks)
			gotSharedMarks, gotSharedDiags := sharedDeps.Marks(ctx)
			if diff := cmp.Diff(test.WantSharedMarks, gotSharedMarks); diff != "" {
				t.Error("wrong shared marks\n" + diff)
			}
			if diff := cmp.Diff(test.WantSharedDiags.ForRPC(), gotSharedDiags.ForRPC()); diff != "" {
				t.Error("wrong diagnostics for shared marks\n" + diff)
			}

			if test.InstanceScope != nil {
				instanceDeps := compileInstanceDeps(test.InstanceScope)
				gotInstanceMarks, gotInstanceDiags := instanceDeps.Marks(ctx)
				if diff := cmp.Diff(test.WantInstanceMarks, gotInstanceMarks); diff != "" {
					t.Error("wrong instance-specific marks\n" + diff)
				}
				if diff := cmp.Diff(test.WantInstanceDiags.ForRPC(), gotInstanceDiags.ForRPC()); diff != "" {
					t.Error("wrong diagnostics for instance-specific marks\n" + diff)
				}
			} else {
				if test.WantInstanceMarks != nil || test.WantInstanceDiags != nil {
					t.Errorf("invalid test: must set InstanceScope in order to test instance-specific marks and diagnostics")
				}
			}
		})
	}
}

// dependsOnForTesting constructs a [dependsOn] that just returns exactly
// the given marks, without any further processing.
func dependsOnForTesting(marks ...any) dependsOn {
	var markSet cty.ValueMarks
	if len(marks) != 0 {
		markSet = make(cty.ValueMarks, len(marks))
		for _, mark := range marks {
			markSet[mark] = struct{}{}
		}
	}
	return dependsOn{
		valuer: exprs.ConstantValuer(cty.NullVal(cty.DynamicPseudoType).WithMarks(markSet)),
	}
}
