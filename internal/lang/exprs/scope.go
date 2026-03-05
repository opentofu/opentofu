// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exprs

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// Scope is implemented by types representing containers that can have
// expressions evaluated within them.
//
// For example, it might make sense for a type representing a module instance
// to implement this interface to describe the symbols and functions that
// result from the declarations in that module instance. In that case, the
// module instance _is_ the scope, rather than the scope being a separate thing
// derived from that module instance.
//
// A Scope is essentially just an extension of [SymbolTable] which also includes
// a table of functions.
type Scope interface {
	SymbolTable

	// ResolveFunc looks up a function by name, either returning its
	// implementation or error diagnostics if no such function exists.
	ResolveFunc(call *hcl.StaticCall) (function.Function, tfdiags.Diagnostics)
}

// ChildScopeBuilder is the signature for a function that can build a child
// scope that wraps some given parent scope.
//
// A nil [ChildScopeBuilder] represents that no child scope is needed and the
// parent should just be used directly. Use [ChildScopeBuilder.Build] instead
// of directly calling the function to obtain that behavior automatically.
type ChildScopeBuilder func(ctx context.Context, parent Scope) Scope

func (b ChildScopeBuilder) Build(ctx context.Context, parent Scope) Scope {
	if b == nil {
		return parent
	}
	return b(ctx, parent)
}

// FlatTestingScope returns a [Scope] implementation that is intended for
// isolated testing of codepaths that do arbitrary expression evaluation without
// any direct expectation about what's in scope, by providing just a flat
// table of top-level symbols.
//
// This is not intended for use in non-test code, and it's probably also not
// an ergonomic way to test codepaths that expect a specific shape of symbol
// table, such as the global scope of a module. The best use of this is when
// you just want to test whether a codepath is correctly using a scope provided
// by its caller, where you can tailor the expressions to suit the constraints
// of this function.
//
// The returned scope does not include any functions.
func FlatScopeForTesting(symbols map[string]cty.Value) Scope {
	return flatValueScope(symbols)
}

type flatValueScope map[string]cty.Value

// HandleInvalidStep implements [Scope].
func (s flatValueScope) HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	diags = diags.Append(fmt.Errorf("invalid reference at %s", rng.StartString()))
	return diags
}

// ResolveAttr implements [Scope].
func (s flatValueScope) ResolveAttr(ref hcl.TraverseAttr) (Attribute, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	v, ok := s[ref.Name]
	if !ok {
		diags = diags.Append(fmt.Errorf("no symbol named %q", ref.Name))
		return nil, diags
	}
	return ValueOf(ConstantValuer(v)), diags
}

// ResolveFunc implements [Scope].
func (s flatValueScope) ResolveFunc(call *hcl.StaticCall) (function.Function, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	diags = diags.Append(fmt.Errorf("no function named %s", call.Name))
	return function.Function{}, diags
}
