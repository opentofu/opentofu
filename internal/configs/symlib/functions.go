// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package symlib

import (
	"fmt"
	"slices"
	"strings"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function"
)

type Function struct {
	Name        string
	Description string

	Params     []FunctionParameter
	VarParam   *FunctionParameter
	Locals     map[string]hcl.Expression
	ReturnType hcl.Expression
	Return     hcl.Expression

	DeclRange hcl.Range
}

type FunctionParameter struct {
	Name         string
	Description  string
	TypeExpr     *hcl.Expression
	AllowNull    bool
	AllowUnknown bool
	Validations  []FunctionParameterValidation
}

type FunctionParameterValidation struct {
	Condition    hcl.Expression
	ErrorMessage hcl.Expression
}

func decodeFunctionBlock(block *hcl.Block) (*Function, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	fn := &Function{
		Name:      block.Labels[0],
		DeclRange: block.DefRange,
	}

	content, moreDiags := block.Body.Content(functionBlockSchema)
	diags = diags.Extend(moreDiags)

	if !hclsyntax.ValidIdentifier(fn.Name) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid function name",
			Detail:   badIdentifierDetail,
			Subject:  &block.LabelRanges[0],
		})
	}

	fn.Locals = map[string]hcl.Expression{}

	for _, block := range content.Blocks {
		if block.Type == "parameter" {
			param := FunctionParameter{
				Name:     block.Labels[0],
				TypeExpr: nil,

				AllowUnknown: true,
				AllowNull:    true,
			}
			if !hclsyntax.ValidIdentifier(param.Name) {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid parameter name",
					Detail:   badIdentifierDetail,
					Subject:  &block.LabelRanges[0],
				})
			}

			content, moreDiags := block.Body.Content(functionParameterSchema)
			diags = diags.Extend(moreDiags)

			if attr, ok := content.Attributes["type"]; ok {
				param.TypeExpr = &attr.Expr
			}
			if attr, ok := content.Attributes["description"]; ok {
				valDiags := gohcl.DecodeExpression(attr.Expr, nil, &param.Description)
				diags = append(diags, valDiags...)
			}
			if attr, ok := content.Attributes["allow_unknown"]; ok {
				valDiags := gohcl.DecodeExpression(attr.Expr, nil, &param.AllowUnknown)
				diags = append(diags, valDiags...)
			}
			if attr, ok := content.Attributes["allow_null"]; ok {
				valDiags := gohcl.DecodeExpression(attr.Expr, nil, &param.AllowNull)
				diags = append(diags, valDiags...)
			}

			for _, block := range content.Blocks {
				if block.Type != "validation" {
					panic("unreachable: schema does not allow any block other than validation")
				}

				content, moreDiags := block.Body.Content(functionParameterValidationSchema)
				diags = diags.Extend(moreDiags)
				param.Validations = append(param.Validations, FunctionParameterValidation{
					Condition:    content.Attributes["condition"].Expr,
					ErrorMessage: content.Attributes["error_message"].Expr,
				})
			}

			variadic := false
			if attr, ok := content.Attributes["variadic"]; ok {
				valDiags := gohcl.DecodeExpression(attr.Expr, nil, &variadic)
				diags = append(diags, valDiags...)
			}

			if variadic {
				fn.VarParam = &param
			} else {
				fn.Params = append(fn.Params, param)
			}
		}

		if block.Type == "locals" {
			attrs, diags := block.Body.JustAttributes()
			for name, attr := range attrs {
				if !hclsyntax.ValidIdentifier(name) {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid local value name",
						Detail:   badIdentifierDetail,
						Subject:  &attr.NameRange,
					})
				}
				if existing, ok := fn.Locals[name]; ok {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Duplicate local definition",
						Detail:   fmt.Sprintf("A local named %q was already defined at %s. Local names must be unique within a function.", name, existing.Range()),
						Subject:  &attr.NameRange,
					})
				}

				fn.Locals[name] = attr.Expr
			}
		}
	}

	if attr, ok := content.Attributes["description"]; ok {
		valDiags := gohcl.DecodeExpression(attr.Expr, nil, &fn.Description)
		diags = append(diags, valDiags...)
	}

	if attr, ok := content.Attributes["type"]; ok {
		fn.ReturnType = attr.Expr
	}

	if attr, ok := content.Attributes["return"]; ok {
		fn.Return = attr.Expr
	}

	return fn, diags
}

