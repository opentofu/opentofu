// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

//go:generate go tool golang.org/x/tools/cmd/stringer -type=InstanceType instancetype.go

// InstanceType is an enum of the various types of instances store in the State
type InstanceType int

const (
	TypeInvalid InstanceType = iota
	TypePrimary
	TypeTainted
	TypeDeposed
)
