// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

func selfScope(parentScope exprs.Scope, self cty.Value, ident string, name string) exprs.Scope {
	return &selfOverlayScope{
		ident:  ident,
		name:   name,
		self:   self,
		parent: parentScope,
	}
}

type selfOverlayScope struct {
	ident  string
	name   string
	self   cty.Value
	parent exprs.Scope
}

// HandleInvalidStep implements exprs.Scope.
func (i *selfOverlayScope) HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics {
	return i.parent.HandleInvalidStep(rng)
}

// ResolveAttr implements exprs.Scope.
func (i *selfOverlayScope) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	if ref.Name == "self" {
		return exprs.ValueOf(exprs.ConstantValuer(i.self)), nil
	}

	parentRef, diags := i.parent.ResolveAttr(ref)
	if ref.Name == i.ident && !diags.HasErrors() {
		nestedTable := exprs.NestedSymbolTableFromAttribute(parentRef)
		if nestedTable != nil {
			return exprs.NestedSymbolTable(&selfOverlaySymbolTable{
				name:        i.name,
				self:        i.self,
				nestedTable: nestedTable,
			}), diags
		}
	}

	// Everything else is delegated to the parent scope.
	return parentRef, diags

}

// ResolveFunc implements exprs.Scope.
func (i *selfOverlayScope) ResolveFunc(ctx context.Context, call *hcl.StaticCall) (function.Function, tfdiags.Diagnostics) {
	// no extra functions in this local scope
	return i.parent.ResolveFunc(ctx, call)
}

type selfOverlaySymbolTable struct {
	name        string
	self        cty.Value
	nestedTable exprs.SymbolTable
}

// ResolveAttr implements exprs.SymbolTable.
func (s *selfOverlaySymbolTable) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	if ref.Name == s.name {
		return exprs.ValueOf(exprs.ConstantValuer(s.self)), nil
	}
	return s.nestedTable.ResolveAttr(ref)
}

// HandleInvalidStep implements exprs.SymbolTable.
func (s *selfOverlaySymbolTable) HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics {
	return s.nestedTable.HandleInvalidStep(rng)
}
