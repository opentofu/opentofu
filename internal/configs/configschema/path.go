// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configschema

import (
	"github.com/zclconf/go-cty/cty"
)

// AttributeByPath looks up the Attribute schema which corresponds to the given
// cty.Path. A nil value is returned if the given path does not correspond to a
// specific attribute.
func (b *Block) AttributeByPath(path cty.Path) *Attribute {
	block := b
	for i, step := range path {
		switch step := step.(type) {
		case cty.GetAttrStep:
			if attr := block.Attributes[step.Name]; attr != nil {
				// If the Attribute is defined with a NestedType and there's
				// more to the path, descend into the NestedType
				if attr.NestedType != nil && i < len(path)-1 {
					return attr.NestedType.AttributeByPath(path[i+1:])
				} else if i < len(path)-1 { // There's more to the path, but not more to this Attribute.
					return nil
				}
				return attr
			}

			if nestedBlock := block.BlockTypes[step.Name]; nestedBlock != nil {
				block = &nestedBlock.Block
				continue
			}

			return nil
		}
	}
	return nil
}

// AttributeByPath recurses through a NestedType to look up the Attribute scheme
// which corresponds to the given cty.Path. A nil value is returned if the given
// path does not correspond to a specific attribute.
func (o *Object) AttributeByPath(path cty.Path) *Attribute {
	for i, step := range path {
		switch step := step.(type) {
		case cty.GetAttrStep:
			if attr := o.Attributes[step.Name]; attr != nil {
				if attr.NestedType != nil && i < len(path)-1 {
					return attr.NestedType.AttributeByPath(path[i+1:])
				} else if i < len(path)-1 { // There's more to the path, but not more to this Attribute.
					return nil
				}
				return attr
			}
		}
	}
	return nil
}

// PathName is trying to get the name of the attribute referenced by the cty.Path.
// Due to the implementation difference between cty.IndexStep and cty.GetAttrStep,
// this method is trying to find the most right-sided cty.GetAttrStep and returns the
// name of that.
// If the path contains no cty.GetAttrStep, it returns <unknown>.
// TODO ephemeral - is there a better way to do this?
func PathName(path cty.Path) string {
	for i := len(path) - 1; i >= 0; i-- {
		step := path[i]
		switch st := step.(type) {
		case cty.GetAttrStep:
			return st.Name
		}
	}
	return "<unknown>"
}
