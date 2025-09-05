// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"fmt"
	"iter"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestInputVariable_Value(t *testing.T) {
	testValuer(t, map[string]valuerTest[*InputVariable]{
		"no constraints whatsoever": {
			&InputVariable{
				RawValue:   constantOnceValuer(cty.StringVal("Hello, world!")),
				TargetType: cty.DynamicPseudoType, // like "type = any" in the surface language
			},
			cty.StringVal("Hello, world!"),
			nil,
		},
		"valid type conversion": {
			&InputVariable{
				RawValue:   constantOnceValuer(cty.True),
				TargetType: cty.String,
			},
			cty.StringVal("true"),
			nil,
		},
		"invalid type conversion": {
			&InputVariable{
				Addr:       addrs.InputVariable{Name: "foo"}.Absolute(addrs.RootModuleInstance),
				RawValue:   constantOnceValuerWithSource(cty.EmptyObjectVal, tfdiags.SourceRange{Filename: "test.tf"}),
				TargetType: cty.String,
			},
			exprs.AsEvalError(cty.UnknownVal(cty.String)),
			diagsForTest(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid value for input variable",
				Detail:   `Unsuitable value for variable "foo": string required, but have object.`,
				Subject:  &hcl.Range{Filename: "test.tf"},
			}),
		},
		"object type with optional attributes": {
			&InputVariable{
				RawValue: constantOnceValuer(cty.EmptyObjectVal),
				TargetType: cty.ObjectWithOptionalAttrs(map[string]cty.Type{
					"a": cty.String,
					"b": cty.Bool,
				}, []string{"a", "b"}),
				TargetDefaults: &typeexpr.Defaults{
					Type: cty.ObjectWithOptionalAttrs(map[string]cty.Type{
						"a": cty.String,
						"b": cty.Bool,
					}, []string{"a", "b"}),
					DefaultValues: map[string]cty.Value{
						"b": cty.False,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"a": cty.NullVal(cty.String), // because no default value was provided
				"b": cty.False,               // because of the provided default value
			}),
			nil,
		},
		"failing custom validation rule": {
			&InputVariable{
				Addr:       addrs.InputVariable{Name: "foo"}.Absolute(addrs.RootModuleInstance),
				RawValue:   constantOnceValuerWithSource(cty.EmptyObjectVal, tfdiags.SourceRange{Filename: "parent.tf"}),
				TargetType: cty.EmptyObject,
				CompileValidationRules: func(ctx context.Context, value cty.Value) iter.Seq[*CheckRule] {
					return func(yield func(*CheckRule) bool) {
						yield(&CheckRule{
							// Marks from the condition should carry over to the result
							// because the condition was used in the decision for whether
							// to let the value pass through or not.
							ConditionValuer: constantOnceValuer(cty.False.Mark("beep boop")),
							// Marks from the error message DO NOT carry over, because they
							// are used only in the error message when things fail, and not
							// as part of the decision.
							ErrorMessageValuer: constantOnceValuer(cty.StringVal(fmt.Sprintf("The value was %#v.", value)).Mark("oh no")),
							DeclSourceRange:    tfdiags.SourceRange{Filename: "child.tf"},
						})
					}
				},
			},
			exprs.AsEvalError(cty.UnknownVal(cty.EmptyObject)).Mark("beep boop"),
			diagsForTest(&hcl.Diagnostic{
				// Note that the "Subject" of the error is the location where
				// the value came from, because that's the value that we're
				// saying is invalid here, but the text of the error message
				// also mentions the location of the check rule so an author
				// can find it if they want to learn more about it.
				Severity: hcl.DiagError,
				Summary:  "Invalid value for input variable",
				Detail: `The value was cty.EmptyObjectVal.

This problem was reported by the validation rule at child.tf:0,0.`,
				Subject: &hcl.Range{Filename: "parent.tf"},
			}),
		},
		"passing custom validation rule": {
			&InputVariable{
				Addr:       addrs.InputVariable{Name: "foo"}.Absolute(addrs.RootModuleInstance),
				RawValue:   constantOnceValuerWithSource(cty.EmptyObjectVal, tfdiags.SourceRange{Filename: "parent.tf"}),
				TargetType: cty.EmptyObject,
				CompileValidationRules: func(ctx context.Context, value cty.Value) iter.Seq[*CheckRule] {
					return func(yield func(*CheckRule) bool) {
						yield(&CheckRule{
							// Marks from the condition should carry over to the result
							// because the condition was used in the decision for whether
							// to let the value pass through or not.
							ConditionValuer: constantOnceValuer(cty.True.Mark("beep boop")),
							// Marks from the error message DO NOT carry over, because they
							// are used only in the error message when things fail, and not
							// as part of the decision.
							ErrorMessageValuer: constantOnceValuer(cty.StringVal(fmt.Sprintf("The value was %#v.", value)).Mark("oh no")),
							DeclSourceRange:    tfdiags.SourceRange{Filename: "child.tf"},
						})
					}
				},
			},
			cty.EmptyObjectVal.Mark("beep boop"),
			nil,
		},
	})
}
