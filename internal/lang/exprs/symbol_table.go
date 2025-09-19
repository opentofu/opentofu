// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exprs

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// SymbolTable is an interface implemented by types that have an associated
// symbol table, meaning that they contain a set of attributes that can be
// looked up by name.
type SymbolTable interface {
	// ResolveSymbol looks up a symbol by name, either returning what it
	// refers to or error diagnostics if no such symbol exists.
	ResolveAttr(ref hcl.TraverseAttr) (Attribute, tfdiags.Diagnostics)

	// HandleInvalidStep is called if a reference contains anything other
	// than an attribute access at a position handled by a symbol table,
	// so that the symbol table can produce a specialized error message
	// explaining what kind of attributes are expected.
	//
	// The given source range refers either to the non-attribute step that
	// was encountered or, if the problem is that nothing was present at all,
	// then to the entire reference expression visited so far.
	//
	// The result of this method MUST include at least one error diagnostic.
	HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics
}
