package configs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/zclconf/go-cty/cty"
)

type StaticIdentifier struct {
	Module string
	Type   string
	Name   string
}

func (ref StaticIdentifier) String() string {
	val := ref.Name
	if len(ref.Type) != 0 {
		val = ref.Type + "." + val
	}
	if len(ref.Module) != 0 {
		val = ref.Module + "." + val
	}
	return val
}

type StaticReference struct {
	Identifier StaticIdentifier
	Value      func(stack []StaticIdentifier) (cty.Value, hcl.Diagnostics)
}

func (ref StaticReference) Cached() StaticReference {
	var val cty.Value
	var diags hcl.Diagnostics
	cached := false

	return StaticReference{
		Identifier: ref.Identifier,
		Value: func(stack []StaticIdentifier) (cty.Value, hcl.Diagnostics) {
			if !cached {
				val, diags = ref.Value(stack)
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
			Identifier: StaticIdentifier{
				Type: "terraform",
				Name: "workspace",
			},
			Value: func(stack []StaticIdentifier) (cty.Value, hcl.Diagnostics) {
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
		Identifier: StaticIdentifier{
			Module: s.Params.Name,
			Type:   "var",
			Name:   variable.Name,
		},
		Value: func(stack []StaticIdentifier) (cty.Value, hcl.Diagnostics) {
			// This is a raw value passed in via the command line.
			// Currently not EvalContextuated with any context.
			if v, ok := s.Params.Raw[variable.Name]; ok {
				return v(variable.ParsingMode)
			}

			// This is a module call parameter and may or may not be a resolved reference
			if call, ok := s.Params.Call[variable.Name]; ok {
				// We might want to enhance the diags here if diags.HasErrors()
				return call.Value(nil)
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

func traversalToIdentifier(ident hcl.Traversal) (StaticIdentifier, hcl.Diagnostics) {
	// Everything *should* be root.attr
	root, rootOk := ident[0].(hcl.TraverseRoot)
	attr, attrOk := ident[1].(hcl.TraverseAttr)
	if !rootOk || !attrOk {
		return StaticIdentifier{}, hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference, expected x.y",
			Subject:  ident.SourceRange().Ptr(),
		}}
	}
	return StaticIdentifier{Type: root.Name, Name: attr.Name}, nil
}

func (s *StaticContext) followTraversal(trav hcl.Traversal) (*StaticReference, hcl.Diagnostics) {
	ident, diags := traversalToIdentifier(trav)
	if diags.HasErrors() {
		return nil, diags
	}

	switch ident.Type {
	case "var":
		variable, ok := s.vars[ident.Name]
		if !ok {
			return nil, hcl.Diagnostics{&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Undefined variable",
				Detail:   fmt.Sprintf("Undefined variable %s", ident.String()),
				Subject:  trav.SourceRange().Ptr(),
			}}
		}
		return &variable, nil
	case "local":
		local, ok := s.locals[ident.Name]
		if !ok {
			return nil, hcl.Diagnostics{&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Undefined local",
				Detail:   fmt.Sprintf("Undefined local %s", ident.String()),
				Subject:  trav.SourceRange().Ptr(),
			}}
		}
		return &local, nil
	case "terraform":
		return &s.workspace, nil
	default:
		return nil, hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Dynamic value in static context",
			Detail:   fmt.Sprintf("Unable to use %s in static context", ident.String()),
			Subject:  trav.SourceRange().Ptr(),
		}}
	}
}

func (s *StaticContext) buildEvaluationContext(deps []hcl.Traversal, source StaticIdentifier, stack []StaticIdentifier) (*hcl.EvalContext, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	variables := make(map[string]map[string]cty.Value)

	for _, ident := range deps {
		ref, rDiags := s.followTraversal(ident)
		diags = append(diags, rDiags...)

		if rDiags.HasErrors() {
			// Something critical happened, invalid reference
			continue
		}

		if _, ok := variables[ref.Identifier.Type]; !ok {
			variables[ref.Identifier.Type] = make(map[string]cty.Value)
		}

		circular := false
		for _, frame := range stack {
			if frame.String() == source.String() {
				circular = true
				break
			}
		}
		if circular {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Circular reference",
				Detail:   fmt.Sprintf("%s is self referential", ref.Identifier.String()), // TODO use stack in error message
				Subject:  ident.SourceRange().Ptr(),
			})
			continue
		}

		val, vDiags := ref.Value(append(stack, ref.Identifier))
		diags = append(diags, vDiags...)
		if vDiags.HasErrors() {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unable to compute static value",
				Detail:   fmt.Sprintf("%s depends on %s which is not available", source.String(), ref.Identifier.String()),
				Subject:  ident.SourceRange().Ptr(),
			})
		}

		variables[ref.Identifier.Type][ref.Identifier.Name] = val
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
	ident := StaticIdentifier{
		Module: s.Params.Name,
		Type:   "local",
		Name:   local.Name,
	}
	s.locals[local.Name] = StaticReference{
		Identifier: ident,
		Value: func(stack []StaticIdentifier) (cty.Value, hcl.Diagnostics) {
			ctx, diags := s.buildEvaluationContext(local.Expr.Variables(), ident, stack)
			if diags.HasErrors() {
				return cty.NilVal, diags
			}
			return local.Expr.Value(ctx)
		},
	}.Cached()
}

func (s StaticContext) Evaluate(expr hcl.Expression, ident StaticIdentifier) StaticReference {
	return StaticReference{
		Identifier: ident,
		Value: func(stack []StaticIdentifier) (cty.Value, hcl.Diagnostics) {
			ctx, diags := s.buildEvaluationContext(expr.Variables(), ident, stack)
			if diags.HasErrors() {
				return cty.NilVal, diags
			}
			return expr.Value(ctx)
		},
	}.Cached()
}

// This is heavily inspired by gohcl.DecodeExpression
func (s StaticContext) DecodeExpression(expr hcl.Expression, ident StaticIdentifier, val any) hcl.Diagnostics {
	ctx, diags := s.buildEvaluationContext(expr.Variables(), ident, nil)
	if diags.HasErrors() {
		return diags
	}

	return gohcl.DecodeExpression(expr, ctx, val)
}

func (s StaticContext) DecodeBlock(body hcl.Body, spec hcldec.Spec, ident StaticIdentifier) (cty.Value, hcl.Diagnostics) {
	ctx, diags := s.buildEvaluationContext(hcldec.Variables(body, spec), ident, nil)

	val, valDiags := hcldec.Decode(body, spec, ctx)
	if !diags.HasErrors() {
		// We rely on the Decode for generating a valid return cty.Value, even if references are not
		// satisfiable.  We only care about the valDiags if we think all of the references are valid.
		// Otherwise, we would get junk in the diags about variables that don't exist.
		diags = append(diags, valDiags...)
	}

	return val, diags
}
