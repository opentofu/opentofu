// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configschema

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// copyAndExtendPath returns a copy of a cty.Path with some additional
// `cty.PathStep`s appended to its end, to simplify creating new child paths.
func copyAndExtendPath(path cty.Path, nextSteps ...cty.PathStep) cty.Path {
	newPath := make(cty.Path, len(path), len(path)+len(nextSteps))
	copy(newPath, path)
	newPath = append(newPath, nextSteps...)
	return newPath
}

func deprecationMark(subject *addrs.AbsResourceInstance, path cty.Path, message string) any {
	return marks.DeprecationMark(subject.Module, subject.Resource.String()+tfdiags.FormatCtyPath(path), message)
}

// ValueMarks returns a set of path value marks for a given value and path,
// based on the sensitive flag for each attribute within the schema. Nested
// blocks are descended (if present in the given value).
// If subject is nil, deprecated marks will not be added. This is an intended usage.
func (b *Block) ValueMarks(val cty.Value, path cty.Path, subject *addrs.AbsResourceInstance) []cty.PathValueMarks {
	var pvm []cty.PathValueMarks

	var blockMarks []any

	// When the block is marked as ephemeral, the whole value needs to be marked accordingly.
	// Inner attributes should carry no ephemeral mark.
	// The ephemerality of the attributes is given by the mark on the val and not by individual marks
	// as it's the case for the sensitive mark.
	if b.Ephemeral {
		blockMarks = append(blockMarks, marks.Ephemeral)
	}
	if b.Deprecated && subject != nil {
		blockMarks = append(blockMarks, deprecationMark(subject, path, b.DeprecationMessage))
	}

	if len(blockMarks) != 0 {
		pvm = append(pvm, cty.PathValueMarks{
			Path:  path, // raw received path is indicating that the whole value needs to be marked.
			Marks: cty.NewValueMarks(blockMarks...),
		})
	}

	// We can mark attributes as sensitive even if the value is null
	for name, attrS := range b.Attributes {
		var attrMarks []any
		if attrS.Sensitive {
			attrMarks = append(attrMarks, marks.Sensitive)
		}
		if attrS.Deprecated && subject != nil {
			attrPath := copyAndExtendPath(path, cty.GetAttrStep{Name: name})
			attrMarks = append(attrMarks, deprecationMark(subject, attrPath, attrS.DeprecationMessage))
		}

		if len(attrMarks) != 0 {
			// Create a copy of the path, with this step added, to add to our PathValueMarks slice
			attrPath := copyAndExtendPath(path, cty.GetAttrStep{Name: name})
			pvm = append(pvm, cty.PathValueMarks{
				Path:  attrPath,
				Marks: cty.NewValueMarks(attrMarks...),
			})
		}
	}

	// If the value is null, no other marks are possible
	if val.IsNull() {
		return pvm
	}

	// Extract marks for nested attribute type values
	for name, attrS := range b.Attributes {
		// If the attribute has no nested type, or the nested type doesn't
		// contain any sensitive attributes, skip inspecting it
		if attrS.NestedType == nil || !attrS.NestedType.ContainsSensitive() && !attrS.NestedType.ContainsDeprecated() {
			continue
		}

		// Create a copy of the path, with this step added, to add to our PathValueMarks slice
		attrPath := copyAndExtendPath(path, cty.GetAttrStep{Name: name})

		pvm = append(pvm, attrS.NestedType.ValueMarks(val.GetAttr(name), attrPath, subject)...)
	}

	// Extract marks for nested blocks
	for name, blockS := range b.BlockTypes {
		// If our block doesn't contain any sensitive attributes, skip inspecting it
		if !blockS.Block.ContainsSensitive() && !blockS.Block.ContainsDeprecated() {
			continue
		}

		blockV := val.GetAttr(name)
		if blockV.IsNull() || !blockV.IsKnown() {
			continue
		}

		// Create a copy of the path, with this step added, to add to our PathValueMarks slice
		blockPath := copyAndExtendPath(path, cty.GetAttrStep{Name: name})

		switch blockS.Nesting {
		case NestingSingle, NestingGroup:
			pvm = append(pvm, blockS.Block.ValueMarks(blockV, blockPath, subject)...)
		case NestingList, NestingMap, NestingSet:
			for it := blockV.ElementIterator(); it.Next(); {
				idx, blockEV := it.Element()
				// Create a copy of the path, with this block instance's index
				// step added, to add to our PathValueMarks slice
				blockInstancePath := copyAndExtendPath(blockPath, cty.IndexStep{Key: idx})
				morePaths := blockS.Block.ValueMarks(blockEV, blockInstancePath, subject)
				pvm = append(pvm, morePaths...)
			}
		default:
			panic(fmt.Sprintf("unsupported nesting mode %s", blockS.Nesting))
		}
	}
	return pvm
}

