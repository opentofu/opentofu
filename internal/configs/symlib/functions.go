package symlib

import (
	"fmt"

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
}

// Easier to ignore overrides for prototyping
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

func (fn *Function) Impl(lib *Library) (function.Function, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	typeCtx := lib.TypeContext

	spec := &function.Spec{
		Description: fn.Description,
	}

	returnType := cty.DynamicPseudoType
	if fn.ReturnType != nil {
		var valDiags hcl.Diagnostics
		returnType, _, valDiags = typeCtx.TypeConstraintWithDefaults(fn.ReturnType)
		diags = append(diags, valDiags...)
	}
	spec.Type = function.StaticReturnType(returnType)

	defaults := map[string]*typeexpr.Defaults{}

	decodeParam := func(param FunctionParameter) (function.Parameter, hcl.Diagnostics) {
		fnp := function.Parameter{
			Name:         param.Name,
			Description:  param.Description,
			Type:         cty.DynamicPseudoType,
			AllowNull:    param.AllowNull,
			AllowUnknown: param.AllowUnknown,
		}

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

	spec.Impl = func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		// This is bad and I should feel bad

		hclCtx := &hcl.EvalContext{
			Variables: map[string]cty.Value{},
			// TODO builtin functions and references to other functions in the configuration
		}

		paramObj := map[string]cty.Value{}
		for i, arg := range args[:len(spec.Params)] {
			param := spec.Params[i]

			if defaults[param.Name] != nil && !arg.IsNull() {
				arg = defaults[param.Name].Apply(arg)
			}

			paramObj[param.Name] = arg
		}
		if spec.VarParam != nil && len(spec.Params) != len(args) {
			paramObj[spec.VarParam.Name] = cty.ListVal(args[len(spec.Params):])
		}
		hclCtx.Variables["param"] = cty.ObjectVal(paramObj)

		localObj := map[string]cty.Value{}
		hclCtx.Variables["local"] = cty.ObjectVal(localObj)

		// TODO track stack / circuilar dependencies
		var computeValue func(expr hcl.Expression) (cty.Value, error)
		computeValue = func(expr hcl.Expression) (cty.Value, error) {
			for _, trav := range expr.Variables() {
				if len(trav) < 2 {
					return cty.NilVal, fmt.Errorf("Bad traversal: %#v", trav)
				}

				switch trav[0].(hcl.TraverseRoot).Name {
				case "param":
					// Could validate if we care
				case "local":
					localName := trav[1].(hcl.TraverseAttr).Name
					if _, ok := localObj[localName]; !ok {
						var computeErr error
						localObj[localName], computeErr = computeValue(fn.Locals[localName])
						hclCtx.Variables["local"] = cty.ObjectVal(localObj)
						if computeErr != nil {
							return cty.NilVal, computeErr
						}
					}
				default:
					return cty.NilVal, fmt.Errorf("Bad traversal: %#v", trav)
				}
			}

			val, vDiags := expr.Value(hclCtx)
			if vDiags.HasErrors() {
				return val, vDiags
			}
			return val, nil
		}

		return computeValue(fn.Return)
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
