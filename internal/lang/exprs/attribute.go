// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exprs

// Attribute is implemented for the fixed set of types that can be returned
// from [SymbolTable.ResolveAttr] implementations.
//
// This is a closed interface implemented only by types within this package.
type Attribute interface {
	attributeImpl()
}

// NestedSymbolTable constructs an [Attribute] representing a nested symbol
// table, which must therefore be traversed through by a subsequent attribute
// step.
//
// For example, a module instance acting as a symbol table would respond to
// a lookup of the attribute "var" by returning a nested symbol table whose
// symbols correspond to all of the input variables declared in the module,
// so that a reference like "var.foo" would then look up "foo" in the nested
// table.
func NestedSymbolTable(table SymbolTable) Attribute {
	return nestedSymbolTable{table}
}

// ValueOf constructs an [Attribute] representing the endpoint of a static
// traversal, where a dynamic value should be placed.
func ValueOf(v Valuer) Attribute {
	return valueOf{v}
}

// nestedSymbolTable is the [Attribute] implementation for symbols that act as
// nested symbol tables, resolving another set of child attributes within.
type nestedSymbolTable struct {
	SymbolTable
}

// scopeStep implements [Attribute].
func (n nestedSymbolTable) attributeImpl() {}

// valueOf is the [Attribute] implementation for symbols that correspond to
// leaf values, produced by implementations of [Valuer].
type valueOf struct {
	Valuer
}

// scopeStep implements [Attribute].
func (v valueOf) attributeImpl() {}
