// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configschema

type FilterT[T any] func(string, T) bool

var (
	FilterReadOnlyAttribute = func(name string, attribute *Attribute) bool {
		return attribute.Computed && !attribute.Optional
	}

	FilterHelperSchemaIdAttribute = func(name string, attribute *Attribute) bool {
		if name == "id" && attribute.Computed && attribute.Optional {
			return true
		}
		return false
	}

	FilterDeprecatedAttribute = func(name string, attribute *Attribute) bool {
		return attribute.Deprecated
	}

	FilterDeprecatedBlock = func(name string, block *NestedBlock) bool {
		return block.Deprecated
	}
)

func FilterOr[T any](filters ...FilterT[T]) FilterT[T] {
	return func(name string, value T) bool {
		for _, f := range filters {
			if f(name, value) {
				return true
			}
		}
		return false
	}
}

func (b *Block) Filter(filterAttribute FilterT[*Attribute], filterBlock FilterT[*NestedBlock]) *Block {
	ret := &Block{
		Description:     b.Description,
		DescriptionKind: b.DescriptionKind,
		Deprecated:      b.Deprecated,
	}

	if b.Attributes != nil {
		ret.Attributes = make(map[string]*Attribute, len(b.Attributes))
	}
	for name, attrS := range b.Attributes {
		// Copy the attributes of the block. Otherwise, if the filterNestedType is filtering out some attributes,
		// the underlying schema is getting altered too, rendering the providers.SchemaCache invalid.
		attr := *attrS
		if filterAttribute == nil || !filterAttribute(name, &attr) {
			ret.Attributes[name] = &attr
		}

		if attr.NestedType != nil {
			ret.Attributes[name].NestedType = filterNestedType((&attr).NestedType, filterAttribute)
		}
	}

	if b.BlockTypes != nil {
		ret.BlockTypes = make(map[string]*NestedBlock, len(b.BlockTypes))
	}
	for name, blockS := range b.BlockTypes {
		if filterBlock == nil || !filterBlock(name, blockS) {
			block := blockS.Filter(filterAttribute, filterBlock)
			ret.BlockTypes[name] = &NestedBlock{
				Block:    *block,
				Nesting:  blockS.Nesting,
				MinItems: blockS.MinItems,
				MaxItems: blockS.MaxItems,
			}
		}
	}

	return ret
}

func filterNestedType(obj *Object, filterAttribute FilterT[*Attribute]) *Object {
	if obj == nil {
		return nil
	}

	ret := &Object{
		Attributes: map[string]*Attribute{},
		Nesting:    obj.Nesting,
	}

	for name, attrS := range obj.Attributes {
		// Copy the attributes of the block. Otherwise, if the filterNestedType is filtering out some attributes,
		// the underlying schema is getting altered too, rendering the providers.SchemaCache invalid.
		attr := *attrS
		if filterAttribute == nil || !filterAttribute(name, &attr) {
			ret.Attributes[name] = &attr
			if attr.NestedType != nil {
				ret.Attributes[name].NestedType = filterNestedType(attr.NestedType, filterAttribute)
			}
		}
	}

	return ret
}
