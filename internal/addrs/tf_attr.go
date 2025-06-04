// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import "github.com/zclconf/go-cty/cty"

const (
	IdentTerraform = "terraform"
	IdentTofu      = "tofu"
)

func NewTerraformAttr(alias, name string) TerraformAttr {
	return TerraformAttr{
		Name:  name,
		Alias: alias,
	}
}

// TerraformAttr is the address of an attribute of the "terraform" and "tofu" object in
// the interpolation scope, like "terraform.workspace" and "tofu.workspace".
type TerraformAttr struct {
	referenceable
	Name  string
	Alias string
}

func (ta TerraformAttr) String() string {
	return ta.Alias + "." + ta.Name
}

func (ta TerraformAttr) Path() cty.Path {
	return cty.GetAttrPath(ta.Alias).GetAttr(ta.Name)
}

func (ta TerraformAttr) UniqueKey() UniqueKey {
	return ta // A TerraformAttr is its own UniqueKey
}

func (ta TerraformAttr) uniqueKeySigil() {}
