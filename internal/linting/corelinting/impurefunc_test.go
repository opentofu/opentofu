// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package corelinting

import (
	"context"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/collections"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/linting"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func TestCoreRule_ImpureFunc(t *testing.T) {
	cases := map[string]struct {
		setup func(t *testing.T) (marks.LintingInfo, hcl.Range, tfdiags.Diagnostics)
	}{
		"impure function usage detected in nested attribute": {
			setup: func(t *testing.T) (marks.LintingInfo, hcl.Range, tfdiags.Diagnostics) {
				v := cty.ObjectVal(map[string]cty.Value{
					"inner": cty.ObjectVal(map[string]cty.Value{
						"value": cty.StringVal("my value").Mark(marks.ImpureFuncUsageMark("uuid")),
					}),
				})
				_, linfo := marks.ExtractLintingInformationFromValue(v)
				rng := hcl.Range{
					Filename: "main.tf",
					Start:    hcl.Pos{Line: 1, Column: 2, Byte: 2},
					End:      hcl.Pos{Line: 1, Column: 10, Byte: 11},
				}
				want := tfdiags.New(tfdiags.LintMessage(
					ruleIDImpureFuncUsed,
					[]linting.RuleAddr{GroupIDAll, GroupIDNoConverge},
					"Impure function used in a location where convergence is typically expected",
					`Argument ".inner.value" value computed by using impure function "uuid"`,
					new(tfdiags.SourceRangeFromHCL(rng)),
					nil,
				))
				return linfo, rng, want
			},
		},
		"impure function detected at root level": {
			setup: func(t *testing.T) (marks.LintingInfo, hcl.Range, tfdiags.Diagnostics) {
				v := cty.ObjectVal(map[string]cty.Value{
					"value": cty.StringVal("my value").Mark(marks.ImpureFuncUsageMark("uuid")),
				})
				_, linfo := marks.ExtractLintingInformationFromValue(v)
				rng := hcl.Range{
					Filename: "main.tf",
					Start:    hcl.Pos{Line: 1, Column: 2, Byte: 2},
					End:      hcl.Pos{Line: 1, Column: 10, Byte: 11},
				}
				want := tfdiags.New(tfdiags.LintMessage(
					ruleIDImpureFuncUsed,
					[]linting.RuleAddr{GroupIDAll, GroupIDNoConverge},
					"Impure function used in a location where convergence is typically expected",
					`Argument ".value" value computed by using impure function "uuid"`,
					new(tfdiags.SourceRangeFromHCL(rng)),
					nil,
				))
				return linfo, rng, want
			},
		},
		"impure function detected on simple value": {
			setup: func(t *testing.T) (marks.LintingInfo, hcl.Range, tfdiags.Diagnostics) {
				v := cty.StringVal("my value").Mark(marks.ImpureFuncUsageMark("uuid"))
				_, linfo := marks.ExtractLintingInformationFromValue(v)
				rng := hcl.Range{
					Filename: "main.tf",
					Start:    hcl.Pos{Line: 1, Column: 2, Byte: 2},
					End:      hcl.Pos{Line: 1, Column: 10, Byte: 11},
				}
				want := tfdiags.New(tfdiags.LintMessage(
					ruleIDImpureFuncUsed,
					[]linting.RuleAddr{GroupIDAll, GroupIDNoConverge},
					"Impure function used in a location where convergence is typically expected",
					`Value computed by using impure function "uuid"`,
					new(tfdiags.SourceRangeFromHCL(rng)),
					nil,
				))
				return linfo, rng, want
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			runAgainstMultipleIdentifiers(t, func(t *testing.T, ctx context.Context) {
				lintingInfo, rng, expectedDiags := tc.setup(t)
				gotDiags := ImpureFuncs(ctx, rng, lintingInfo)
				compareDiagnostics(t, expectedDiags, gotDiags)
			}, []linting.RuleAddr{ruleIDImpureFuncUsed}, []linting.RuleAddr{GroupIDNoConverge})
		})
	}

	t.Run("running impurefunc without necessary information skips recording its execution", func(t *testing.T) {
		ctx := tfdiags.ContextWithLintFilterHints(t.Context(), collections.NewSet(ruleIDImpureFuncUsed), nil)
		v := cty.StringVal("my value") // no linting mark so the next extract call returns no info
		_, linfo := marks.ExtractLintingInformationFromValue(v)
		rng := hcl.Range{
			Filename: "main.tf",
			Start:    hcl.Pos{Line: 1, Column: 2, Byte: 2},
			End:      hcl.Pos{Line: 1, Column: 10, Byte: 11},
		}
		want := tfdiags.New()
		// calling the linting rule with an empty list of linting information will result in skipping from recording the
		// rule as executed for the given configuration source range
		got := ImpureFuncs(ctx, rng, linfo)
		compareDiagnostics(t, want, got)

		// now marking the value, will result in having at least one linting information for the rule to process
		v = v.Mark(marks.ImpureFuncUsageMark("uuid"))
		_, linfo = marks.ExtractLintingInformationFromValue(v)
		want = tfdiags.New(tfdiags.LintMessage(
			ruleIDImpureFuncUsed,
			[]linting.RuleAddr{GroupIDAll, GroupIDNoConverge},
			"Impure function used in a location where convergence is typically expected",
			`Value computed by using impure function "uuid"`,
			new(tfdiags.SourceRangeFromHCL(rng)),
			nil,
		))
		// calling again the linting rule with the same source range, it will run it again and record it as executed
		got = ImpureFuncs(ctx, rng, linfo)
		compareDiagnostics(t, want, got)

		// to ensure that the previous execution has been recorded correctly, let's call it one more time and we expect no diags to be returned
		want = tfdiags.New()
		got = ImpureFuncs(ctx, rng, linfo)
		compareDiagnostics(t, want, got)
	})
}
