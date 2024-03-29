// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package differ

// NestingMode is a wrapper around a string type to describe the various
// different kinds of nesting modes that can be applied to nested blocks and
// objects.
type NestingMode string

const (
	nestingModeSet    NestingMode = "set"
	nestingModeList   NestingMode = "list"
	nestingModeMap    NestingMode = "map"
	nestingModeSingle NestingMode = "single"
	nestingModeGroup  NestingMode = "group"
)
