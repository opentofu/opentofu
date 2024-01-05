package configs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

type StaticMissing struct {
	// Info
	Name string
	Decl hcl.Range

	Reason    *string
	Reference *StaticReference
}

func (m StaticMissing) Chain() []StaticMissing {
	result := []StaticMissing{m}
	if m.Reference != nil {
		result = append(result, m.Reference.Missing.Chain()...)
	}
	return result
}

type StaticReference struct {
	Value   *cty.Value
	Missing *StaticMissing
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

type StaticParams struct {
	// Absolute Module Name
	Name string
	Raw  map[string]string // CLI
	Call StaticReferences  // Module Call
}

type StaticContext struct {
	// What should be resolved
	Variables map[string]*Variable
	Locals    map[string]*Local

	// Parameters
	Params StaticParams

	// Current Evaluation Context
	EvalContext *hcl.EvalContext
	vars        StaticReferences
	locals      StaticReferences
}

func CreateStaticContext(vars map[string]*Variable, locals map[string]*Local, Params StaticParams) (*StaticContext, hcl.Diagnostics) {
	ctx := StaticContext{
		Variables: vars,
		Locals:    locals,
		Params:    Params,
		EvalContext: &hcl.EvalContext{
			Variables: map[string]cty.Value{},
			// TODO functions
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
		_, diags := ctx.addLocal(l)
		if len(diags) != 0 {
			// Could not construct reference
			return nil, diags
		}
	}

	return &ctx, nil
}

func (s *StaticContext) resolveVariable(variable *Variable) (StaticReference, hcl.Diagnostics) {
	// This is a raw value passed in via the command line.
	// Currently not EvalContextuated with any context.
	if v, ok := s.Params.Raw[variable.Name]; ok {
		val, diags := variable.ParsingMode.Parse(variable.Name, v)
		if len(diags) == 0 {
			return StaticReference{Value: &val}, nil
		}
		err := diags.Error()
		return StaticReference{Missing: &StaticMissing{Reason: &err}}, diags
	}

	// This is a module call parameter and may or may not be a resolved reference
	if ref, ok := s.Params.Call[variable.Name]; ok {
		if ref.Value != nil {
			// Resolved, pass through
			return ref, nil
		} else {
			return StaticReference{Missing: &StaticMissing{Name: s.Params.Name + ".var." + variable.Name, Decl: variable.DeclRange, Reference: &ref}}, nil
		}
	}

	return StaticReference{Value: &variable.Default}, nil
}

func (s *StaticContext) addVariable(variable *Variable) (StaticReference, hcl.Diagnostics) {
	ref, diags := s.resolveVariable(variable)

	// TODO type contraints
	// TODO validations?

	// Put var into context
	if len(diags) == 0 {
		s.vars[variable.Name] = ref
		if ref.Value != nil {
			s.EvalContext.Variables["var"] = s.vars.ToCty()
		}
	}

	return ref, diags
}

func (s *StaticContext) resolveLocal(local *Local) (StaticReference, hcl.Diagnostics) {
	fullName := s.Params.Name + ".local." + local.Name

	// Determine dependencies, fail on first problem area
	for _, ident := range local.Expr.Variables() {
		// Everything *should* be root.attr
		root := ident[0].(hcl.TraverseRoot)
		attr := ident[1].(hcl.TraverseAttr)

		switch root.Name {
		case "var":
			// All variables should be known at this point.  This could change if we make variable defaults an expression
			ref, ok := s.vars[attr.Name]
			if !ok {
				// Undefined
				diags := hcl.Diagnostics{&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Undefined variable",
					Detail:   fmt.Sprintf("Undefined variable %s.%s used in %s", root.Name, attr.Name, fullName),
					Subject:  &local.DeclRange,
				}}
				return StaticReference{}, diags
			} else if ref.Value == nil {
				// Not Static
				return StaticReference{Missing: &StaticMissing{Name: fullName, Decl: local.DeclRange, Reference: &ref}}, nil
			}
		case "local":
			// TODO We will need to prevent this from recursing infinitely

			// First check if we have already processed this local
			ref, ok := s.locals[attr.Name]
			if !ok {
				// If not, let's try to load this local
				modLocal, exists := s.Locals[attr.Name]
				if !exists {
					// Undefined
					diags := hcl.Diagnostics{&hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Undefined local",
						Detail:   fmt.Sprintf("Undefined local %s.%s used in %s", root.Name, attr.Name, fullName),
						Subject:  &local.DeclRange,
					}}
					return StaticReference{}, diags
				}
				var diags hcl.Diagnostics
				ref, diags = s.addLocal(modLocal)
				if len(diags) != 0 {
					// Passthrough
					return ref, diags
				}
			}
			// We now have valid ref, though it may not be available for use in a static context
			if ref.Value == nil {
				// Not static
				return StaticReference{Missing: &StaticMissing{Name: fullName, Decl: local.DeclRange, Reference: &ref}}, nil
			}
		case "terraform":
			// Static, rely on the EvalContext below.
		default:
			// not supported
			reason := fmt.Sprintf("Unable to use %s.%s in static context", root.Name, attr.Name) //TODO this is a bad error message
			return StaticReference{Missing: &StaticMissing{Name: fullName, Decl: local.DeclRange, Reason: &reason}}, nil
		}
	}

	// If we have reached this point, all references *should* be valid.
	val, diags := local.Expr.Value(s.EvalContext)
	if len(diags) != 0 {
		// Something broke, hopefully this is just a bad function reference
		return StaticReference{}, diags
	}
	return StaticReference{Value: &val}, nil
}

