// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

// BackendAttr is the address of an attribute of the "backend" object in
// the interpolation scope, like "backend.<config_attr>".
type BackendAttr struct {
	referenceable
	Type string
}

func (ba BackendAttr) String() string {
	return "backend." + ba.Type
}

func (ba BackendAttr) UniqueKey() UniqueKey {
	return ba // A Backend is its own UniqueKey
}

func (ba BackendAttr) uniqueKeySigil() {}
