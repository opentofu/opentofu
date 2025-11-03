// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

type Function struct {
	Name string

	Description string
	// Deprecated?

	Parameters []*FunctionParameter
	Scratch    map[string]*FunctionScratch
	Return     FunctionReturn

	DeclRange hcl.Range
}

type FunctionParameter struct {
	Name      string
	DeclRange hcl.Range

	Type        cty.Type
	Validations []*CheckRule

	// Unsure at this point
	Sensitive bool
	Ephemeral bool

	Nullable bool
	Variadic bool
}

type FunctionScratch struct {
	Name string
	Expr hcl.Expression

	DeclRange hcl.Range
}

type FunctionReturn struct {
	Expr hcl.Expression

	Sensitive bool

	Preconditions []*CheckRule

	DeclRange hcl.Range
}

func decodeFunctionBlock(block *hcl.Block) (*Function, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	fn := &Function{
		Name:      block.Labels[0],
		DeclRange: block.DefRange,

		Scratch: map[string]*FunctionScratch{},
	}

	if !hclsyntax.ValidIdentifier(fn.Name) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid function name",
			Detail:   badIdentifierDetail,
			Subject:  &block.LabelRanges[0],
		})
	}

	content, moreDiags := block.Body.Content(functionBlockSchema)
	diags = append(diags, moreDiags...)

	if attr, ok := content.Attributes["description"]; ok {
		decodeDiags := gohcl.DecodeExpression(attr.Expr, nil, &fn.Description)
		diags = append(diags, decodeDiags...)
	}

	for _, block := range content.Blocks {
		switch block.Type {
		case "parameter":
			param := &FunctionParameter{
				Name:      block.Labels[0],
				DeclRange: block.DefRange,
			}

			content, moreDiags := block.Body.Content(functionParameterBlockSchema)
			diags = append(diags, moreDiags...)

			if attr, exists := content.Attributes["type"]; exists {
				ty, _, _, tyDiags := decodeVariableType(attr.Expr)
				diags = append(diags, tyDiags...)
				//param.ConstraintType = ty
				//param.Typeaults = tyaults
				param.Type = ty.WithoutOptionalAttributesDeep()
			}

			if attr, exists := content.Attributes["sensitive"]; exists {
				valDiags := gohcl.DecodeExpression(attr.Expr, nil, &param.Sensitive)
				diags = append(diags, valDiags...)
			}

			if attr, exists := content.Attributes["ephemeral"]; exists {
				valDiags := gohcl.DecodeExpression(attr.Expr, nil, &param.Ephemeral)
				diags = append(diags, valDiags...)
			}

			if attr, exists := content.Attributes["nullable"]; exists {
				valDiags := gohcl.DecodeExpression(attr.Expr, nil, &param.Nullable)
				diags = append(diags, valDiags...)
			} else {
				// The current default is true, which is subject to change in a future
				// language edition.
			}

			if attr, exists := content.Attributes["variadic"]; exists {
				valDiags := gohcl.DecodeExpression(attr.Expr, nil, &param.Variadic)
				diags = append(diags, valDiags...)
			}

			for _, block := range content.Blocks {
				switch block.Type {

				case "validation":
					vv, moreDiags := decodeVariableValidationBlock(param.Name, block, false)
					diags = append(diags, moreDiags...)
					param.Validations = append(param.Validations, vv)

				default:
					// The above cases should be exhaustive for all block types
					// defined in functionReturnBlockSchema
					panic(fmt.Sprintf("unhandled block type %q", block.Type))
				}
			}

			fn.Parameters = append(fn.Parameters, param)
		case "scratch":
			attrs, moreDiags := block.Body.JustAttributes()
			diags = append(diags, moreDiags...)
			for name, attr := range attrs {
				if !hclsyntax.ValidIdentifier(name) {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid scratch value name",
						Detail:   badIdentifierDetail,
						Subject:  &attr.NameRange,
					})
				}

				if _, ok := fn.Scratch[name]; ok {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Duplicate scratch entry",
						Detail:   fmt.Sprintf("The function block already has a scratch entry named %s.", name),
						Subject:  &block.DefRange,
					})
					continue
				}
				fn.Scratch[name] = &FunctionScratch{
					Name:      name,
					Expr:      attr.Expr,
					DeclRange: attr.Range,
				}
			}
		case "return":
			if !fn.Return.DeclRange.Empty() {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate return block",
					Detail:   fmt.Sprintf("The function block already has a return block at %s.", fn.Return.DeclRange),
					Subject:  &block.DefRange,
				})
				continue
			}
			content, moreDiags := block.Body.Content(functionReturnBlockSchema)
			diags = append(diags, moreDiags...)

			fn.Return.Expr = content.Attributes["value"].Expr
			fn.Return.DeclRange = block.DefRange

			if attr, ok := content.Attributes["sensitive"]; ok {
				decodeDiags := gohcl.DecodeExpression(attr.Expr, nil, &fn.Return.Sensitive)
				diags = append(diags, decodeDiags...)
			}
		}
	}

	if fn.Return.DeclRange.Empty() {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Missing return block",
			Detail:   "The function block is missing a return block",
			Subject:  &block.DefRange,
		})
	}

	// TODO sort params by decl order

	return fn, diags
}

