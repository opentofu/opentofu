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
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func compileModuleInstanceInputVariables(_ context.Context, configs map[string]*configs.Variable, values exprs.Valuer, declScope exprs.Scope, moduleInstAddr addrs.ModuleInstance, missingDefRange *tfdiags.SourceRange) map[addrs.InputVariable]*configgraph.InputVariable {
	ret := make(map[addrs.InputVariable]*configgraph.InputVariable, len(configs))
	for name, vc := range configs {
		addr := addrs.InputVariable{Name: name}

		// The valuer for an individual input variable derives from the
		// valuer for the single object representing all of the input
		// variables together.
		rawValuer := compileInputVariableValuer(values, vc, moduleInstAddr, missingDefRange)
		ret[addr] = &configgraph.InputVariable{
			Addr:           moduleInstAddr.InputVariable(name),
			RawValue:       configgraph.ValuerOnce(rawValuer),
			TargetType:     vc.ConstraintType,
			TargetDefaults: vc.TypeDefaults,
			FinalizeValue: func(_ context.Context, v cty.Value) (cty.Value, tfdiags.Diagnostics) {
				var diags tfdiags.Diagnostics
				if vc.Sensitive {
					v = v.Mark(marks.Sensitive)
				}
				if vc.Ephemeral {
					v = v.Mark(marks.Ephemeral)
				} else {
					// At the boundary between modules we treat ephemerality
					// as a static concern, so that module authors don't need to
					// defensively handle ephemeral values in all input
					// variables. An input variable is either entirely ephemeral
					// or not ephemeral at all.
					if v.HasMarkDeep(marks.Ephemeral) {
						diags = diags.Append(&hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Invalid value for input variable",
							Detail:   fmt.Sprintf("The given value is derived from an ephemeral object, but %s is not declared as ephemeral.", addr),
							Subject:  configgraph.MaybeHCLSourceRange(rawValuer.ValueSourceRange()),
						})
					}
				}
				// TODO: Do we want to do something with "const" here too?
				// In our new runtime we don't have an explicit concept of
				// "early evaluation" and so if we want to support this we'll
				// need to find a more nuanced definition of what it means for
				// a variable to be "constant", such as disallowing it being
				// derived from any resource instances, having any unknown
				// values inside it and/or having ephemeral values anywhere.
				// TODO: Handle "Deprecated" in here too.
				return v, diags
			},
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

func compileInputVariableValuer(valuesValuer exprs.Valuer, config *configs.Variable, moduleInstAddr addrs.ModuleInstance, missingDefRange *tfdiags.SourceRange) exprs.Valuer {
	name := config.Name
	return exprs.DerivedValuer(valuesValuer, func(values cty.Value, _ tfdiags.Diagnostics) (cty.Value, tfdiags.Diagnostics) {
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
		if valueRange := valuesValuer.ValueSourceRange(); valueRange != nil {
			defRange = valueRange
		}

		ty := values.Type()
		if ty == cty.DynamicPseudoType {
			return cty.DynamicVal.WithSameMarks(values), diags
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
			return cty.DynamicVal.WithSameMarks(values), diags
		}
		if values.IsNull() {
			// Again this suggests a bug in the caller, but we'll handle
			// it for robustness.
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid input values",
				Detail:   fmt.Sprintf("The object describing the input values for %s must not be null.", moduleInstAddr),
				Subject:  configgraph.MaybeHCLSourceRange(defRange),
			})
			return cty.DynamicVal.WithSameMarks(values), diags
		}

		// "Required" and "Nullable" are related but separate:
		//
		// "Required" means that an attribute representing the variable
		// must be present in the object that's representing all of the
		// input variables, but still allows the explicit definition to
		// assign it the value "null". A non-required (i.e. optional) input
		// variable always has a default value, which is allowed to be null
		// if the variable is nullable.
		//
		// "Nullable" means that the value of the attribute may not be
		// null, regardless of whether that null is explicitly set or
		// implied by omission. If assigned null when a default is available
		// then the default value is used instead of failing.

		if !ty.HasAttribute(name) {
			if config.Required() {
				var diags tfdiags.Diagnostics
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Missing definition for required input variable",
					Detail:   fmt.Sprintf("Input variable %q is required, and so it must be provided as an argument to this module.", name),
					Subject:  configgraph.MaybeHCLSourceRange(defRange),
				})
				return cty.DynamicVal.WithSameMarks(values), diags
			}
			return config.Default, diags
		}

		// If we get here then the variable is definitely defined, but we don't
		// yet know if it's null or not.
		v := values.GetAttr(name)
		if !config.Nullable && v.IsNull() {
			if config.Required() {
				var diags tfdiags.Diagnostics
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid value for input variable",
					Detail:   fmt.Sprintf("Input variable %q is required and not nullable, and so it cannot be set to null.", name),
					Subject:  configgraph.MaybeHCLSourceRange(defRange),
				})
				return cty.DynamicVal.WithSameMarks(values), diags
			}
			return config.Default, diags
		}
		return v, diags
	})
}