func (s *StaticContext) addLocal(local *Local) (StaticReference, hcl.Diagnostics) {
	ref, diags := s.resolveLocal(local)

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
	// FIXME This is copy-pasted from locals above and slightly modified

	// Determine dependencies, fail on first problem area
	for _, ident := range expr.Variables() {

		// Everything *should* be root.attr
		root := ident[0].(hcl.TraverseRoot)
		attr := ident[1].(hcl.TraverseAttr)
		switch root.Name {
		case "var":
			// All variables should be known at this point.
			ref, ok := s.vars[attr.Name]
			if !ok {
				// Undefined
				diags := hcl.Diagnostics{&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Undefined variable",
					Detail:   fmt.Sprintf("Undefined variable %s.%s used in %s", root.Name, attr.Name, fullName),
					Subject:  expr.Range().Ptr(),
				}}
				return StaticReference{}, diags
			} else if ref.Value == nil {
				// Not Static
				return StaticReference{Missing: &StaticMissing{Name: fullName, Decl: expr.Range(), Reference: &ref}}, nil
			}
		case "local":
			// All locals should be known at this point.
			ref, ok := s.locals[attr.Name]
			if !ok {
				// Undefined
				diags := hcl.Diagnostics{&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Undefined local",
					Detail:   fmt.Sprintf("Undefined local %s.%s used in %s", root.Name, attr.Name, fullName),
					Subject:  expr.Range().Ptr(),
				}}
				return StaticReference{}, diags
			} else if ref.Value == nil {
				// Not static
				return StaticReference{Missing: &StaticMissing{Name: fullName, Decl: expr.Range(), Reference: &ref}}, nil
			}
		case "terraform":
			// Static, rely on the EvalContext below.
		default:
			// not supported
			reason := fmt.Sprintf("Unable to use %s.%s in static context", root.Name, attr.Name) //TODO this is a bad error message
			return StaticReference{Missing: &StaticMissing{Name: fullName, Decl: expr.Range(), Reason: &reason}}, nil
		}
	}

	// If we have reached this point, all references *should* be valid.
	val, diags := expr.Value(s.EvalContext)
	if len(diags) != 0 {
		// Something broke, hopefully this is just a bad function reference
		return StaticReference{}, diags
	}
	return StaticReference{Value: &val}, nil
}
