// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package backend

//go:generate go tool golang.org/x/tools/cmd/stringer -type=OperationType operation_type.go

// OperationType is an enum used with Operation to specify the operation
// type to perform for OpenTofu.
type OperationType uint

const (
	OperationTypeInvalid OperationType = iota
	OperationTypeRefresh
	OperationTypePlan
	OperationTypeApply
)
