// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/collections"
	"github.com/opentofu/opentofu/internal/linting"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestCoreRule_RedundantDependsOn(t *testing.T) {
	cases := map[string]struct {
		setup func(ctx context.Context) (context.Context, addrs.ConfigResource, hcl.Range, func() []*addrs.Reference, func() []*addrs.Reference, bool, bool, tfdiags.Diagnostics)
	}{
		"resource in root module depends on other resource": {
			setup: func(ctx context.Context) (context.Context, addrs.ConfigResource, hcl.Range, func() []*addrs.Reference, func() []*addrs.Reference, bool, bool, tfdiags.Diagnostics) {
				newCtx := tfdiags.ContextWithLintFilterHints(ctx, collections.NewSet(ruleIDRedundantDependsOn), nil)
				targetRes := addrs.MustParseResourceAddr("test.resource_name")
				targetResRange := hcl.Range{Filename: "test.tf", Start: hcl.Pos{Line: 1, Byte: 4, Column: 4}, End: hcl.Pos{Line: 1, Byte: 10, Column: 10}}
				ref := addrs.Reference{
					Subject: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "targeted_res",
					},
					SourceRange: tfdiags.SourceRange{
						Filename: "test.tf",
						Start:    tfdiags.SourcePos{Line: 2, Column: 4, Byte: 4},
						End:      tfdiags.SourcePos{Line: 2, Column: 10, Byte: 10},
					},
					Remaining: hcl.Traversal{hcl.TraverseAttr{Name: "attribute"}},
				}
				directRefCb := func() []*addrs.Reference {
					return []*addrs.Reference{&ref}
				}
				dependsOnCb := func() []*addrs.Reference {
					return []*addrs.Reference{&ref}
				}
				wantDiags := tfdiags.New(tfdiags.LintMessage(
					ruleIDRedundantDependsOn,
					[]linting.RuleAddr{GroupIDConfusing},
					"Redundant 'depends_on' usage",
					fmt.Sprintf("Resource %q configures %q as 'depends_on'. The configured dependency is already automatically inferred which makes this particular 'depends_on' reference redundant.", targetRes.String(), unkeyedAddressReferenceKey(ref)),
					new(tfdiags.SourceRangeFromHCL(ref.SourceRange.ToHCL())),
					new(tfdiags.SourceRangeFromHCL(targetResRange)),
				))
				return newCtx, targetRes, targetResRange, directRefCb, dependsOnCb, true, true, wantDiags
			},
		},
		"resource in root module depends on resource instance": {
			setup: func(ctx context.Context) (context.Context, addrs.ConfigResource, hcl.Range, func() []*addrs.Reference, func() []*addrs.Reference, bool, bool, tfdiags.Diagnostics) {
				newCtx := tfdiags.ContextWithLintFilterHints(ctx, collections.NewSet(ruleIDRedundantDependsOn), nil)
				targetRes := addrs.MustParseResourceAddr("test.resource_name")
				targetResRange := hcl.Range{Filename: "test.tf", Start: hcl.Pos{Line: 1, Byte: 4, Column: 4}, End: hcl.Pos{Line: 1, Byte: 10, Column: 10}}
				ref := addrs.Reference{
					Subject: addrs.ResourceInstance{
						Resource: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test",
							Name: "targeted_res",
						},
						Key: addrs.IntKey(0),
					},
					SourceRange: tfdiags.SourceRange{
						Filename: "test.tf",
						Start:    tfdiags.SourcePos{Line: 2, Column: 4, Byte: 4},
						End:      tfdiags.SourcePos{Line: 2, Column: 10, Byte: 10},
					},
					Remaining: hcl.Traversal{hcl.TraverseAttr{Name: "attribute"}},
				}
				directRefCb := func() []*addrs.Reference {
					return []*addrs.Reference{&ref}
				}
				dependsOnCb := func() []*addrs.Reference {
					return []*addrs.Reference{&ref}
				}
				wantDiags := tfdiags.New(tfdiags.LintMessage(
					ruleIDRedundantDependsOn,
					[]linting.RuleAddr{GroupIDConfusing},
					"Redundant 'depends_on' usage",
					fmt.Sprintf("Resource %q configures %q as 'depends_on'. The configured dependency is already automatically inferred which makes this particular 'depends_on' reference redundant.", targetRes.String(), unkeyedAddressReferenceKey(ref)),
					new(tfdiags.SourceRangeFromHCL(ref.SourceRange.ToHCL())),
					new(tfdiags.SourceRangeFromHCL(targetResRange)),
				))
				return newCtx, targetRes, targetResRange, directRefCb, dependsOnCb, true, true, wantDiags
			},
		},
		"resource in root module depends on module": {
			setup: func(ctx context.Context) (context.Context, addrs.ConfigResource, hcl.Range, func() []*addrs.Reference, func() []*addrs.Reference, bool, bool, tfdiags.Diagnostics) {
				newCtx := tfdiags.ContextWithLintFilterHints(ctx, collections.NewSet(ruleIDRedundantDependsOn), nil)
				targetRes := addrs.MustParseResourceAddr("test.resource_name")
				targetResRange := hcl.Range{Filename: "test.tf", Start: hcl.Pos{Line: 1, Byte: 4, Column: 4}, End: hcl.Pos{Line: 1, Byte: 10, Column: 10}}
				ref := addrs.Reference{
					Subject: addrs.ModuleCall{
						Name: "targeted_res",
					},
					SourceRange: tfdiags.SourceRange{
						Filename: "test.tf",
						Start:    tfdiags.SourcePos{Line: 2, Column: 4, Byte: 4},
						End:      tfdiags.SourcePos{Line: 2, Column: 10, Byte: 10},
					},
					Remaining: hcl.Traversal{hcl.TraverseAttr{Name: "output"}},
				}
				directRefCb := func() []*addrs.Reference {
					return []*addrs.Reference{&ref}
				}
				dependsOnCb := func() []*addrs.Reference {
					return []*addrs.Reference{&ref}
				}
				wantDiags := tfdiags.New(tfdiags.LintMessage(
					ruleIDRedundantDependsOn,
					[]linting.RuleAddr{GroupIDConfusing},
					"Redundant 'depends_on' usage",
					fmt.Sprintf("Resource %q configures %q as 'depends_on'. The configured dependency is already automatically inferred which makes this particular 'depends_on' reference redundant.", targetRes.String(), unkeyedAddressReferenceKey(ref)),
					new(tfdiags.SourceRangeFromHCL(ref.SourceRange.ToHCL())),
					new(tfdiags.SourceRangeFromHCL(targetResRange)),
				))
				return newCtx, targetRes, targetResRange, directRefCb, dependsOnCb, true, true, wantDiags
			},
		},
		"resource in root module depends on module instance": {
			setup: func(ctx context.Context) (context.Context, addrs.ConfigResource, hcl.Range, func() []*addrs.Reference, func() []*addrs.Reference, bool, bool, tfdiags.Diagnostics) {
				newCtx := tfdiags.ContextWithLintFilterHints(ctx, collections.NewSet(ruleIDRedundantDependsOn), nil)
				targetRes := addrs.MustParseResourceAddr("test.resource_name")
				targetResRange := hcl.Range{Filename: "test.tf", Start: hcl.Pos{Line: 1, Byte: 4, Column: 4}, End: hcl.Pos{Line: 1, Byte: 10, Column: 10}}
				ref := addrs.Reference{
					Subject: addrs.ModuleCallInstance{
						Call: addrs.ModuleCall{
							Name: "targeted_res",
						},
						Key: addrs.IntKey(0),
					},
					SourceRange: tfdiags.SourceRange{
						Filename: "test.tf",
						Start:    tfdiags.SourcePos{Line: 2, Column: 4, Byte: 4},
						End:      tfdiags.SourcePos{Line: 2, Column: 10, Byte: 10},
					},
					Remaining: hcl.Traversal{hcl.TraverseAttr{Name: "output"}},
				}
				directRefCb := func() []*addrs.Reference {
					return []*addrs.Reference{&ref}
				}
				dependsOnCb := func() []*addrs.Reference {
					return []*addrs.Reference{&ref}
				}
				wantDiags := tfdiags.New(tfdiags.LintMessage(
					ruleIDRedundantDependsOn,
					[]linting.RuleAddr{GroupIDConfusing},
					"Redundant 'depends_on' usage",
					fmt.Sprintf("Resource %q configures %q as 'depends_on'. The configured dependency is already automatically inferred which makes this particular 'depends_on' reference redundant.", targetRes.String(), unkeyedAddressReferenceKey(ref)),
					new(tfdiags.SourceRangeFromHCL(ref.SourceRange.ToHCL())),
					new(tfdiags.SourceRangeFromHCL(targetResRange)),
				))
				return newCtx, targetRes, targetResRange, directRefCb, dependsOnCb, true, true, wantDiags
			},
		},
		"resource in root module depends on resource that has no references too": {
			setup: func(ctx context.Context) (context.Context, addrs.ConfigResource, hcl.Range, func() []*addrs.Reference, func() []*addrs.Reference, bool, bool, tfdiags.Diagnostics) {
				newCtx := tfdiags.ContextWithLintFilterHints(ctx, collections.NewSet(ruleIDRedundantDependsOn), nil)
				targetRes := addrs.MustParseResourceAddr("test.resource_name")
				targetResRange := hcl.Range{Filename: "test.tf", Start: hcl.Pos{Line: 1, Byte: 4, Column: 4}, End: hcl.Pos{Line: 1, Byte: 10, Column: 10}}
				directRefCb := func() []*addrs.Reference {
					// has direct references to other blocks
					return []*addrs.Reference{
						{
							Subject: addrs.Resource{
								Mode: addrs.DataResourceMode,
								Type: "test",
								Name: "dummy_data",
							},
							SourceRange: tfdiags.SourceRange{
								Filename: "test.tf",
								Start:    tfdiags.SourcePos{Line: 4, Column: 4, Byte: 4},
								End:      tfdiags.SourcePos{Line: 4, Column: 10, Byte: 10},
							},
							Remaining: hcl.Traversal{hcl.TraverseAttr{Name: "attribute"}},
						},
					}
				}
				dependsOnCb := func() []*addrs.Reference {
					return []*addrs.Reference{
						{
							Subject: addrs.Resource{
								Mode: addrs.ManagedResourceMode,
								Type: "test",
								Name: "targeted_res",
							},
							SourceRange: tfdiags.SourceRange{
								Filename: "test.tf",
								Start:    tfdiags.SourcePos{Line: 2, Column: 4, Byte: 4},
								End:      tfdiags.SourcePos{Line: 2, Column: 10, Byte: 10},
							},
							Remaining: hcl.Traversal{hcl.TraverseAttr{Name: "attribute"}},
						},
					}
				}
				var wantDiags tfdiags.Diagnostics // since the depends_on contains something else than the directly referenced blocks, there should be no lint diagnostics
				return newCtx, targetRes, targetResRange, directRefCb, dependsOnCb, true, true, wantDiags
			},
		},
		"resource in root module depends on multiple referenced blocks": {
			setup: func(ctx context.Context) (context.Context, addrs.ConfigResource, hcl.Range, func() []*addrs.Reference, func() []*addrs.Reference, bool, bool, tfdiags.Diagnostics) {
				newCtx := tfdiags.ContextWithLintFilterHints(ctx, collections.NewSet(ruleIDRedundantDependsOn), nil)
				targetRes := addrs.MustParseResourceAddr("test.resource_name")
				targetResRange := hcl.Range{Filename: "test.tf", Start: hcl.Pos{Line: 1, Byte: 4, Column: 4}, End: hcl.Pos{Line: 1, Byte: 10, Column: 10}}
				resRef := addrs.Reference{
					Subject: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "targeted_res",
					},
					SourceRange: tfdiags.SourceRange{
						Filename: "test.tf",
						Start:    tfdiags.SourcePos{Line: 2, Column: 4, Byte: 4},
						End:      tfdiags.SourcePos{Line: 2, Column: 10, Byte: 10},
					},
					Remaining: hcl.Traversal{hcl.TraverseAttr{Name: "attribute"}},
				}
				modRef := addrs.Reference{
					Subject: addrs.ModuleCallOutput{
						Call: addrs.ModuleCall{
							Name: "targeted_module",
						},
						Name: "output",
					},
					SourceRange: tfdiags.SourceRange{
						Filename: "test.tf",
						Start:    tfdiags.SourcePos{Line: 4, Column: 4, Byte: 4},
						End:      tfdiags.SourcePos{Line: 4, Column: 10, Byte: 10},
					},
					Remaining: hcl.Traversal{hcl.TraverseAttr{Name: "attribute"}},
				}
				modInstanceRef := addrs.Reference{
					Subject: addrs.ModuleCallInstanceOutput{
						Call: addrs.ModuleCallInstance{
							Call: addrs.ModuleCall{Name: "targeted_module"},
						},
						Name: "output",
					},
					SourceRange: tfdiags.SourceRange{
						Filename: "test.tf",
						Start:    tfdiags.SourcePos{Line: 4, Column: 4, Byte: 4},
						End:      tfdiags.SourcePos{Line: 4, Column: 10, Byte: 10},
					},
					Remaining: hcl.Traversal{hcl.TraverseAttr{Name: "attribute"}},
				}
				varRef := addrs.Reference{
					Subject: addrs.InputVariable{
						Name: "targeted_res",
					},
					SourceRange: tfdiags.SourceRange{
						Filename: "test.tf",
						Start:    tfdiags.SourcePos{Line: 6, Column: 4, Byte: 4},
						End:      tfdiags.SourcePos{Line: 6, Column: 10, Byte: 10},
					},
				}
				directRefCb := func() []*addrs.Reference {
					return []*addrs.Reference{&resRef, &modRef}
				}
				dependsOnCb := func() []*addrs.Reference {
					return []*addrs.Reference{&resRef, &modRef, &varRef, &modInstanceRef}
				}
				wantDiags := tfdiags.New(
					tfdiags.LintMessage(
						ruleIDRedundantDependsOn,
						[]linting.RuleAddr{GroupIDConfusing},
						"Redundant 'depends_on' usage",
						fmt.Sprintf("Resource %q configures %q as 'depends_on'. The configured dependency is already automatically inferred which makes this particular 'depends_on' reference redundant.", targetRes.String(), unkeyedAddressReferenceKey(resRef)),
						new(tfdiags.SourceRangeFromHCL(resRef.SourceRange.ToHCL())),
						new(tfdiags.SourceRangeFromHCL(targetResRange)),
					),
					tfdiags.LintMessage(
						ruleIDRedundantDependsOn,
						[]linting.RuleAddr{GroupIDConfusing},
						"Redundant 'depends_on' usage",
						fmt.Sprintf("Resource %q configures %q as 'depends_on'. The configured dependency is already automatically inferred which makes this particular 'depends_on' reference redundant.", targetRes.String(), unkeyedAddressReferenceKey(modRef)),
						new(tfdiags.SourceRangeFromHCL(modRef.SourceRange.ToHCL())),
						new(tfdiags.SourceRangeFromHCL(targetResRange)),
					),
				)
				return newCtx, targetRes, targetResRange, directRefCb, dependsOnCb, true, true, wantDiags
			},
		},
		"resource has no depends_on references so direct references callback is not even called": {
			setup: func(ctx context.Context) (context.Context, addrs.ConfigResource, hcl.Range, func() []*addrs.Reference, func() []*addrs.Reference, bool, bool, tfdiags.Diagnostics) {
				newCtx := tfdiags.ContextWithLintFilterHints(ctx, collections.NewSet(ruleIDRedundantDependsOn), nil)
				targetRes := addrs.MustParseResourceAddr("test.resource_name")
				targetResRange := hcl.Range{Filename: "test.tf", Start: hcl.Pos{Line: 1, Byte: 4, Column: 4}, End: hcl.Pos{Line: 1, Byte: 10, Column: 10}}
				resRef := addrs.Reference{
					Subject: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "targeted_res",
					},
					SourceRange: tfdiags.SourceRange{
						Filename: "test.tf",
						Start:    tfdiags.SourcePos{Line: 2, Column: 4, Byte: 4},
						End:      tfdiags.SourcePos{Line: 2, Column: 10, Byte: 10},
					},
					Remaining: hcl.Traversal{hcl.TraverseAttr{Name: "attribute"}},
				}
				directRefCb := func() []*addrs.Reference {
					return []*addrs.Reference{&resRef}
				}
				dependsOnCb := func() []*addrs.Reference {
					return []*addrs.Reference{}
				}
				var wantDiags tfdiags.Diagnostics
				return newCtx, targetRes, targetResRange, directRefCb, dependsOnCb, false, true, wantDiags // the callback for direct references is not called
			},
		},
		"no callback calls when linting rule is not enabled": {
			setup: func(ctx context.Context) (context.Context, addrs.ConfigResource, hcl.Range, func() []*addrs.Reference, func() []*addrs.Reference, bool, bool, tfdiags.Diagnostics) {
				newCtx := tfdiags.ContextWithLintFilterHints(ctx, collections.NewSet[linting.RuleAddr](), nil)
				targetRes := addrs.MustParseResourceAddr("test.resource_name")
				targetResRange := hcl.Range{Filename: "test.tf", Start: hcl.Pos{Line: 1, Byte: 4, Column: 4}, End: hcl.Pos{Line: 1, Byte: 10, Column: 10}}
				directRefCb := func() []*addrs.Reference { return []*addrs.Reference{} }
				dependsOnCb := func() []*addrs.Reference { return []*addrs.Reference{} }
				var wantDiags tfdiags.Diagnostics
				return newCtx, targetRes, targetResRange, directRefCb, dependsOnCb, false, false, wantDiags // no callback called
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, targetRes, targetResRange, directRefCb, dependsOnCb, wantDirectRefCbCalled, wantDependsOnCbCalled, expectedDiags := tc.setup(t.Context())
			var (
				directRefCbCalled bool
				dependsOnCbCalled bool
			)
			wrapDirectRefCb := func() []*addrs.Reference {
				directRefCbCalled = true
				return directRefCb()
			}
			wrapDependsOnCb := func() []*addrs.Reference {
				dependsOnCbCalled = true
				return dependsOnCb()
			}
			gotDiags := RedundantDependsOn(ctx, targetRes, targetResRange, wrapDirectRefCb, wrapDependsOnCb)
			if directRefCbCalled != wantDirectRefCbCalled {
				t.Errorf("unexpected call status for direct references callback. wanted %t but got %t", wantDirectRefCbCalled, directRefCbCalled)
			}
			if dependsOnCbCalled != wantDependsOnCbCalled {
				t.Errorf("unexpected call status for direct references callback. wanted %t but got %t", wantDependsOnCbCalled, dependsOnCbCalled)
			}
			compareDiagnostics(t, expectedDiags, gotDiags)
		})
	}
}

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
		t.Errorf("%sinvalid source. want: %+v but got %+v", prefix, wv, gv)
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
