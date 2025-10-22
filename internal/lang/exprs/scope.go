// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exprs

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/tfdiags"
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
