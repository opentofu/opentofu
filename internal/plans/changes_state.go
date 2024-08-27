// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plans

import (
	"github.com/terramate-io/opentofulib/internal/states"
)

// PlannedState merges the set of changes described by the receiver into the
// given prior state to produce the planned result state.
//
// The result is an approximation of the state as it would exist after
// applying these changes, omitting any values that cannot be determined until
// the changes are actually applied.
func (c *Changes) PlannedState(prior *states.State) (*states.State, error) {
	panic("Changes.PlannedState not yet implemented")
}