// ValueMarks returns a set of path value marks for a given value and path,
// based on the sensitive flag for each attribute within the nested attribute.
// Attributes with nested types are descended (if present in the given value).
// If subject is nil, deprecated marks will not be added. This is an intended usage.
func (o *Object) ValueMarks(val cty.Value, path cty.Path, subject *addrs.AbsResourceInstance) []cty.PathValueMarks {
	var pvm []cty.PathValueMarks

	if val.IsNull() || !val.IsKnown() {
		return pvm
	}

	for name, attrS := range o.Attributes {
		// Skip attributes which can never produce sensitive or deprecate path value marks
		cantProduceSensitive := !attrS.Sensitive && (attrS.NestedType == nil || !attrS.NestedType.ContainsSensitive())
		cantProduceDeprecated := !attrS.Deprecated && (attrS.NestedType == nil || !attrS.NestedType.ContainsDeprecated()) || subject == nil
		if cantProduceSensitive && cantProduceDeprecated {
			continue
		}

		switch o.Nesting {
		case NestingSingle, NestingGroup:
			// Create a path to this attribute
			attrPath := copyAndExtendPath(path, cty.GetAttrStep{Name: name})

			var attrMarks []any
			if attrS.Sensitive {
				// If the entire attribute is sensitive, mark it so
				attrMarks = append(attrMarks, marks.Sensitive)
			}
			if attrS.Deprecated && subject != nil {
				// If the entire attribute is deprecated, mark it so
				attrMarks = append(attrMarks, deprecationMark(subject, attrPath, attrS.DeprecationMessage))
			}
			if len(attrMarks) != 0 {
				pvm = append(pvm, cty.PathValueMarks{
					Path:  attrPath,
					Marks: cty.NewValueMarks(attrMarks...),
				})
			} else {
				// The attribute has a nested type which contains sensitive
				// attributes, so recurse
				pvm = append(pvm, attrS.NestedType.ValueMarks(val.GetAttr(name), attrPath, subject)...)
			}
		case NestingList, NestingMap, NestingSet:
			// For nested attribute types which have a non-single nesting mode,
			// we add path value marks for each element of the collection
			for it := val.ElementIterator(); it.Next(); {
				idx, attrEV := it.Element()
				attrV := attrEV.GetAttr(name)

				// Create a path to this element of the attribute's collection. Note
				// that the path is extended in opposite order to the iteration order
				// of the loops: index into the collection, then the contained
				// attribute name. This is because we have one type
				// representing multiple collection elements.
				attrPath := copyAndExtendPath(path, cty.IndexStep{Key: idx}, cty.GetAttrStep{Name: name})

				var attrMarks []any
				if attrS.Sensitive {
					// If the entire attribute is sensitive, mark it so
					attrMarks = append(attrMarks, marks.Sensitive)
				}
				if attrS.Deprecated && subject != nil {
					// If the entire attribute is deprecated, mark it so
					attrMarks = append(attrMarks, deprecationMark(subject, attrPath, attrS.DeprecationMessage))
				}
				if len(attrMarks) != 0 {
					pvm = append(pvm, cty.PathValueMarks{
						Path:  attrPath,
						Marks: cty.NewValueMarks(attrMarks...),
					})
				} else {
					// The attribute has a nested type which contains sensitive
					// attributes, so recurse
					pvm = append(pvm, attrS.NestedType.ValueMarks(attrV, attrPath, subject)...)
				}
			}
		default:
			panic(fmt.Sprintf("unsupported nesting mode %s", attrS.NestedType.Nesting))
		}
	}
	return pvm
}

// RemoveEphemeralFromWriteOnly gets the value and for the attributes that are
// configured as write-only removes the marks.Ephemeral mark.
// Write-only arguments are only available in managed resources.
// Write-only arguments are the only managed resource's attribute type
// that can reference ephemeral values.
// Also, the provider framework sdk is responsible with nullify these attributes
// before returning back to OpenTofu.
//
// Therefore, before writing the changes/state of a managed resource to its store,
// we want to be sure that the nil value of the attribute is not marked as ephemeral
// in case it got its value from evaluating an expression where an ephemeral value has
// been involved.
func (b *Block) RemoveEphemeralFromWriteOnly(v cty.Value) cty.Value {
	unmarkedV, valMarks := v.UnmarkDeepWithPaths()
	for _, pathMark := range valMarks {
		if _, ok := pathMark.Marks[marks.Ephemeral]; !ok {
			continue
		}
		attr := b.AttributeByPath(pathMark.Path)
		if attr == nil {
			continue
		}
		if !attr.WriteOnly {
			continue
		}
		delete(pathMark.Marks, marks.Ephemeral)
	}
	return unmarkedV.MarkWithPaths(valMarks)
}
