// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configschema

import (
	"github.com/zclconf/go-cty/cty"
)

// UnknownValue returns the "unknown value" for the receiving block, which for
// a block type is a non-null object where all of the attribute values are
// the unknown values of the block's attributes and nested block types.
//
// In other words, it returns the value that would be returned if an unknown
// block were decoded against the receiving schema, assuming that no required
// attribute or block constraints were honored.
func (b *Block) UnknownValue() cty.Value {
	vals := make(map[string]cty.Value)
	for name, attrS := range b.Attributes {
		vals[name] = cty.UnknownVal(attrS.ImpliedType())
	}
	for name, blockS := range b.BlockTypes {
		vals[name] = cty.UnknownVal(blockS.ImpliedType())
	}
	return cty.ObjectVal(vals)
}
