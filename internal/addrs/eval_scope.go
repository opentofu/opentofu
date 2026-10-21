// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// EvalScope is the address of something tha can have expressions evaluated
// inside it.
//
// More specifically, [Referenceable] addresses are always resolved relative
// to some container that determines which symbols are available, and
// EvalScope values can be used to refer to a subset of those containers.
//
// This type is not capable of identifying every possible specialist scope
// where expressions might be evaluated: it focuses only on some prominent ones
// that are useful for end-users to interact with directly using features like
// the "tofu console" command.
type EvalScope interface {
	fmt.Stringer

	// evalScopeSigil is used only to mark which types are considered to
	// implement this closed interface.
	evalScopeSigil()
}

// ModuleInstance can be used as an [EvalScope].
func (ModuleInstance) evalScopeSigil() {}

// ParseEvalScope attempts to interpret the given absolute traversal as the
// address of an evaluation scope. This function effectively defines what subset
// of scopes can be directly selected by end-user-facing features such as the
// "tofu console" command.
func ParseEvalScope(traversal hcl.Traversal) (EvalScope, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Currently only module instance addresses are allowed as explicit
	// scopes. Other possible addressable scopes could include instances of
	// resources or module calls where symbols like "each.key" would be
	// available, but we don't yet know of a compelling use-case for that to
	// justify the additional complexity it would imply for the evaluator
	// implementation.

	if len(traversal) == 0 {
		// Special case for referring to the root module.
		return RootModuleInstance, diags
	}

	ret, remain, moreDiags := parseModuleInstancePrefixWithOptionalRanges(traversal, false)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	if len(remain) != 0 {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid evaluation scope address",
			Detail:   "Only an entire module instance can be used as an evaluation scope.",
			Subject:  remain.SourceRange().Ptr(),
		})
		return nil, diags
	}

	return ret, diags
}

// ParseEvalScopeStr is a helper wrapper around [ParseEvalScope] that first
// attempts to parse the given string as HCL traversal syntax.
func ParseEvalScopeStr(str string) (EvalScope, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	if len(strings.TrimSpace(str)) == 0 {
		// Empty string would not be accepted as valid HCL traversal syntax but
		// represents the root module as a special case.
		return RootModuleInstance, diags
	}

	traversal, hclDiags := hclsyntax.ParseTraversalAbs([]byte(str), "", hcl.Pos{Line: 1, Column: 1})
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	ret, moreDiags := ParseEvalScope(traversal)
	diags = diags.Append(moreDiags)
	return ret, diags
}
