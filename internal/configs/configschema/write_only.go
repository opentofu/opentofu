// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configschema

import (
	"fmt"

	"github.com/zclconf/go-cty/cty"
)

// PathSetContainsWriteOnly checks that the given cty.PathSet contains *at least* one
// of write-only attribute from the given value's schema.
func (b *Block) PathSetContainsWriteOnly(v cty.Value, ps cty.PathSet) bool {
	paths := b.WriteOnlyPaths(v, nil)
	for _, path := range paths {
		if ps.Has(path) {
			return true
		}
	}
	return false
}

// WriteOnlyPaths returns the list of paths where write-only attributes
// exist in the given value.
// This logic is similar to the Block.ValueMarks since the logic of drilling into the
// value is similar.
func (b *Block) WriteOnlyPaths(val cty.Value, path cty.Path) []cty.Path {
	var res []cty.Path

	// No need to get the paths since the value has no values inside.
	if val.IsNull() || !val.IsKnown() {
		return res
	}

	for name, attrS := range b.Attributes {
		if attrS.WriteOnly {
			// Create a copy of the path, with this step added, to add to our paths slice
			attrPath := copyAndExtendPath(path, cty.GetAttrStep{Name: name})
			res = append(res, attrPath)
		}
	}

	// Extract paths for nested attribute type values
	for name, attrS := range b.Attributes {
		// If the attribute has no nested type, or the nested type doesn't
		// contain any write-only attributes, skip inspecting it
		if attrS.NestedType == nil || !attrS.NestedType.ContainsWriteOnly() {
			continue
		}

		// Create a copy of the path, with this step added, to add to our paths slice
		attrPath := copyAndExtendPath(path, cty.GetAttrStep{Name: name})

		res = append(res, attrS.NestedType.WriteOnlyPaths(val.GetAttr(name), attrPath)...)
	}

	// Extract paths from nested blocks
	for name, blockS := range b.BlockTypes {
		// If our block doesn't contain any write-only attributes, skip inspecting it
		if !blockS.Block.ContainsWriteOnly() {
			continue
		}

		blockV := val.GetAttr(name)
		if blockV.IsNull() || !blockV.IsKnown() {
			continue
		}

		// Create a copy of the path, with this step added, to add to our paths slice
		blockPath := copyAndExtendPath(path, cty.GetAttrStep{Name: name})

		switch blockS.Nesting {
		case NestingSingle, NestingGroup:
			res = append(res, blockS.Block.WriteOnlyPaths(blockV, blockPath)...)
		case NestingList, NestingMap, NestingSet:
			for it := blockV.ElementIterator(); it.Next(); {
				idx, blockEV := it.Element()
				// Create a copy of the path, with this block instance's index
				// step added, to add to our paths slice
				blockInstancePath := copyAndExtendPath(blockPath, cty.IndexStep{Key: idx})
				morePaths := blockS.Block.WriteOnlyPaths(blockEV, blockInstancePath)
				res = append(res, morePaths...)
			}
		default:
			panic(fmt.Sprintf("unsupported nesting mode %s", blockS.Nesting))
		}
	}
	return res
}

// WriteOnlyPaths returns a slice of paths pointing to the attributes that are
// configured as write-only.
func (o *Object) WriteOnlyPaths(val cty.Value, path cty.Path) []cty.Path {
	var res []cty.Path

	if val.IsNull() || !val.IsKnown() {
		return res
	}

	for name, attrS := range o.Attributes {
		// Skip attributes which can never produce write-only paths
		if !attrS.WriteOnly && (attrS.NestedType == nil || !attrS.NestedType.ContainsWriteOnly()) {
			continue
		}

		switch o.Nesting {
		case NestingSingle, NestingGroup:
			// Create a path to this attribute
			attrPath := copyAndExtendPath(path, cty.GetAttrStep{Name: name})

			if attrS.WriteOnly {
				// If the entire attribute is write-only save that path fully
				res = append(res, attrPath)
			} else {
				// The attribute has a nested type which contains write-only
				// attributes, so recurse
				res = append(res, attrS.NestedType.WriteOnlyPaths(val.GetAttr(name), attrPath)...)
			}
		case NestingList, NestingMap, NestingSet:
			// For nested attribute types which have a non-single nesting mode,
			// we add write-only paths for each element.
			for it := val.ElementIterator(); it.Next(); {
				idx, attrEV := it.Element()
				attrV := attrEV.GetAttr(name)

				// Create a path to this element of the attribute's collection. Note
				// that the path is extended in opposite order to the iteration order
				// of the loops: index into the collection, then the contained
				// attribute name. This is because we have one type
				// representing multiple collection elements.
				attrPath := copyAndExtendPath(path, cty.IndexStep{Key: idx}, cty.GetAttrStep{Name: name})

				if attrS.WriteOnly {
					// If the entire attribute is configured as write only, mark it so
					res = append(res, attrPath)
				} else {
					// The attribute has a nested type which contains write-only
					// attributes, so recurse
					res = append(res, attrS.NestedType.WriteOnlyPaths(attrV, attrPath)...)
				}
			}
		default:
			panic(fmt.Sprintf("unsupported nesting mode %s", attrS.NestedType.Nesting))
		}
	}
	return res
}

// ContainsWriteOnly returns true if any of the attributes of the receiving
// block or any of its descendent blocks are marked as write-only.
//
// Blocks themselves cannot be write-only as a whole -- write-only is a
// per-attribute idea -- but sometimes we want to include a whole object
// decoded from a block in some UI output, and that is safe to do only if
// none of the contained attributes are write-only.
func (b *Block) ContainsWriteOnly() bool {
	for _, attrS := range b.Attributes {
		if attrS.WriteOnly {
			return true
		}
		if attrS.NestedType != nil && attrS.NestedType.ContainsWriteOnly() {
			return true
		}
	}
	for _, blockS := range b.BlockTypes {
		if blockS.ContainsWriteOnly() {
			return true
		}
	}
	return false
}

// ContainsWriteOnly returns true if any of the attributes of the receiving
// Object are marked as write-only.
func (o *Object) ContainsWriteOnly() bool {
	for _, attrS := range o.Attributes {
		if attrS.WriteOnly {
			return true
		}
		if attrS.NestedType != nil && attrS.NestedType.ContainsWriteOnly() {
			return true
		}
	}
	return false
}
