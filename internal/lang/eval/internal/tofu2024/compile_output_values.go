// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"slices"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func compileModuleInstanceOutputValues(
	_ context.Context,
	configs map[string]*configs.Output,
	declScope exprs.Scope,
	moduleInstAddr addrs.ModuleInstance,
	extraMarks cty.ValueMarks,
) map[addrs.OutputValue]*configgraph.OutputValue {
	ret := make(map[addrs.OutputValue]*configgraph.OutputValue, len(configs))
	for name, vc := range configs {
		addr := addrs.OutputValue{Name: name}

		dependsOn, _ := compileDependsOn(vc.DependsOn, declScope, extraMarks)

		value := configgraph.ValuerOnce(exprs.DerivedValuerContext(
			exprs.NewClosure(exprs.EvalableHCLExpression(vc.Expr), declScope),
			func(ctx context.Context, v cty.Value, diags tfdiags.Diagnostics) (cty.Value, tfdiags.Diagnostics) {
				marks, markDiags := dependsOn.Marks(ctx)
				diags = diags.Append(markDiags)
				return v.WithMarks(marks), diags
			},
		))
		ret[addr] = &configgraph.OutputValue{
			Addr:     moduleInstAddr.OutputValue(name),
			RawValue: value,

			// Our current language doesn't allow specifying a type constraint
			// for an output value, so these are always the most liberal
			// possible constraint. Making these customizable could be part
			// of a solution to:
			//     https://github.com/opentofu/opentofu/issues/2831
			TargetType:     cty.DynamicPseudoType,
			TargetDefaults: nil,

			ForceSensitive: vc.Sensitive,
			ForceEphemeral: vc.Ephemeral,
			Preconditions:  slices.Collect(compileCheckRules(vc.Preconditions, declScope)),
		}
	}
	return ret
}
