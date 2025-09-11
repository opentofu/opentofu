// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"fmt"
	"iter"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func compileModuleInstanceInputVariables(_ context.Context, configs map[string]*configs.Variable, values exprs.Valuer, declScope exprs.Scope, moduleInstAddr addrs.ModuleInstance, missingDefRange *tfdiags.SourceRange) map[addrs.InputVariable]*configgraph.InputVariable {
	ret := make(map[addrs.InputVariable]*configgraph.InputVariable, len(configs))
	for name, vc := range configs {
		addr := addrs.InputVariable{Name: name}

		// The valuer for an individual input variable derives from the
		// valuer for the single object representing all of the input
		// variables together.
		rawValuer := exprs.DerivedValuer(values, func(v cty.Value, _ tfdiags.Diagnostics) (cty.Value, tfdiags.Diagnostics) {
			// We intentionally avoid passing on the diagnostics from the
			// "values" valuer here both because they will be about the
			// entire object rather than the individual attribute we're
			// interested in and because whatever produced the "values"
			// valuer should've already reported its own errors when
			// it was checked directly.
			//
			// We might return additional diagnostics about the individual
			// atribute we're extracting, though.
			var diags tfdiags.Diagnostics

			defRange := missingDefRange
			if valueRange := values.ValueSourceRange(); valueRange != nil {
				defRange = valueRange
			}

			ty := v.Type()
			if ty == cty.DynamicPseudoType {
				return cty.DynamicVal.WithSameMarks(v), diags
			}
			if !ty.IsObjectType() {
				// Should not get here because the caller should always pass
				// us an object type based on the arguments in the module
				// call, but we'll deal with it anyway for robustness.
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid input values",
					Detail:   fmt.Sprintf("Input variable values for %s module must be provided as an object value, not %s.", moduleInstAddr, ty.FriendlyName()),
					Subject:  configgraph.MaybeHCLSourceRange(defRange),
				})
				return cty.DynamicVal.WithSameMarks(v), diags
			}
			if v.IsNull() {
				// Again this suggests a bug in the caller, but we'll handle
				// it for robustness.
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid input values",
					Detail:   fmt.Sprintf("The object describing the input values for %s must not be null.", moduleInstAddr),
					Subject:  configgraph.MaybeHCLSourceRange(defRange),
				})
				return cty.DynamicVal.WithSameMarks(v), diags
			}

			if !ty.HasAttribute(name) {
				if vc.Required() {
					// We don't actually _need_ to handle an error here because
					// the final evaluation of the variables must deal with the
					// possibility of the final value being null anyway, but
					// by handling this here we can produce a more helpful error
					// message that talks about the definition being statically
					// absent instead of dynamically null.
					var diags tfdiags.Diagnostics
					diags = diags.Append(&hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Missing definition for required input variable",
						Detail:   fmt.Sprintf("Input variable %q is required, and so it must be provided as an argument to this module.", name),
						Subject:  configgraph.MaybeHCLSourceRange(defRange),
					})
					return cty.DynamicVal.WithSameMarks(v), diags
				} else {
					// For a non-required variable we'll provide a placeholder
					// null value so that the evaluator can treat this the same
					// as if there was an explicit definition evaluating to null.
					return cty.NullVal(cty.DynamicPseudoType).WithSameMarks(v), diags
				}
			}
			// After all of the checks above we should now be able to call
			// GetAttr for this name without panicking. (If v is unknown
			// or marked then cty will automatically return a derived unknown
			// or marked value.)
			return v.GetAttr(name), diags
		})
		ret[addr] = &configgraph.InputVariable{
			Addr:           moduleInstAddr.InputVariable(name),
			RawValue:       configgraph.ValuerOnce(rawValuer),
			TargetType:     vc.ConstraintType,
			TargetDefaults: vc.TypeDefaults,
			CompileValidationRules: func(ctx context.Context, value cty.Value) iter.Seq[*configgraph.CheckRule] {
				// For variable validation we need to use a special overlay
				// scope that resolves the single variable we are validating
				// to the given constant value but delegates everything else
				// to the parent scope. This overlay is important because
				// these checks are run as part of the normal process of
				// handling a reference to this variable, and so if we used
				// the normal scope here we'd be depending on our own result.
				childScope := &inputVariableValidationScope{
					wantName:    name,
					parentScope: declScope,
					finalVal:    value,
				}
				return compileCheckRules(vc.Validations, childScope)
			},
		}
	}
	return ret
}
