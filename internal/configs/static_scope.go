// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/didyoumean"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// newStaticScope creates a lang.Scope that's backed by the static view of the module represented by the StaticEvaluator
func newStaticScope(eval *StaticEvaluator, stack0 StaticIdentifier, stack ...StaticIdentifier) *lang.Scope {
	return &lang.Scope{
		Data:        staticScopeData{eval, append([]StaticIdentifier{stack0}, stack...)},
		ParseRef:    addrs.ParseRef,
		BaseDir:     ".", // Always current working directory for now. (same as Evaluator.Scope())
		PureOnly:    false,
		ConsoleMode: false,
	}
}

// This structure represents the data required to evaluate a specific identifier reference (top of the stack)
// It is used by lang.Scope to link the given StaticEvaluator data to addrs.References in the current scope.
type staticScopeData struct {
	eval  *StaticEvaluator
	stack []StaticIdentifier
}

// staticScopeData must implement lang.Data
var _ lang.Data = (*staticScopeData)(nil)

// Creates a nested scope to evaluate nested references
func (s staticScopeData) scope(ident StaticIdentifier) (*lang.Scope, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	for _, frame := range s.stack {
		if frame.String() == ident.String() {
			return nil, diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Circular reference",
				Detail:   fmt.Sprintf("%s is self referential", ident.String()), // TODO use stack in error message
				Subject:  ident.DeclRange.Ptr(),
			})
		}
	}
	return newStaticScope(s.eval, s.stack[0], append(s.stack[1:], ident)...), diags
}

// If an error occurs when resolving a dependent value, we need to add additional context to the diagnostics
func (s staticScopeData) enhanceDiagnostics(ident StaticIdentifier, diags tfdiags.Diagnostics) tfdiags.Diagnostics {
	if diags.HasErrors() {
		top := s.stack[len(s.stack)-1]
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to compute static value",
			Detail:   fmt.Sprintf("%s depends on %s which is not available", top, ident.String()),
			Subject:  top.DeclRange.Ptr(),
		})
	}
	return diags
}

// Early check to only allow references we expect in a static context
func (s staticScopeData) StaticValidateReferences(refs []*addrs.Reference, _ addrs.Referenceable, _ addrs.Referenceable) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	top := s.stack[len(s.stack)-1]
	for _, ref := range refs {
		switch subject := ref.Subject.(type) {
		case addrs.LocalValue:
			continue
		case addrs.InputVariable:
			continue
		case addrs.PathAttr:
			continue
		case addrs.TerraformAttr:
			continue
		case addrs.ModuleCallInstanceOutput:
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Module output not supported in static context",
				Detail:   fmt.Sprintf("Unable to use %s in static context, which is required by %s", subject.String(), top.String()),
				Subject:  ref.SourceRange.ToHCL().Ptr(),
			})
		case addrs.ProviderFunction:
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Provider function in static context",
				Detail:   fmt.Sprintf("Unable to use %s in static context, which is required by %s", subject.String(), top.String()),
				Subject:  ref.SourceRange.ToHCL().Ptr(),
			})
		default:
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Dynamic value in static context",
				Detail:   fmt.Sprintf("Unable to use %s in static context, which is required by %s", subject.String(), top.String()),
				Subject:  ref.SourceRange.ToHCL().Ptr(),
			})
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

func (s staticScopeData) GetLocalValue(ident addrs.LocalValue, rng tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	local, ok := s.eval.cfg.Locals[ident.Name]
	if !ok {
		return cty.DynamicVal, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Undefined local",
			Detail:   fmt.Sprintf("Undefined local %s", ident.String()),
			Subject:  rng.ToHCL().Ptr(),
		})
	}

	id := StaticIdentifier{
		Module:    s.eval.call.addr,
		Subject:   fmt.Sprintf("local.%s", local.Name),
		DeclRange: local.DeclRange,
	}

	scope, scopeDiags := s.scope(id)
	diags = diags.Append(scopeDiags)
	if diags.HasErrors() {
		return cty.DynamicVal, diags
	}

	val, valDiags := scope.EvalExpr(context.TODO(), local.Expr, cty.DynamicPseudoType)
	return val, s.enhanceDiagnostics(id, diags.Append(valDiags))
}

func (s staticScopeData) GetModule(addrs.ModuleCall, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}

func (s staticScopeData) GetPathAttr(addr addrs.PathAttr, rng tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	// TODO this is copied and trimmed down from tofu/evaluate.go GetPathAttr.  Ideally this should be refactored to a common location.
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
		return cty.StringVal(s.eval.cfg.SourceDir), diags

	case "root":
		return cty.StringVal(s.eval.call.rootPath), diags

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

func (s staticScopeData) GetTerraformAttr(addr addrs.TerraformAttr, rng tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	// TODO this is copied and trimmed down from tofu/evaluate.go GetTerraformAttr.  Ideally this should be refactored to a common location.
	var diags tfdiags.Diagnostics
	switch addr.Name {
	case "workspace":
		workspaceName := s.eval.call.workspace
		return cty.StringVal(workspaceName), diags

	case "env":
		// Prior to Terraform 0.12 there was an attribute "env", which was
		// an alias name for "workspace". This was deprecated and is now
		// removed.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  `Invalid "terraform" attribute`,
			Detail:   `The terraform.env attribute was deprecated in v0.10 and removed in v0.12. The "state environment" concept was renamed to "workspace" in v0.12, and so the workspace name can now be accessed using the terraform.workspace attribute.`,
			Subject:  rng.ToHCL().Ptr(),
		})
		return cty.DynamicVal, diags

	default:
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  `Invalid "terraform" attribute`,
			Detail:   fmt.Sprintf(`The "terraform" object does not have an attribute named %q. The only supported attribute is terraform.workspace, the name of the currently-selected workspace.`, addr.Name),
			Subject:  rng.ToHCL().Ptr(),
		})
		return cty.DynamicVal, diags
	}
}