func (fn *Function) Implementation(parentCtx *hcl.EvalContext) (function.Function, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	spec := &function.Spec{
		Description: fn.Description,
		Type:        function.StaticReturnType(cty.DynamicPseudoType), // I don't know if we care to try to make this smarter
	}

	for i, param := range fn.Parameters {
		entry := function.Parameter{
			Name: param.Name,
			//Description: param.Description,
			Type:             param.Type,
			AllowNull:        param.Nullable,
			AllowUnknown:     true,
			AllowDynamicType: true,
			AllowMarked:      true,
		}

		if param.Variadic {
			if i != len(fn.Parameters)-1 {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid parameter",
					Detail:   "Variadic parameters must be the final parameter defined in a function",
					Subject:  &fn.DeclRange,
				})
				continue
			}
			spec.VarParam = &entry
		} else {

			spec.Params = append(spec.Params, entry)
		}
	}

	spec.Impl = func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		hclCtx := parentCtx.NewChild()
		hclCtx.Variables = map[string]cty.Value{}

		paramMap := map[string]cty.Value{}
		for i, param := range spec.Params {
			paramMap[param.Name] = args[i]
		}
		if spec.VarParam != nil {
			// TODO check indexes
			paramMap[spec.VarParam.Name] = cty.TupleVal(args[len(spec.Params):])
		}
		hclCtx.Variables["param"] = cty.ObjectVal(paramMap)

		// Mini Graph Time!
		var stack []string
		scratch := map[string]cty.Value{}

		var addScratch func(*FunctionScratch)
		addScratch = func(entry *FunctionScratch) {
			if _, ok := scratch[entry.Name]; ok {
				// Already added
				return
			}
			if slices.Contains(stack, entry.Name) {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Circular scratch dependency",
					Detail:   strings.Join(append(stack, entry.Name), ", "),
					Subject:  &entry.DeclRange,
				})
				scratch[entry.Name] = cty.NilVal
				return
			}
			// Push
			stack = append(stack, entry.Name)
			// Pop
			defer func() {
				stack = stack[:len(stack)-1]
			}()

			for _, v := range entry.Expr.Variables() {
				if v.RootName() == "scratch" {
					if len(v) < 1 {
						panic("booo")
					}
					attr, ok := v[1].(hcl.TraverseAttr)
					if !ok {
						// Handle error elsewhere
						continue
					}
					dep, ok := fn.Scratch[attr.Name]
					if !ok {
						// Handle error elsewhere
						continue
					}
					addScratch(dep)
				}
			}

			val, valDiags := entry.Expr.Value(hclCtx)
			diags = append(diags, valDiags...)

			scratch[entry.Name] = val

			hclCtx.Variables["scratch"] = cty.ObjectVal(scratch)
		}

		for _, scratch := range fn.Scratch {
			addScratch(scratch)
		}

		// TODO preconditions
		// TODO provider functions?
		val, valDiags := fn.Return.Expr.Value(hclCtx)
		diags = append(diags, valDiags...)
		if fn.Return.Sensitive {
			val = val.Mark(marks.Sensitive)
		}
		if diags.HasErrors() {
			return val, diags
		}
		return val, nil
	}

	return function.New(spec), diags
}

var functionBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "description"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "parameter", LabelNames: []string{"name"}},
		{Type: "scratch"},
		{Type: "return"},
	},
}
var functionParameterBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name: "type",
		},
		{
			Name: "sensitive",
		},
		{
			Name: "ephemeral",
		},
		{
			Name: "nullable",
		},
		{
			Name: "variadic",
		},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type: "validation",
		},
	},
}
var functionReturnBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name:     "value",
			Required: true,
		},
		{
			Name: "sensitive",
		},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "precondition"},
	},
}
