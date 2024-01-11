package configs

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/gocty"
)

// TODO len(diags) -> diags.HasErrors()

type StaticReference struct {
	Name  string
	Range hcl.Range

	// Either we have a computed Value, Reference to a dynamic value, or an Error
	Value     *cty.Value
	Reference *StaticReference
	Error     string
}

func (ref StaticReference) StaticValue() (*cty.Value, hcl.Diagnostics) {
	if len(ref.Error) != 0 {
		// Direct error
		return nil, hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to use dynamic value in static context",
			Detail:   fmt.Sprintf("%s: %s", ref.Name, ref.Error),
			Subject:  ref.Range.Ptr(),
		}}
	}
	if ref.Reference != nil {
		// Referenced error
		_, diags := ref.Reference.StaticValue()
		return nil, append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to reference dynamic value in static context",
			Detail:   fmt.Sprintf("%s attempted to use %s in static context", ref.Name, ref.Reference.Name),
			Subject:  ref.Range.Ptr(),
		})
	}
	if ref.Value != nil {
		return ref.Value, nil
	}

	// This should not be possible
	panic(fmt.Sprintf("Invalid StaticReference %s : %s", ref.Name, ref.Range.String()))
}

type StaticReferences map[string]StaticReference

func (r StaticReferences) ToCty() cty.Value {
	values := make(map[string]cty.Value)
	for name, ref := range r {
		if ref.Value != nil {
			values[name] = *ref.Value
		}
	}
	return cty.ObjectVal(values)
}

type VariableValueFunc func(VariableParsingMode) (cty.Value, hcl.Diagnostics)
type RawVariables map[string]VariableValueFunc

type StaticModuleCall struct {
	// Absolute Module Name
	Name string
	Raw  RawVariables     // CLI
	Call StaticReferences // Module Call
}

type StaticContext struct {
	// What should be resolved
	Variables map[string]*Variable
	Locals    map[string]*Local

	// Parameters
	Params StaticModuleCall

	// Current Evaluation Context
	EvalContext *hcl.EvalContext
	vars        StaticReferences
	locals      StaticReferences
}

func CreateStaticContext(vars map[string]*Variable, locals map[string]*Local, Params StaticModuleCall) (*StaticContext, hcl.Diagnostics) {
	ctx := StaticContext{
		Variables: vars,
		Locals:    locals,
		Params:    Params,
		EvalContext: &hcl.EvalContext{
			Variables: map[string]cty.Value{},
			Functions: (&lang.Scope{PureOnly: true}).Functions(),
		},
		vars:   make(StaticReferences),
		locals: make(StaticReferences),
	}

	ctx.EvalContext.Variables["terraform"] = cty.ObjectVal(map[string]cty.Value{
		"workspace": cty.StringVal("TODO"),
	})

	// Process all variables
	for _, v := range ctx.Variables {
		_, diags := ctx.addVariable(v)
		if len(diags) != 0 {
			// Could not construct reference
			return nil, diags
		}
	}

	// Process all locals
	for _, l := range ctx.Locals {
		_, diags := ctx.addLocal(l, make([]string, 0))
		if len(diags) != 0 {
			// Could not construct reference
			return nil, diags
		}
	}

	return &ctx, nil
}

func (s *StaticContext) resolveVariable(variable *Variable) (StaticReference, hcl.Diagnostics) {
	ref := StaticReference{
		Name:  s.Params.Name + ".var." + variable.Name,
		Range: variable.DeclRange,
	}

	// This is a raw value passed in via the command line.
	// Currently not EvalContextuated with any context.
	if v, ok := s.Params.Raw[variable.Name]; ok {
		val, diags := v(variable.ParsingMode)
		if len(diags) == 0 {
			ref.Value = &val
			return ref, nil
		}
		ref.Error = diags.Error()
		return ref, diags
	}

	// This is a module call parameter and may or may not be a resolved reference
	if call, ok := s.Params.Call[variable.Name]; ok {
		// Pass through Value
		ref.Value = call.Value
		ref.Reference = &call
		return ref, nil
	}

	if variable.Default.IsNull() {
		ref.Error = "Variable not set with null default"
		return ref, nil
	}

	ref.Value = &variable.Default
	return ref, nil
}

