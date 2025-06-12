// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import "github.com/zclconf/go-cty/cty"

// Self is the address of the special object "self" that behaves as an alias
// for a containing object currently in scope.
const Self selfT = 0

type selfT int

var selfPath = [...]cty.PathStep{cty.GetAttrStep{Name: "self"}}

func (s selfT) referenceableSigil() {
}

func (s selfT) String() string {
	return "self"
}

func (s selfT) Path() cty.Path {
	return selfPath[:] // no excess capacity
}

func (s selfT) UniqueKey() UniqueKey {
	return Self // Self is its own UniqueKey
}

func (s selfT) uniqueKeySigil() {}
