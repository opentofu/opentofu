// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package schema

//go:generate go tool golang.org/x/tools/cmd/stringer -type=ValueType valuetype.go

// ValueType is an enum of the type that can be represented by a schema.
type ValueType int

const (
	TypeInvalid ValueType = iota
	TypeBool
	TypeInt
	TypeFloat
	TypeString
	TypeList
	TypeMap
	TypeSet
	typeObject
)

// NOTE: ValueType has more functions defined on it in schema.go. We can't
// put them here because we reference other files.
