// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ExprScope is an interface type implemented by address types that can be used to
// explicitly identify expression evaluation scopes.
//
// Not all evaluation scopes can be represented by implementations of this interface.
// The implementations are primarily motivated by what's useful to use with
// external-facing expression evaluation mechanisms, like the "tofu console" command.
type ExprScope interface {
	scopeSigil()
}

var (
	_ ExprScope = ModuleInstance(nil)
)

// ParseExprScope attempts to parse the given traversal as an expression scope address.
//
// If the returned diagnostics returns errors then the [ExprScope] result is unspecified
// and must not be used.
//
// Note that [addrs.RootModuleInstance], the singleton instance of the root module, is a
// valid expression scope but has no address that can be written as a traversal. Callers
// intending to allow user-specified scope addresses must offer some other way to select
// the root module scope, such as selecting it by default if no other scope is specified.
func ParseExprScope(traversal hcl.Traversal) (ExprScope, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Currently the only scope type we support is a module instance address.
	// We'll check for that explicitly before we try to parse it so that we can
	// return a more specific error message for this case.
	if traversal.RootName() != "module" {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unsupported expression scope address",
			Detail:   "An expression scope must be the address of a module instance, starting with the 'module.' prefix.",
			Subject:  traversal.SourceRange().Ptr(),
		})
		return nil, diags
	}

	// If we know that the address starts with "module." then we can assume
	// caller intent for this to be a module instance address and so just
	// rely on the normal module instance address parsing.
	addr, parseDiags := ParseModuleInstance(traversal)
	diags = diags.Append(parseDiags)
	return addr, diags
}

// ParseExprScopeStr is a helper wrapper around [ParseExprScope] that first asks
// HCL to parse the given string as an absolute traversal, and then parses that
// traversal into an expression scope.
func ParseExprScopeStr(str string) (ExprScope, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	traversal, parseDiags := hclsyntax.ParseTraversalAbs([]byte(str), "", hcl.Pos{Line: 1, Column: 1})
	diags = diags.Append(parseDiags)
	if parseDiags.HasErrors() {
		return nil, diags
	}

	scopeAddr, scopeDiags := ParseExprScope(traversal)
	diags = diags.Append(scopeDiags)
	return scopeAddr, diags
}
