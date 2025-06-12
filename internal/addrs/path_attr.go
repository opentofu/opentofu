// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import "github.com/zclconf/go-cty/cty"

// PathAttr is the address of an attribute of the "path" object in
// the interpolation scope, like "path.module".
type PathAttr struct {
	referenceable
	Name string
}

func (pa PathAttr) String() string {
	return "path." + pa.Name
}

func (pa PathAttr) Path() cty.Path {
	return cty.GetAttrPath("path").GetAttr(pa.Name)
}

func (pa PathAttr) UniqueKey() UniqueKey {
	return pa // A PathAttr is its own UniqueKey
}

func (pa PathAttr) uniqueKeySigil() {}
