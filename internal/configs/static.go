package configs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type StaticIdentifier struct {
	Module    addrs.Module
	Subject   addrs.Referenceable
	DeclRange hcl.Range
}

func (ref StaticIdentifier) String() string {
	val := ref.Subject.String()
	if len(ref.Module) != 0 {
		val = ref.Module.String() + "." + val
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
	// Absolute Module Addr
	Addr addrs.Module
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
				Subject: addrs.TerraformAttr{Name: "workspace"},
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
			Module:    s.Params.Addr,
			Subject:   addrs.InputVariable{Name: variable.Name},
			DeclRange: variable.DeclRange,
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

type ScopeData struct {
	sctx   *StaticContext
	source StaticIdentifier
	stack  []StaticIdentifier
}

func (s ScopeData) StaticValidateReferences(refs []*addrs.Reference, self addrs.Referenceable, source addrs.Referenceable) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	for _, ref := range refs {
		switch subject := ref.Subject.(type) {
		case addrs.TerraformAttr:
			continue
		case addrs.LocalValue:
			continue
		case addrs.InputVariable:
			continue
		default:
			diags = diags.Append(hcl.Diagnostics{&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Dynamic value in static context",
				Detail:   fmt.Sprintf("Unable to use %s in static context", subject.String()),
				Subject:  ref.SourceRange.ToHCL().Ptr(),
			}})
		}
	}
	return diags
}

func (s ScopeData) GetCountAttr(addrs.CountAttr, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}
func (s ScopeData) GetForEachAttr(addrs.ForEachAttr, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}
func (s ScopeData) GetResource(addrs.Resource, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}

func (s ScopeData) eval(ref StaticReference) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	circular := false
	for _, frame := range s.stack {
		if frame.String() == s.source.String() {
			circular = true
			break
		}
	}
	if circular {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Circular reference",
			Detail:   fmt.Sprintf("%s is self referential", ref.Identifier.String()), // TODO use stack in error message
			Subject:  ref.Identifier.DeclRange.Ptr(),
		})
		return cty.NilVal, diags
	}

	val, vDiags := ref.Value(append(s.stack, ref.Identifier))
	diags = diags.Append(vDiags)
	if vDiags.HasErrors() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to compute static value",
			Detail:   fmt.Sprintf("%s depends on %s which is not available", s.source.String(), ref.Identifier.String()),
			Subject:  ref.Identifier.DeclRange.Ptr(),
		})
	}
	return val, diags
}

func (s ScopeData) GetLocalValue(ident addrs.LocalValue, rng tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	local, ok := s.sctx.locals[ident.Name]
	if !ok {
		return cty.NilVal, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Undefined local",
			Detail:   fmt.Sprintf("Undefined local %s", ident.String()),
			Subject:  rng.ToHCL().Ptr(),
		})
	}
	return s.eval(local)
}
func (s ScopeData) GetModule(addrs.ModuleCall, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}
func (s ScopeData) GetPathAttr(addrs.PathAttr, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	// TODO
	panic("Not Available in Static Context")
}
func (s ScopeData) GetTerraformAttr(ident addrs.TerraformAttr, rng tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	if ident.Name != "workspace" {
		return cty.NilVal, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Undefined terraform attr",
			Detail:   fmt.Sprintf("Undefined %s", ident.String()),
			Subject:  rng.ToHCL().Ptr(),
		})
	}
	// TODO shortcut
	return s.eval(s.sctx.workspace)
}
func (s ScopeData) GetInputVariable(ident addrs.InputVariable, rng tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	variable, ok := s.sctx.vars[ident.Name]
	if !ok {
		return cty.NilVal, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Undefined variable",
			Detail:   fmt.Sprintf("Undefined variable %s", ident.String()),
			Subject:  rng.ToHCL().Ptr(),
		})
	}
	return s.eval(variable)
}
func (s ScopeData) GetOutput(addrs.OutputValue, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}
func (s ScopeData) GetCheckBlock(addrs.Check, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}

func (s *StaticContext) scope(ident StaticIdentifier, stack []StaticIdentifier) *lang.Scope {
	return &lang.Scope{
		Data:     ScopeData{s, ident, stack},
		ParseRef: addrs.ParseRef,
		//SourceAddr: ident.Subject?
		//TODO BaseDir string
		//TODO PureOnly bool
		//ConsoleMode bool
	}
}

func (s *StaticContext) addLocal(local *Local) {
	ident := StaticIdentifier{
		Module:    s.Params.Addr,
		Subject:   addrs.LocalValue{Name: local.Name},
		DeclRange: local.DeclRange,
	}
	s.locals[local.Name] = StaticReference{
		Identifier: ident,
		Value: func(stack []StaticIdentifier) (cty.Value, hcl.Diagnostics) {
			val, diags := s.scope(ident, stack).EvalExpr(local.Expr, cty.DynamicPseudoType)
			return val, diags.ToHCL()
		},
	}.Cached()
}

func (s StaticContext) Evaluate(expr hcl.Expression, ident StaticIdentifier) StaticReference {
	return StaticReference{
		Identifier: ident,
		Value: func(stack []StaticIdentifier) (cty.Value, hcl.Diagnostics) {
			val, diags := s.scope(ident, stack).EvalExpr(expr, cty.DynamicPseudoType)
			return val, diags.ToHCL()
		},
	}.Cached()
}

// This is heavily inspired by gohcl.DecodeExpression
func (s StaticContext) DecodeExpression(expr hcl.Expression, ident StaticIdentifier, val any) hcl.Diagnostics {
	refs, diags := lang.ReferencesInExpr(addrs.ParseRef, expr)
	if diags.HasErrors() {
		return diags.ToHCL()
	}

	// TODO overrides diag warnings
	ctx, diags := s.scope(ident, nil).EvalContext(refs)
	if diags.HasErrors() {
		return diags.ToHCL()
	}

	return gohcl.DecodeExpression(expr, ctx, val)
}

func (s StaticContext) DecodeBlock(body hcl.Body, spec hcldec.Spec, ident StaticIdentifier) (cty.Value, hcl.Diagnostics) {
	refs, rdiags := lang.References(addrs.ParseRef, hcldec.Variables(body, spec))
	if rdiags.HasErrors() {
		return cty.NilVal, rdiags.ToHCL()
	}

	// TODO overrides diag warnings
	ctx, cdiags := s.scope(ident, nil).EvalContext(refs)
	diags := cdiags.ToHCL()
	if cdiags.HasErrors() {
		return cty.NilVal, cdiags.ToHCL()
	}

	val, valDiags := hcldec.Decode(body, spec, ctx)
	if !diags.HasErrors() {
		// We rely on the Decode for generating a valid return cty.Value, even if references are not
		// satisfiable.  We only care about the valDiags if we think all of the references are valid.
		// Otherwise, we would get junk in the diags about variables that don't exist.
		diags = append(diags, valDiags...)
	}

	return val, diags
}
