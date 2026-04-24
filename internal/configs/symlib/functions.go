package symlib

import (
	"fmt"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
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
				//AllowMarked?
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
					panic("BUG")
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
				// TODO dupe check locals

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

func (fn *Function) Impl(w *workgraph.Worker, s *scope) (function.Function, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	spec := &function.Spec{
		Description: fn.Description,
	}

	returnType := cty.DynamicPseudoType
	if fn.ReturnType != nil {
		typeCtx, tDiags := s.typeContext(w, fn.ReturnType)
		diags = diags.Extend(tDiags)

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
		}

		validations[fnp.Name] = param.Validations

		if param.TypeExpr != nil {
			typeCtx, tDiags := s.typeContext(w, *param.TypeExpr)
			diags = diags.Extend(tDiags)

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

	spec.Impl = func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		var diags hcl.Diagnostics

		// This could also be accomplished by creating a full evalcontext
		// and building deps internally via workgraph
		w := workgraph.NewWorker()
		s := s.clone()

		for i, arg := range args[:len(spec.Params)] {
			param := spec.Params[i]

			if defaults[param.Name] != nil && !arg.IsNull() {
				arg = defaults[param.Name].Apply(arg)
			}
			s.addVar("param", param.Name, &hclsyntax.LiteralValueExpr{Val: arg})
		}
		if spec.VarParam != nil {
			// TODO defaults + validations
			if len(spec.Params) != len(args) {
				s.addVar("param", spec.VarParam.Name, &hclsyntax.LiteralValueExpr{Val: cty.ListVal(args[len(spec.Params):])})
			} else {
				s.addVar("param", spec.VarParam.Name, &hclsyntax.LiteralValueExpr{Val: cty.ListValEmpty(spec.VarParam.Type)})
			}
		}

		for i := range args[:len(spec.Params)] {
			param := spec.Params[i]

			for _, validation := range validations[param.Name] {
				hclCtx, hDiags := s.evalContext(w, validation.Condition)
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
					hclCtx, hDiags := s.evalContext(w, validation.ErrorMessage)
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
					})
				}
			}
		}

		for name, expr := range fn.Locals {
			s.addVar("local", name, expr)
		}

		hclCtx, hDiags := s.evalContext(w, fn.Return)
		diags = diags.Extend(hDiags)

		val, vDiags := fn.Return.Value(hclCtx)
		diags = diags.Extend(vDiags)

		if diags.HasErrors() {
			return val, error(diags)
		}
		return val, nil
	}

	return function.New(spec), diags
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
