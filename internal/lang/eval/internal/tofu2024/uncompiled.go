// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"fmt"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type uncompiledModule struct {
	sourceAddr addrs.ModuleSource
	mod        *configs.Module
}

func NewUncompiledModule(sourceAddr addrs.ModuleSource, mod *configs.Module) evalglue.UncompiledModule {
	return &uncompiledModule{sourceAddr, mod}
}

// ValidateModuleInputs implements evalglue.UncompiledModule.
func (u *uncompiledModule) ValidateModuleInputs(ctx context.Context, inputsVal cty.Value) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	ty := inputsVal.Type()
	if !ty.IsObjectType() {
		// NOTE: As long as the rest of the system is building inputsVal by
		// decoding an HCL body we shouldn't be able to get here, so this
		// case is just here for robustness in case we change some assumptions
		// in future.
		diags = diags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Invalid module inputs value",
			"Module inputs must be represented as an object whose attributes correspond to the child module's input variables.",
			nil, // empty path representing that the top-level object is invalid
		))
	}

	decls := u.mod.Variables
	for name, decl := range decls {
		if !ty.HasAttribute(name) {
			if decl.Required() {
				diags = diags.Append(tfdiags.AttributeValue(
					tfdiags.Error,
					"Missing definition for required input variable",
					fmt.Sprintf("The child module requires a value for the input variable %q.", name),
					cty.GetAttrPath(name),
				))
			}
			continue
		}
		v := inputsVal.GetAttr(name)
		wantTy := decl.ConstraintType
		_, err := convert.Convert(v, wantTy)
		if err != nil {
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Invalid value for input variable",
				fmt.Sprintf("Unsuitable value for input variable %q: %s.", name, tfdiags.FormatError(err)),
				cty.GetAttrPath(name),
			))
		}
	}

	for name := range ty.AttributeTypes() {
		_, declared := decls[name]
		if !declared {
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Definition for undeclared input variable",
				fmt.Sprintf("The child module has no input variable named %q.", name),
				cty.GetAttrPath(name),
			))
		}
	}

	return diags
}

// ModuleOutputsTypeConstraint implements evalglue.UncompiledModule.
func (u *uncompiledModule) ModuleOutputsTypeConstraint(ctx context.Context) cty.Type {
	atys := make(map[string]cty.Type)
	for name := range u.mod.Outputs {
		// We don't currently support type constraints on output values, so
		// they are always cty.DynamicPseudoType to represent what the surface
		// language calls "any".
		atys[name] = cty.DynamicPseudoType
	}
	return cty.Object(atys)
}

// CompileModuleInstance implements evalglue.UncompiledModule.
func (u *uncompiledModule) CompileModuleInstance(ctx context.Context, calleeAddr addrs.ModuleInstance, call *evalglue.ModuleCall) (evalglue.CompiledModuleInstance, tfdiags.Diagnostics) {
	rootModuleCall := &ModuleInstanceCall{
		CalleeAddr:           calleeAddr,
		InputValues:          call.InputValues,
		EvaluationGlue:       call.EvaluationGlue,
		AllowImpureFunctions: call.AllowImpureFunctions,
		EvalContext:          call.EvalContext,
	}
	rootModuleInstance := CompileModuleInstance(ctx, u.mod, u.sourceAddr, rootModuleCall)
	return rootModuleInstance, nil
}