type functionWithStack func(w func() *workgraph.Worker, stack []string) function.Function

// ForInternalCaller produces a function for use within a symlib scope where the worker is already known
func (f functionWithStack) ForInternalCaller(w *workgraph.Worker, stack []string) function.Function {
	return f(func() *workgraph.Worker { return w }, stack)
}

// ForExternalCaller produces a function for use outside of the scope that it resides in. This is helpful
// when exposing symlib to callers that may be calling the same function from multiple go-routines.
//
// Future Note: We may want OpenTofu's new runtime to share it's current worker with the symlib function it's
// calling, though the benefits of that are still to be debated.
func (f functionWithStack) ForExternalCaller() function.Function {
	return f(func() *workgraph.Worker { return workgraph.NewWorker() }, nil)
}

func (fn *Function) Compile(w *workgraph.Worker, libScope *symbolScope) (functionWithStack, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	spec := &function.Spec{
		Description: fn.Description,
	}

	typeCtx := typeContext(w, libScope)

	returnType := cty.DynamicPseudoType
	if fn.ReturnType != nil {
		var valDiags hcl.Diagnostics
		returnType, _, valDiags = typeCtx.TypeConstraintWithDefaults(fn.ReturnType)
		diags = append(diags, valDiags...)
	}
	spec.Type = function.StaticReturnType(returnType)

	defaults := map[string]*typeexpr.Defaults{}
	validations := map[string][]FunctionParameterValidation{}

	decodeParam := func(param FunctionParameter) (function.Parameter, hcl.Diagnostics) {
		fnp := function.Parameter{
			Name:         param.Name,
			Description:  param.Description,
			Type:         cty.DynamicPseudoType,
			AllowNull:    param.AllowNull,
			AllowUnknown: param.AllowUnknown,
			AllowMarked:  true,
		}

		validations[fnp.Name] = param.Validations

		if param.TypeExpr != nil {
			var valDiags hcl.Diagnostics
			fnp.Type, defaults[fnp.Name], valDiags = typeCtx.TypeConstraintWithDefaults(*param.TypeExpr)
			return fnp, valDiags
		}

		return fnp, nil
	}

	for _, param := range fn.Params {
		fnp, pDiags := decodeParam(param)
		diags = append(diags, pDiags...)
		spec.Params = append(spec.Params, fnp)
	}
	if fn.VarParam != nil {
		fnp, pDiags := decodeParam(*fn.VarParam)
		diags = append(diags, pDiags...)
		spec.VarParam = &fnp
	}

	return func(wf func() *workgraph.Worker, stack []string) function.Function {
		// Duplicate spec struct for impl overrides
		spec := new(*spec)
		spec.Impl = func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			var diags hcl.Diagnostics

			// Ensure each call to this function has it's own copy of the stack
			stack := slices.Clone(stack)
			// Ensure each call uses the correct worker
			w := wf()

			callName := TypeSymbols + "::" + fn.Name
			for i, entry := range stack {
				if entry == callName {
					return cty.NilVal, fmt.Errorf("Recursive call to %s detected, call stack: %s", callName, strings.Join(append(stack[i:], callName+"()"), "() -> "))
				}
			}
			stack = append(stack, callName)

			paramEntries := map[string]cty.Value{}
			for i, arg := range args[:len(spec.Params)] {
				param := spec.Params[i]

				if defaults[param.Name] != nil && !arg.IsNull() {
					arg = defaults[param.Name].Apply(arg)
				}
				paramEntries[param.Name] = arg
			}
			if spec.VarParam != nil {
				if len(spec.Params) != len(args) {
					paramEntries[spec.VarParam.Name] = cty.ListVal(args[len(spec.Params):])
				} else {
					paramEntries[spec.VarParam.Name] = cty.ListValEmpty(spec.VarParam.Type)
				}
			}

			funcScope := newFunctionScope(libScope, fn.Name, paramEntries)

			for i := range args[:len(spec.Params)] {
				param := spec.Params[i]

				for _, validation := range validations[param.Name] {
					hclCtx, hDiags := hclContext(w, funcScope, validation.Condition, stack)
					diags = diags.Extend(hDiags)
					if hDiags.HasErrors() {
						continue
					}

					condVal, cDiags := validation.Condition.Value(hclCtx)
					diags = diags.Extend(cDiags)

					if cDiags.HasErrors() {
						continue
					}

					if condVal.IsKnown() && condVal.False() {
						hclCtx, hDiags := hclContext(w, funcScope, validation.ErrorMessage, stack)
						diags = diags.Extend(hDiags)

						msgVal, mDiags := validation.ErrorMessage.Value(hclCtx)
						diags = diags.Extend(mDiags)

						if !msgVal.IsKnown() {
							println(msgVal.Type().GoString())
							continue
						}

						diags = diags.Append(&hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Function parameter failed validation",
							Detail:   fmt.Sprintf("Parameter %q: %s", param.Name, msgVal.AsString()),
							Subject:  validation.Condition.Range().Ptr(),
						})
					}
				}
			}

			// Now that parameters have been processed, we can add locals
			funcScope.locals = map[string]valuer[cty.Value]{}
			for name, expr := range fn.Locals {
				id := ident{name: "local." + name, src: expr.Range().Ptr()}
				funcScope.locals[name] = onceValuer[cty.Value](funcScope, id, func(w *workgraph.Worker) (cty.Value, hcl.Diagnostics) {
					hclCtx, diags := hclContext(w, funcScope, expr, stack)
					if diags.HasErrors() {
						return cty.NilVal, diags
					}
					val, vDiags := expr.Value(hclCtx)
					diags = diags.Extend(vDiags)
					return val, diags
				})
			}

			hclCtx, hDiags := hclContext(w, funcScope, fn.Return, stack)
			diags = diags.Extend(hDiags)

			if diags.HasErrors() {
				return cty.NilVal, error(diags)
			}

			val, vDiags := fn.Return.Value(hclCtx)
			diags = diags.Extend(vDiags)

			if diags.HasErrors() {
				return val, error(diags)
			}

			// Ensure that we convert to the return type here instead of the raw val
			typedVal, err := convert.Convert(val, retType)
			if err != nil {
				return cty.NilVal, err
			}
			return typedVal, nil
		}

		return function.New(spec)
	}, diags
}

var functionParameterSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "type"},
		{Name: "description"},
		{Name: "variadic"},
		{Name: "allow_null"},
		{Name: "allow_unknown"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "validation"},
	},
}

var functionParameterValidationSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "condition", Required: true},
		{Name: "error_message", Required: true},
	},
}

var functionBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "description"},
		{Name: "return", Required: true},
		{Name: "type"},
		// TODO check conditions
	},
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type:       "parameter",
			LabelNames: []string{"name"},
		},
		{Type: "locals"},
	},
}

func DecompactFunctionErrors(diags hcl.Diagnostics) hcl.Diagnostics {
	var out hcl.Diagnostics
	for _, diag := range diags {
		if funcExtra, ok := diag.Extra.(hclsyntax.FunctionCallDiagExtra); ok {
			err := funcExtra.FunctionCallError()
			if moreDiags, ok := err.(hcl.Diagnostics); ok {
				diag.Extra = nil
				diag.Detail = fmt.Sprintf("Call to function %s failed, see additional diagnostics for more details", funcExtra.CalledFunctionName())

				// Include original diagnostic for tracability
				out = out.Append(diag)
				// Append decompacted diags
				out = out.Extend(DecompactFunctionErrors(moreDiags))
				continue
			}
		}
		out = out.Append(diag)
	}
	return out
}
