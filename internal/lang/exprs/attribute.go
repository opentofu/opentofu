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

// ### Regarding the possibility of instances of one resource being able to ###
// ### refer to each other without that being treated as a "self-reference" ###
// ### error...                                                             ###
//
// TODO: We could potentially have two more implementations of [Attribute] here
// which represent object and tuple values (respectively) that can potentially
// be partially-constructed when all of the references include a known
// index step underneath the attribute, but behave like a normal
// [ValueOf] if there's at least one reference that doesn't include a known
// index step. In principle that could be used for multi-instance objects
// like resources to allow instances to refer to each other without it being
// treated as a self-reference. This also seems like a necessary building block
// to replicate the traditional language runtime's ability for an input variable
// of a module call to depend on an output value from the same module call,
// because that means the input variable must be able to evaluate against a
// not-yet-complete object representing the module instance's output values.
//
// If the hcl.EvalContext builder has known index steps then it can build
// an object or tuple where any indices not accessed are either not populated
// at all (for an object) or set to [cty.DynamicVal] (for a tuple, where we
// need to populate any "gaps" between the indices being used).
//
// However, there's various other groundwork we'd need to do before we could
// make that work, including but probably not limited to:
// - Have some alternative to hcl.Traversal that can support index steps whose
//   keys are [Valuer] instead of static [cty.Value], so that a reference
//   like aws_instance.example[each.key] can have that each.key evaluated
//   as part of preparing the hcl.EvalContext and we can dynamically decide
//   which individual index to populate to satisfy that reference.
// - Some way to make sure that any marks that would normally be placed on
//   naked aws_instance.example still get applied to the result even when
//   we skip calling the [Valuer] for aws_instance.example as a whole.
//
// As long as we're using [hcl.Traversal] in its current form we would only
// be able to do this partial-building trick when the index key is a constant,
// like in aws_instance.example["foo"].

// NestedSymbolTableFromAttribute returns the symbol table from an attribute
// that was returned from [NestedSymbolTable], or nil for any other kind of
// attribute.
func NestedSymbolTableFromAttribute(attr Attribute) SymbolTable {
	withTable, ok := attr.(nestedSymbolTable)
	if !ok {
		return nil
	}
	return withTable.SymbolTable
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
