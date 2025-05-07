// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plans

// Mode represents the various mutually-exclusive modes for creating a plan.
type Mode rune

//go:generate go run golang.org/x/tools/cmd/stringer -type Mode

const (
	// NormalMode is the default planning mode, which aims to synchronize the
	// prior state with remote objects and plan a set of actions intended to
	// make those remote objects better match the current configuration.
	NormalMode Mode = 0

	// DestroyMode is a special planning mode for situations where the goal
	// is to destroy all remote objects that are bound to instances in the
	// prior state, even if the configuration for those instances is still
	// present.
	//
	// This mode corresponds with the "-destroy" option to "tofu plan",
	// and with the plan created by the "tofu destroy" command.
	DestroyMode Mode = 'D'

	// RefreshOnlyMode is a special planning mode which only performs the
	// synchronization of prior state with remote objects, and skips any
	// effort to generate any change actions for resource instances even if
	// the configuration has changed relative to the state.
	//
	// This mode corresponds with the "-refresh-only" option to
	// "tofu plan".
	RefreshOnlyMode Mode = 'R'
)

// UIName returns a name suitable for describing the mode in the UI.
func (m Mode) UIName() string {
	switch m {
	case NormalMode:
		return "normal"
	case DestroyMode:
		return "destroy"
	case RefreshOnlyMode:
		return "refresh-only"
	default:
		// Should not get here because the cases above should cover every
		// valid value of this type.
		return "unknown"
	}
}