func (s *StaticContext) addVariable(variable *Variable) (StaticReference, hcl.Diagnostics) {
	ref, diags := s.resolveVariable(variable)

	// TODO type contraints
	// TODO validations?

	// Put var into context
	s.vars[variable.Name] = ref
	s.EvalContext.Variables["var"] = s.vars.ToCty()

	return ref, diags
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

func (s *StaticContext) checkTraversal(ident hcl.Traversal, ref StaticReference, locals []string) (StaticReference, hcl.Diagnostics) {
	root, attr, diags := traversalToIdentifier(ident)
	if len(diags) != 0 {
		ref.Error = diags.Error()
		ref.Range = ident.SourceRange()
		return ref, diags
	}

	switch root {
	case "var":
		// All variables should be known at this point.  This could change if we make variable defaults an expression
		variable, ok := s.vars[attr]
		if !ok {
			ref.Error = fmt.Sprintf("Undefined variable %s.%s used in %s", root, attr, ref.Name)
			ref.Range = ident.SourceRange()
			return ref, hcl.Diagnostics{&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Undefined variable",
				Detail:   ref.Error,
				Subject:  &ref.Range,
			}}
		} else if variable.Value == nil {
			// Not Static
			ref.Reference = &variable
			ref.Range = ident.SourceRange()
			return ref, nil
		}
	case "local":
		// First check if we have already processed this local
		local, ok := s.locals[attr]
		if !ok {
			// If not, let's try to load this local
			loadLocal, exists := s.Locals[attr]
			if !exists {
				ref.Error = fmt.Sprintf("Undefined local %s.%s used in %s", root, attr, ref.Name)
				ref.Range = ident.SourceRange()
				return ref, hcl.Diagnostics{&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Undefined local",
					Detail:   ref.Error,
					Subject:  &ref.Range,
				}}
			}

			// Make sure we have not tried to circularly reference a local
			for _, l := range locals {
				if l == loadLocal.Name {
					ref.Error = fmt.Sprintf("Circular reference when attempting to load local %s -> %s", strings.Join(locals, " -> "), loadLocal.Name)
					ref.Range = ident.SourceRange()
					return ref, hcl.Diagnostics{&hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Circular local reference",
						Detail:   ref.Error,
						Subject:  &ref.Range,
					}}
				}
			}

			var diags hcl.Diagnostics
			local, diags = s.addLocal(loadLocal, locals)
			if len(diags) != 0 {
				// Unable to load inner local
				ref.Reference = &local
				ref.Range = ident.SourceRange()
				return ref, diags
			}
		}
		// We now have a valid ref, though it may not be available for use in a static context
		if local.Value == nil {
			// Not static
			ref.Reference = &local
			ref.Range = ident.SourceRange()
			return ref, nil
		}
	case "terraform":
		// Static, rely on the EvalContext below.
	default:
		// not supported
		ref.Error = fmt.Sprintf("Unable to use %s.%s in static context", root, attr)
		ref.Range = ident.SourceRange()
		return ref, nil
	}
	return ref, nil
}

func (s *StaticContext) evaluate(expr hcl.Expression, ref StaticReference, locals []string) (StaticReference, hcl.Diagnostics) {
	// Determine dependencies, fail on first problem area and set the references source to the ident location
	for _, ident := range expr.Variables() {
		ref, diags := s.checkTraversal(ident, ref, locals)
		if len(diags) != 0 || ref.Reference != nil || len(ref.Error) != 0 {
			// Something is wrong with this reference, can't evaluate
			return ref, diags
		}
	}

	// If we have reached this point, all references *should* be valid.
	val, diags := expr.Value(s.EvalContext)
	if len(diags) != 0 {
		// Something broke, hopefully this is just a bad function reference
		ref.Error = diags.Error()
		return ref, diags
	}
	ref.Value = &val
	return ref, nil
}

func (s *StaticContext) resolveLocal(local *Local, locals []string) (StaticReference, hcl.Diagnostics) {
	ref := StaticReference{
		Name:  s.Params.Name + ".local." + local.Name,
		Range: local.DeclRange,
	}

	return s.evaluate(local.Expr, ref, locals)
}

func (s *StaticContext) addLocal(local *Local, locals []string) (StaticReference, hcl.Diagnostics) {
	ref, diags := s.resolveLocal(local, append(locals, local.Name))

	if len(diags) == 0 {
		// Update local map
		s.locals[local.Name] = ref
		if ref.Value != nil {
			// Put local map into current context
			s.EvalContext.Variables["local"] = s.locals.ToCty()
		}
	}

	return ref, diags
}

func (s StaticContext) Evaluate(expr hcl.Expression, fullName string) (StaticReference, hcl.Diagnostics) {
	ref := StaticReference{
		Name:  fullName,
		Range: expr.Range(),
	}

	return s.evaluate(expr, ref, make([]string, 0))
}

// This is heavily inspired by gohcl.DecodeExpression
func (s StaticContext) DecodeExpression(expr hcl.Expression, fullName string, val any) hcl.Diagnostics {
	ref, refDiags := s.Evaluate(expr, fullName)
	if refDiags.HasErrors() {
		return refDiags
	}

	refVal, valDiags := ref.StaticValue()
	if valDiags.HasErrors() {
		return valDiags
	}
	srcVal := *refVal
	// TODO make sure refVal IsKnown().  Some impure functions can break that

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
	var diags hcl.Diagnostics

	ref := StaticReference{
		Name: path,
	}
	for _, ident := range hcldec.Variables(body, spec) {
		ref, refDiags := s.checkTraversal(ident, ref, nil)
		diags = append(diags, refDiags...)
		if !refDiags.HasErrors() {
			_, refDiags = ref.StaticValue()
			diags = append(diags, refDiags...)
		}
	}

	val, valDiags := hcldec.Decode(body, spec, s.EvalContext)
	if !diags.HasErrors() {
		// We rely on the Decode for generating a valid return cty.Value, even if references are not
		// satisfiable.  We only care about the valDiags if we think all of the references are valid.
		// Otherwise, we would get junk in the diags about variables that don't exist.
		diags = append(diags, valDiags...)
	}

	return val, diags
}
