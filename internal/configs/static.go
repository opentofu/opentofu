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
type StaticModuleVariables func(v *Variable) (cty.Value, hcl.Diagnostics)

type StaticModuleCall struct {
	Addr addrs.Module

	Variables StaticModuleVariables

	RootPath string
}

type StaticContext struct {
	Call StaticModuleCall

	vars   StaticReferences
	locals StaticReferences
}

func CreateStaticContext(mod *Module, call StaticModuleCall) (*StaticContext, hcl.Diagnostics) {
	ctx := StaticContext{
		Call:   call,
		vars:   make(StaticReferences),
		locals: make(StaticReferences),
	}

	// Process all variables
	for _, v := range mod.Variables {
		ctx.addVariable(v)
	}

	// Process all locals
	for _, l := range mod.Locals {
		ctx.addLocal(l)
	}

	return &ctx, nil
}

func (s *StaticContext) addVariable(variable *Variable) {
	s.vars[variable.Name] = StaticReference{
		Identifier: StaticIdentifier{
			Module:    s.Call.Addr,
			Subject:   addrs.InputVariable{Name: variable.Name},
			DeclRange: variable.DeclRange,
		},
		Value: func(stack []StaticIdentifier) (cty.Value, hcl.Diagnostics) {
			return s.Call.Variables(variable)
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
		case addrs.LocalValue:
			continue
		case addrs.InputVariable:
			continue
		case addrs.PathAttr:
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
		return cty.DynamicVal, diags
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
		return cty.DynamicVal, diags.Append(&hcl.Diagnostic{
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
	panic("TODO")
}
func (s ScopeData) GetTerraformAttr(ident addrs.TerraformAttr, rng tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
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
		Module:    s.Call.Addr,
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

func (s StaticContext) DecodeExpression(expr hcl.Expression, ident StaticIdentifier, val any) hcl.Diagnostics {
	var diags hcl.Diagnostics

	refs, refsDiags := lang.ReferencesInExpr(addrs.ParseRef, expr)
	diags = append(diags, refsDiags.ToHCL()...)
	if diags.HasErrors() {
		return diags
	}

	ctx, ctxDiags := s.scope(ident, nil).EvalContext(refs)
	diags = append(diags, ctxDiags.ToHCL()...)
	if diags.HasErrors() {
		return diags
	}

	return gohcl.DecodeExpression(expr, ctx, val)
}

func (s StaticContext) DecodeBlock(body hcl.Body, spec hcldec.Spec, ident StaticIdentifier) (cty.Value, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	refs, refsDiags := lang.References(addrs.ParseRef, hcldec.Variables(body, spec))
	diags = append(diags, refsDiags.ToHCL()...)
	if diags.HasErrors() {
		return cty.DynamicVal, diags
	}

	ctx, ctxDiags := s.scope(ident, nil).EvalContext(refs)
	diags = append(diags, ctxDiags.ToHCL()...)
	if diags.HasErrors() {
		return cty.DynamicVal, diags
	}

	val, valDiags := hcldec.Decode(body, spec, ctx)
	diags = append(diags, valDiags...)
	return val, diags
}
