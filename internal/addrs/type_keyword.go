// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

// TypeKeyword is a special placeholder address type used only for representing
// mentions of type names like "string", "number", etc that are expected
// to appear in type expressions but not in value expressions.
//
// This is used as part of a special workaround in [ParseRef] that allows
// static analysis of references in an expression to succeed before we get
// far enough to distinguish whether a reference is in an expression
// representing a type or a value. The code elsewhere in the system that
// builds HCL EvalContext for value-based evaluation is then expected to just
// silently ignore references to addresses of this type so that HCL would
// return its usual error for an unrecognized symbol if a type keyword appears
// in a context where a value expression is expected.
type TypeKeyword string

var _ Referenceable = TypeKeyword("")
var _ UniqueKey = TypeKeyword("")

// isTypeKeyword returns true if the given symbol is one that, if appearing as
// the first and only step of an HCL traversal, should be considered to
// represent a type rather than a value.
func isTypeKeyword(symbol string) bool {
	switch symbol {
	// The following is expected to cover all of the keywords that the type
	// expression parser recognizes as if they were symbol table traversals.
	// It does not need to include the keywords that are used with function
	// call syntax to construct new parameterized types.
	case "string", "number", "bool", "any":
		return true
	default:
		return false
	}
}

// String implements [Referenceable].
func (t TypeKeyword) String() string {
	// The canonical representation of a type keyword is literally just that keyword.
	return string(t)
}

// UniqueKey implements [Referenceable].
func (t TypeKeyword) UniqueKey() UniqueKey {
	// A type keyword acts as its own UniqueKey, because this type is comparable.
	return UniqueKey(t)
}

// referenceableSigil implements [Referenceable].
func (t TypeKeyword) referenceableSigil() {}

// uniqueKeySigil implements [UniqueKey].
func (t TypeKeyword) uniqueKeySigil() {}
