package configs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/gocty"
)

// TODO len(diags) -> diags.HasErrors()

type StaticValue func() (cty.Value, hcl.Diagnostics)

// Consider splitting StaticReference and StaticIdentifier

type StaticReference struct {
	Module string
	Type   string
	Name   string
	Range  hcl.Range // Not used?
	Value  StaticValue
}

func (ref StaticReference) DisplayString() string {
	val := ref.Name
	if len(ref.Type) != 0 {
		val = ref.Type + "." + val
	}
	if len(ref.Module) != 0 {
		val = ref.Module + "." + val
	}
	return val
}

func (ref StaticReference) Cached() StaticReference {
	var val cty.Value
	var diags hcl.Diagnostics
	cached := false

	return StaticReference{
		Module: ref.Module,
		Type:   ref.Type,
		Name:   ref.Name,
		Range:  ref.Range,
		Value: func() (cty.Value, hcl.Diagnostics) {
			if !cached {
				val, diags = ref.Value()
				cached = true
			}
			return val, diags
		},
	}
}

type StaticReferences map[string]StaticReference

type VariableValueFunc func(VariableParsingMode) (cty.Value, hcl.Diagnostics)
type RawVariables map[string]VariableValueFunc

type StaticModuleCall struct {
	// Absolute Module Name
	Name string
	Raw  RawVariables     // CLI
	Call StaticReferences // Module Call
}

type StaticContext struct {
	// Parameters
	Params StaticModuleCall

	// Cache
	vars      StaticReferences
	locals    StaticReferences
	workspace StaticReference
}

func CreateStaticContext(vars map[string]*Variable, locals map[string]*Local, Params StaticModuleCall) (*StaticContext, hcl.Diagnostics) {
	ctx := StaticContext{
		Params: Params,
		vars:   make(StaticReferences),
		locals: make(StaticReferences),
		workspace: StaticReference{
			Name: "terraform.workspace",
			Value: func() (cty.Value, hcl.Diagnostics) {
				return cty.StringVal("TODO"), nil
			},
		},
	}

	// Process all variables
	for _, v := range vars {
		ctx.addVariable(v)
	}

	// Process all locals
	for _, l := range locals {
		ctx.addLocal(l)
	}

	return &ctx, nil
}

func (s *StaticContext) addVariable(variable *Variable) {
	s.vars[variable.Name] = StaticReference{
		Module: s.Params.Name,
		Type:   "var",
		Name:   variable.Name,
		Range:  variable.DeclRange,
		Value: func() (cty.Value, hcl.Diagnostics) {
			// This is a raw value passed in via the command line.
			// Currently not EvalContextuated with any context.
			if v, ok := s.Params.Raw[variable.Name]; ok {
				return v(variable.ParsingMode)
			}

			// This is a module call parameter and may or may not be a resolved reference
			if call, ok := s.Params.Call[variable.Name]; ok {
				// We might want to enhance the diags here if diags.HasErrors()
				return call.Value()
			}

			// TODO use type checking to figure this out
			if variable.Default.IsNull() {
				return variable.Default, hcl.Diagnostics{&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Variable not set with null default",
					Subject:  variable.DeclRange.Ptr(),
				}}
			}

			// Not specified, use default instead
			return variable.Default, nil
		},
	}.Cached()
}

func traversalToIdentifier(ident hcl.Traversal) (string, string, hcl.Diagnostics) {
	// Everything *should* be root.attr
	root, rootOk := ident[0].(hcl.TraverseRoot)
	attr, attrOk := ident[1].(hcl.TraverseAttr)
	if !rootOk || !attrOk {
		return "", "", hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference, expected x.y",
			Subject:  ident.SourceRange().Ptr(),
		}}
	}
	return root.Name, attr.Name, nil
}

func (s *StaticContext) followTraversal(ident hcl.Traversal) (*StaticReference, hcl.Diagnostics) {
	root, attr, diags := traversalToIdentifier(ident)
	if diags.HasErrors() {
		return nil, diags
	}

	switch root {
	case "var":
		variable, ok := s.vars[attr]
		if !ok {
			return nil, hcl.Diagnostics{&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Undefined variable",
				Detail:   fmt.Sprintf("Undefined variable %s.%s", root, attr),
				Subject:  ident.SourceRange().Ptr(),
			}}
		}
		return &variable, nil
	case "local":
		local, ok := s.locals[attr]
		if !ok {
			return nil, hcl.Diagnostics{&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Undefined local",
				Detail:   fmt.Sprintf("Undefined local %s.%s", root, attr),
				Subject:  ident.SourceRange().Ptr(),
			}}
		}
		return &local, nil
	case "terraform":
		return &s.workspace, nil
	default:
		return nil, hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Dynamic value in static context",
			Detail:   fmt.Sprintf("Unable to use %s.%s in static context", root, attr),
			Subject:  ident.SourceRange().Ptr(),
		}}
	}
}

