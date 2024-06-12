package configs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/didyoumean"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// StaticIdentifier holds a Referencable item and where it was declared
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

// StaticValue is a lazily evaluated cty.Value used in a static context
type StaticValueFn func(stack []StaticIdentifier) (cty.Value, hcl.Diagnostics)

type StaticModuleVariables func(v *Variable) (cty.Value, hcl.Diagnostics)

// StaticModuleCall contains the information required to call a given module
type StaticModuleCall struct {
	Addr addrs.Module

	Variables StaticModuleVariables

	RootPath string
}

type StaticContext struct {
	Call StaticModuleCall
	cfg  *Module
}

// Creates a static context based from the given module and module call
func NewStaticContext(mod *Module, call StaticModuleCall) *StaticContext {
	return &StaticContext{
		Call: call,
		cfg:  mod,
	}
}

type staticScopeData struct {
	ctx    *StaticContext
	source StaticIdentifier
	stack  []StaticIdentifier
}

func (s staticScopeData) StaticValidateReferences(refs []*addrs.Reference, self addrs.Referenceable, source addrs.Referenceable) tfdiags.Diagnostics {
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

func (s staticScopeData) GetCountAttr(addrs.CountAttr, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}
func (s staticScopeData) GetForEachAttr(addrs.ForEachAttr, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}
func (s staticScopeData) GetResource(addrs.Resource, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}

func (s staticScopeData) eval(ident StaticIdentifier, ref StaticValueFn) (cty.Value, tfdiags.Diagnostics) {
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
			Detail:   fmt.Sprintf("%s is self referential", ident.String()), // TODO use stack in error message
			Subject:  ident.DeclRange.Ptr(),
		})
		return cty.DynamicVal, diags
	}

	val, vDiags := ref(append(s.stack, ident))
	diags = diags.Append(vDiags)
	if vDiags.HasErrors() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to compute static value",
			Detail:   fmt.Sprintf("%s depends on %s which is not available", s.source.String(), ident.String()),
			Subject:  ident.DeclRange.Ptr(),
		})
	}
	return val, diags
}

func (s staticScopeData) GetLocalValue(ident addrs.LocalValue, rng tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	local, ok := s.ctx.cfg.Locals[ident.Name]
	if !ok {
		return cty.DynamicVal, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Undefined local",
			Detail:   fmt.Sprintf("Undefined local %s", ident.String()),
			Subject:  rng.ToHCL().Ptr(),
		})
	}

	id := StaticIdentifier{
		Module:    s.ctx.Call.Addr,
		Subject:   addrs.LocalValue{Name: local.Name},
		DeclRange: local.DeclRange,
	}

	return s.eval(id, func(stack []StaticIdentifier) (cty.Value, hcl.Diagnostics) {
		val, diags := s.ctx.scope(id, stack).EvalExpr(local.Expr, cty.DynamicPseudoType)
		return val, diags.ToHCL()
	})
}
func (s staticScopeData) GetModule(addrs.ModuleCall, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}
func (s staticScopeData) GetPathAttr(addr addrs.PathAttr, rng tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	// TODO this is copied and trimed down from tofu/evaluate.go GetPathAttr.  Ideally this should be refactored to a common location.
	var diags tfdiags.Diagnostics
	switch addr.Name {

	case "cwd":
		wd, err := os.Getwd()
		if err != nil {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  `Failed to get working directory`,
				Detail:   fmt.Sprintf(`The value for path.cwd cannot be determined due to a system error: %s`, err),
				Subject:  rng.ToHCL().Ptr(),
			})
			return cty.DynamicVal, diags
		}
		wd, err = filepath.Abs(wd)
		if err != nil {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  `Failed to get working directory`,
				Detail:   fmt.Sprintf(`The value for path.cwd cannot be determined due to a system error: %s`, err),
				Subject:  rng.ToHCL().Ptr(),
			})
			return cty.DynamicVal, diags
		}

		return cty.StringVal(filepath.ToSlash(wd)), diags

	case "module":
		return cty.StringVal(s.ctx.cfg.SourceDir), diags

	case "root":
		return cty.StringVal(s.ctx.Call.RootPath), diags

	default:
		suggestion := didyoumean.NameSuggestion(addr.Name, []string{"cwd", "module", "root"})
		if suggestion != "" {
			suggestion = fmt.Sprintf(" Did you mean %q?", suggestion)
		}
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  `Invalid "path" attribute`,
			Detail:   fmt.Sprintf(`The "path" object does not have an attribute named %q.%s`, addr.Name, suggestion),
			Subject:  rng.ToHCL().Ptr(),
		})
		return cty.DynamicVal, diags
	}
}
func (s staticScopeData) GetTerraformAttr(ident addrs.TerraformAttr, rng tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}
func (s staticScopeData) GetInputVariable(ident addrs.InputVariable, rng tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	variable, ok := s.ctx.cfg.Variables[ident.Name]
	if !ok {
		return cty.NilVal, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Undefined variable",
			Detail:   fmt.Sprintf("Undefined variable %s", ident.String()),
			Subject:  rng.ToHCL().Ptr(),
		})
	}

	id := StaticIdentifier{
		Module:    s.ctx.Call.Addr,
		Subject:   addrs.InputVariable{Name: variable.Name},
		DeclRange: variable.DeclRange,
	}

	return s.eval(id, func(stack []StaticIdentifier) (cty.Value, hcl.Diagnostics) {
		return s.ctx.Call.Variables(variable)
	})
}
func (s staticScopeData) GetOutput(addrs.OutputValue, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}
func (s staticScopeData) GetCheckBlock(addrs.Check, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}

func (s *StaticContext) scope(ident StaticIdentifier, stack []StaticIdentifier) *lang.Scope {
	return &lang.Scope{
		Data:        staticScopeData{s, ident, stack},
		ParseRef:    addrs.ParseRef,
		SourceAddr:  ident.Subject,
		BaseDir:     ".", // Always current working directory for now. (same as Evaluator.Scope())
		PureOnly:    false,
		ConsoleMode: false,
	}
}

func (s StaticContext) Evaluate(expr hcl.Expression, ident StaticIdentifier) StaticValueFn {
	return func(stack []StaticIdentifier) (cty.Value, hcl.Diagnostics) {
		val, diags := s.scope(ident, stack).EvalExpr(expr, cty.DynamicPseudoType)
		return val, diags.ToHCL()
	}
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
