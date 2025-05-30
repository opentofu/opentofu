// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package mcpconfigs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/configs/configschema"
)

type Schema struct {
	Attributes map[string]*Attribute

	DeclRange hcl.Range
}

type Attribute struct {
	Name           WithRange[string]
	TypeConstraint WithRange[cty.Type]
	DefaultValue   cty.Value
	Role           AttributeRole

	DeclRange hcl.Range
}

func (s *Schema) AsConfigSchema() *configschema.Block {
	ret := &configschema.Block{
		Attributes: make(map[string]*configschema.Attribute, len(s.Attributes)),
	}
	for name, attr := range s.Attributes {
		retAttr := &configschema.Attribute{
			Type: attr.TypeConstraint.Value,
		}
		switch attr.Role {
		case ArgumentAttribute:
			if attr.DefaultValue != cty.NilVal {
				retAttr.Required = true
			} else {
				retAttr.Optional = true
			}
		case ResultAttribute:
			retAttr.Computed = true
		default:
			panic(fmt.Sprintf("invalid AttributeRole value %d", attr.Role))
		}
		ret.Attributes[name] = retAttr
	}
	return ret
}

type AttributeRole int

const (
	ArgumentAttribute AttributeRole = iota
	ResultAttribute
)
