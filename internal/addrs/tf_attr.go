// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

func NewTerraformAttr(alias, name string) TerraformAttr {
	return TerraformAttr{
		name:  name,
		alias: alias,
	}
}

// TerraformAttr is the address of an attribute of the "terraform" and "tofu" object in
// the interpolation scope, like "terraform.workspace" and "tofu.workspace".
type TerraformAttr struct {
	referenceable
	name  string
	alias string
}

func (ta TerraformAttr) Name() string {
	return ta.name
}

func (ta TerraformAttr) Alias() string {
	return ta.alias
}

func (ta TerraformAttr) String() string {
	return ta.alias + "." + ta.name
}

func (ta TerraformAttr) UniqueKey() UniqueKey {
	return ta // A TerraformAttr is its own UniqueKey
}

func (ta TerraformAttr) uniqueKeySigil() {}