func (s staticScopeData) GetInputVariable(ident addrs.InputVariable, rng tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	variable, ok := s.eval.cfg.Variables[ident.Name]
	if !ok {
		return cty.NilVal, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Undefined variable",
			Detail:   fmt.Sprintf("Undefined variable %s", ident.String()),
			Subject:  rng.ToHCL().Ptr(),
		})
	}

	id := StaticIdentifier{
		Module:    s.eval.call.addr,
		Subject:   fmt.Sprintf("var.%s", variable.Name),
		DeclRange: variable.DeclRange,
	}

	val, valDiags := s.eval.call.vars(variable)
	diags = diags.Append(valDiags)
	if valDiags.HasErrors() {
		// If the variable value was too invalid to pass the initial request
		// then we'll bail out immediately here since our other checks below
		// are likely to produce redundant errors that would be confusing.
		return cty.DynamicVal, s.enhanceDiagnostics(id, diags)
	}

	// "val" now contains the raw value passed by the caller. We need to prepare it
	// in various ways based on the declaration, so that it conforms to the
	// requirements expected by the module author.
	// FIXME: This is currently essentially a duplication of various logic from
	// internal/tofu/eval_variable.go, prepareFinalInputVariableValue.
	// We should find some way for both of these codepaths to share this logic.

	convertTy := variable.ConstraintType
	if convertTy == cty.NilType {
		convertTy = cty.DynamicPseudoType
	}

	// FIXME: Failing when a required variable isn't set or substituting its default
	// when it is could be implemented centrally here, but currently variations of
	// that are duplicated into each implementation of s.eval.call.vars, so we'll
	// just assume that the default was already substituted if appropriate.
	// We do still need to re-evaluate the default here though, because we might
	// need it downstream.
	defaultVal := variable.Default
	if defaultVal != cty.NilVal {
		var err error
		defaultVal, err = convert.Convert(defaultVal, convertTy)
		if err != nil {
			// We shouldn't get here because the default value's convertability to
			// the type constraint is checked during config decoding, but we'll
			// handle this here just to be robust.
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid default value for module argument",
				Detail: fmt.Sprintf(
					"The default value for variable %q is incompatible with its type constraint: %s.",
					variable.Name, err,
				),
				Subject: &variable.DeclRange,
			})
			return cty.UnknownVal(variable.ConstraintType), diags
		}
	}

	// Some type constraints contain default values to use when attributes are null,
	// so we need to resolve those before we proceed further.
	if variable.TypeDefaults != nil && !val.IsNull() {
		val = variable.TypeDefaults.Apply(val)
	}

	// The module author specifies what type of value they are expecting. The
	// caller is allowed to provide any value that can convert to the given
	// type constraint, while expressions in the module only see the converted
	// value.
	var err error
	val, err = convert.Convert(val, convertTy)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid value for input variable",
			Detail: fmt.Sprintf(
				"The given value is not suitable for %s declared at %s: %s.",
				id.Subject, id.DeclRange, err,
			),
		})
		// We'll return a placeholder unknown value to avoid producing
		// redundant downstream errors.
		return cty.UnknownVal(variable.Type), diags
	}

	// By the time we get here, we know:
	// - val matches the variable's type constraint
	// - val is definitely not cty.NilVal, but might be a null value if the given was already null.
	//
	// That means we just need to handle the case where the value is null,
	// which might mean we need to use the default value, or produce an error.
	//
	// For historical reasons we do this only for a "non-nullable" variable.
	// Nullable variables just appear as null if they were set to null,
	// regardless of any default value.
	if val.IsNull() && !variable.Nullable {
		if defaultVal != cty.NilVal {
			val = defaultVal
		} else {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  `Required variable not set`,
				Detail: fmt.Sprintf(
					"The given value is not suitable for %s defined at %s: required variable may not be set to null.",
					id.Subject, id.DeclRange.String(),
				),
				Subject: id.DeclRange.Ptr(),
			})
		}
	}

	if variable.Sensitive {
		// Sensitive input variables must always be marked on the way in, so
		// that their sensitivity can propagate to derived expressions.
		val = val.Mark(marks.Sensitive)
	}

	// TODO: We should check the variable validations here too, since module
	// authors expect that any value that doesn't meet the validation requirements
	// will not propagate to any other expression in the module, but that's
	// a non-trivial amount of code to duplicate here so we'll just let it be
	// for now. This means that invalid variable values can make it into
	// static eval contexts where they will probably produce confusing errors,
	// but if we manage to get far enough despite that then we'll catch the
	// variable validation errors once we reach the dynamic eval phase.

	return val, s.enhanceDiagnostics(id, diags)
}

func (s staticScopeData) GetOutput(addrs.OutputValue, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}

func (s staticScopeData) GetCheckBlock(addrs.Check, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	panic("Not Available in Static Context")
}
