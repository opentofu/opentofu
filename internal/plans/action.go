// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plans

type Action rune

const (
	NoOp             Action = 0
	Create           Action = '+'
	Read             Action = '←'
	Update           Action = '~'
	DeleteThenCreate Action = '∓'
	CreateThenDelete Action = '±'
	Delete           Action = '-'
	Forget           Action = '.'
	ForgetThenCreate Action = '⊘'
	Open             Action = '⁐'
	// NOTE: Renew and Close missing on purpose.
	// Those are not meant to be stored in the plan.
	// Instead, we have hooks for those to show progress.
)

//go:generate go tool golang.org/x/tools/cmd/stringer -type Action

// IsReplace returns true if the action is one of the two actions that
// represents replacing an existing object with a new object:
// DeleteThenCreate or CreateThenDelete.
func (a Action) IsReplace() bool {
	return a == DeleteThenCreate || a == CreateThenDelete
}

// CanTriggerDownstreamReplace reports whether this action represents a change
// significant enough to trigger replacement of resources that reference this
// one via replace_triggered_by. This covers any action that produces a new or
// meaningfully changed object: fresh creation, update, any replace variant
// (including ForgetThenCreate for resources with lifecycle.destroy = false).
func (a Action) CanTriggerDownstreamReplace() bool {
	return a == Create || a == Update || a.IsReplace() || a == ForgetThenCreate
}
