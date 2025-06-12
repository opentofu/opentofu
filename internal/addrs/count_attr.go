// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import "github.com/zclconf/go-cty/cty"

// CountAttr is the address of an attribute of the "count" object in
// the interpolation scope, like "count.index".
type CountAttr struct {
	referenceable
	Name string
}

func (ca CountAttr) String() string {
	return "count." + ca.Name
}

func (ca CountAttr) Path() cty.Path {
	return cty.GetAttrPath("count").GetAttr(ca.Name)
}

func (ca CountAttr) UniqueKey() UniqueKey {
	return ca // A CountAttr is its own UniqueKey
}

func (ca CountAttr) uniqueKeySigil() {}
