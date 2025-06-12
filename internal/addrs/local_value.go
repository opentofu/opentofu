// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"fmt"

	"github.com/zclconf/go-cty/cty"
)

// LocalValue is the address of a local value.
type LocalValue struct {
	referenceable
	Name string
}

func (v LocalValue) String() string {
	return "local." + v.Name
}

func (v LocalValue) Path() cty.Path {
	return cty.GetAttrPath("local").GetAttr(v.Name)
}

// Equal is primarily here for go-cmp to use. Use the == operator directly in
// normal code, because LocalValue is naturally comparable.
func (v LocalValue) Equal(other LocalValue) bool {
	return v == other
}

func (v LocalValue) UniqueKey() UniqueKey {
	return v // A LocalValue is its own UniqueKey
}

func (v LocalValue) uniqueKeySigil() {}

// Absolute converts the receiver into an absolute address within the given
// module instance.
func (v LocalValue) Absolute(m ModuleInstance) AbsLocalValue {
	return AbsLocalValue{
		Module:     m,
		LocalValue: v,
	}
}

// AbsLocalValue is the absolute address of a local value within a module instance.
type AbsLocalValue struct {
	Module     ModuleInstance
	LocalValue LocalValue
}

// LocalValue returns the absolute address of a local value of the given
// name within the receiving module instance.
func (m ModuleInstance) LocalValue(name string) AbsLocalValue {
	return AbsLocalValue{
		Module: m,
		LocalValue: LocalValue{
			Name: name,
		},
	}
}

func (v AbsLocalValue) String() string {
	if len(v.Module) == 0 {
		return v.LocalValue.String()
	}
	return fmt.Sprintf("%s.%s", v.Module.String(), v.LocalValue.String())
}
