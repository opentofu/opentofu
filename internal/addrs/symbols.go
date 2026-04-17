// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package addrs

func NewSymbolsAttr(name string) SymbolsAttr {
	return SymbolsAttr{
		Name: name,
	}
}

// SymbolsAttr is the address of an attribute of the "symbols" object in
// the interpolation scope
type SymbolsAttr struct {
	referenceable
	Name string
}

func (ta SymbolsAttr) String() string {
	return "symbols." + ta.Name
}

func (ta SymbolsAttr) UniqueKey() UniqueKey {
	return ta // A SymbolsAttr is its own UniqueKey
}

func (ta SymbolsAttr) uniqueKeySigil() {}