func (s *StaticContext) buildEvaluationContext(deps []hcl.Traversal, source StaticReference) (*hcl.EvalContext, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	variables := make(map[string]map[string]cty.Value)

	for _, ident := range deps {
		ref, rDiags := s.followTraversal(ident)
		diags = append(diags, rDiags...)

		if rDiags.HasErrors() {
			// Something critical happened, invalid reference
			continue
		}

		if _, ok := variables[ref.Type]; !ok {
			variables[ref.Type] = make(map[string]cty.Value)
		}

		val, vDiags := ref.Value()
		diags = append(diags, vDiags...)
		if vDiags.HasErrors() {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unable to compute static value",
				Detail:   fmt.Sprintf("%s depends on %s which is not available", source.DisplayString(), ref.DisplayString()),
				Subject:  ident.SourceRange().Ptr(),
			})
		}

		variables[ref.Type][ref.Name] = val
	}

	if diags.HasErrors() {
		return nil, diags
	}

	ctx := &hcl.EvalContext{
		Variables: map[string]cty.Value{},
		Functions: (&lang.Scope{PureOnly: true}).Functions(),
	}

	for ty, o := range variables {
		ctx.Variables[ty] = cty.ObjectVal(o)
	}

	return ctx, nil
}

func (s *StaticContext) addLocal(local *Local) {
	ref := StaticReference{
		Module: s.Params.Name,
		Type:   "local",
		Name:   local.Name,
		Range:  local.DeclRange,
	}
	ref.Value = func() (cty.Value, hcl.Diagnostics) {
		ctx, diags := s.buildEvaluationContext(local.Expr.Variables(), ref)
		if diags.HasErrors() {
			return cty.NilVal, diags
		}
		return local.Expr.Value(ctx)
	}

	s.locals[local.Name] = ref.Cached()
}

func (s StaticContext) Evaluate(expr hcl.Expression, fullName string) StaticReference {
	ref := StaticReference{
		Name:  fullName, // TODO split up name
		Range: expr.Range(),
	}
	ref.Value = func() (cty.Value, hcl.Diagnostics) {
		ctx, diags := s.buildEvaluationContext(expr.Variables(), ref)
		if diags.HasErrors() {
			return cty.NilVal, diags
		}
		return expr.Value(ctx)
	}

	return ref.Cached()
}

// This is heavily inspired by gohcl.DecodeExpression
func (s StaticContext) DecodeExpression(expr hcl.Expression, fullName string, val any) hcl.Diagnostics {
	srcVal, valDiags := s.Evaluate(expr, fullName).Value()
	if valDiags.HasErrors() {
		return valDiags
	}

	convTy, err := gocty.ImpliedType(val)
	if err != nil {
		panic(fmt.Sprintf("unsuitable DecodeExpression target: %s", err))
	}

	srcVal, err = convert.Convert(srcVal, convTy)
	if err != nil {
		return hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unsuitable value type",
			Detail:   fmt.Sprintf("Unsuitable value: %s", err.Error()),
			Subject:  expr.StartRange().Ptr(),
			Context:  expr.Range().Ptr(),
		}}
	}

	err = gocty.FromCtyValue(srcVal, val)
	if err != nil {
		return hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unsuitable value type",
			Detail:   fmt.Sprintf("Unsuitable value: %s", err.Error()),
			Subject:  expr.StartRange().Ptr(),
			Context:  expr.Range().Ptr(),
		}}
	}

	return nil
}

func (s StaticContext) DecodeBlock(body hcl.Body, spec hcldec.Spec, path string) (cty.Value, hcl.Diagnostics) {
	ctx, diags := s.buildEvaluationContext(hcldec.Variables(body, spec), StaticReference{Name: path})

	val, valDiags := hcldec.Decode(body, spec, ctx)
	if !diags.HasErrors() {
		// We rely on the Decode for generating a valid return cty.Value, even if references are not
		// satisfiable.  We only care about the valDiags if we think all of the references are valid.
		// Otherwise, we would get junk in the diags about variables that don't exist.
		diags = append(diags, valDiags...)
	}

	return val, diags
}
