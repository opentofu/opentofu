// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/resources"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestPrepareDestroyIfUnwanted(t *testing.T) {
	tests := []struct {
		Value     cty.Value
		Want      exprs.FromValue[resources.ActionIfUnwanted]
		WantDiags tfdiags.Diagnostics
	}{
		{
			Value: cty.True,
			Want:  exprs.Known(resources.IfUnwantedDelete),
		},
		{
			Value: cty.False,
			Want:  exprs.Known(resources.IfUnwantedForget),
		},
		{
			Value: cty.NullVal(cty.Bool),
			Want:  exprs.Known(resources.IfUnwantedUndecided),
		},
		{
			Value: cty.NullVal(cty.DynamicPseudoType),
			Want:  exprs.Known(resources.IfUnwantedUndecided),
		},
		{
			Value: cty.StringVal("true"),
			Want:  exprs.Known(resources.IfUnwantedDelete),
		},
		{
			Value: cty.StringVal("false"),
			Want:  exprs.Known(resources.IfUnwantedForget),
		},
		{
			Value: cty.UnknownVal(cty.Bool),
			Want:  exprs.Unknown[resources.ActionIfUnwanted](),
		},
		{
			Value: cty.DynamicVal,
			Want:  exprs.Unknown[resources.ActionIfUnwanted](),
		},
		{
			Value: cty.ListValEmpty(cty.String),
			Want:  exprs.Unknown[resources.ActionIfUnwanted]().Mark(exprs.EvalError),
			WantDiags: tfdiags.New(
				&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  `Invalid value for "destroy" argument`,
					Detail:   `Test case has an invalid value for its "destroy" argument: bool required, but have list of string.`,
					Subject:  &hcl.Range{Filename: "test.tofu"},
				},
			),
		},
		{
			Value: cty.UnknownVal(cty.Map(cty.String)),
			Want:  exprs.Unknown[resources.ActionIfUnwanted]().Mark(exprs.EvalError),
			WantDiags: tfdiags.New(
				&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  `Invalid value for "destroy" argument`,
					Detail:   `Test case has an invalid value for its "destroy" argument: bool required, but have map of string.`,
					Subject:  &hcl.Range{Filename: "test.tofu"},
				},
			),
		},
		{
			Value: cty.True.Mark(marks.Sensitive),
			Want:  exprs.Known(resources.IfUnwantedDelete).Mark(exprs.EvalError),
			WantDiags: tfdiags.New(
				&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  `Invalid value for "destroy" argument`,
					Detail:   "Test case has a sensitive value for its \"destroy\" argument, which is invalid because it would cause OpenTofu to disclose the sensitive value by whether it proposes deleting or forgetting.\n\nIf you know this value is not sensitive in practice, consider using the nonsensitive function to declare that.",
					Subject:  &hcl.Range{Filename: "test.tofu"},
				},
			),
		},
		{
			Value: cty.True.Mark(marks.Ephemeral),
			Want:  exprs.Known(resources.IfUnwantedDelete).Mark(exprs.EvalError),
			WantDiags: tfdiags.New(
				&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  `Invalid value for "destroy" argument`,
					Detail:   "Test case has an ephemeral value for its \"destroy\" argument, which is invalid because the decision for whether to delete or forget objects belonging to this resource must stay consistent between plan and apply.",
					Subject:  &hcl.Range{Filename: "test.tofu"},
				},
			),
		},
		{
			Value: cty.True.Mark("hypothetical future mark"),
			Want:  exprs.Known(resources.IfUnwantedDelete).Mark(exprs.EvalError),
			WantDiags: tfdiags.New(
				&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  `Invalid value for "destroy" argument`,
					Detail:   `Test case has a "destroy" value derived from something that isn't allowed for deciding whether to delete or forget (has internal mark "hypothetical future mark"). The fact that OpenTofu cannot give more details about this is a bug, so please report it!`,
					Subject:  &hcl.Range{Filename: "test.tofu"},
				},
			),
		},
		{
			Value: cty.True.Mark(NewResourceInstanceMark(&ResourceInstance{})), // this package's own internal marks should be stripped out before returning
			Want:  exprs.Known(resources.IfUnwantedDelete),
		},
	}

	for _, test := range tests {
		t.Run(test.Value.GoString(), func(t *testing.T) {
			t.Logf("testing %#v", test.Value)
			gotWithSourceRange, gotDiags := prepareDestroyIfUnwanted("Test case", test.Value, &tfdiags.SourceRange{
				Filename: "test.tofu",
			})
			if gotWithSourceRange.SourceRange == nil || gotWithSourceRange.SourceRange.Filename != "test.tofu" {
				t.Error("source range was not preserved into result")
			}

			got := gotWithSourceRange.FromValue
			want := test.Want
			if diff := cmp.Diff(want, got, exprs.FromValueCmpOptions); diff != "" {
				t.Error("wrong result\n" + diff)
			}

			// We'll use the "For RPC" representations of diagnostics just
			// because that throws away the various internal representational
			// differences that are not important to this test.
			gotDiags = gotDiags.ForRPC()
			wantDiags := test.WantDiags.ForRPC()
			if diff := cmp.Diff(wantDiags, gotDiags); diff != "" {
				t.Error("wrong diagnostics\n" + diff)
			}
		})
	}
}
