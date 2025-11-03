// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libraries

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// symbols is our internal representation of symbols decoded from an HCL
// body, which could either be the top-level of a library file where we
// represent exported symbols or inside a "private" block where we represent
// library-private symbols.
type symbols struct {
	values      map[string]*valueDef
	functions   map[string]*functionDef
	typeAliases map[string]*typeAliasDef
}

type valueDef struct {
	DeclRange tfdiags.SourceRange
}

type functionDef struct {
	DeclRange tfdiags.SourceRange
}

type typeAliasDef struct {
	DeclRange tfdiags.SourceRange
}

// decodeSymbols attempts to decode symbol declarations from the given
// body, returning both the decoded symbols and a body representing anything
// else that was present that wasn't recognized as a symbol.
func decodeSymbols(body hcl.Body) (*symbols, hcl.Body, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	content, remain, hclDiags := body.PartialContent(symbolsSchema)
	diags = diags.Append(hclDiags)

	ret := &symbols{
		values:      make(map[string]*valueDef),
		functions:   make(map[string]*functionDef),
		typeAliases: make(map[string]*typeAliasDef),
	}

	for _, block := range content.Blocks {
		_ = block
	}

	return ret, remain, diags
}

var symbolsSchema = &hcl.BodySchema{
	// symbolsSchema must be defined as a subset of librarySchema,
	// because we reuse the same decoding code for content decoded under
	// both of these schemas, in [decodeSymbols].
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "values"},
		{Type: "function", LabelNames: []string{"name"}},
		{Type: "type_alias", LabelNames: []string{"name"}},
	},
}
