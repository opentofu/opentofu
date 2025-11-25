// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"github.com/zclconf/go-cty/cty"
)

// ManagedResourceObjectFinalPlan represents a final plan -- ready to actually
// be applied -- for some managed resource instance object that could be any of
// a current object for a desired resource instance, a current object for
// an orphan resource instance, or a deposed object for any resource
// instance.
//
// Note that for execution graph purposes a "replace" action is always
// represented as two separate "final plans", where the "delete" leg is
// represented by the configuration being null and the "create" leg is
// represented by the prior state being null. This struct type intentionally
// does not carry any information about the identity of the object the
// plan is for because that is implied by the relationships in the graph and
// there should be no assumptions about e.g. there being exactly one final
// plan per resource instance, etc.
type ManagedResourceObjectFinalPlan struct {
	// ResourceType is the resource type of the object this plan is for, as
	// would be understood by the provider that generated this plan.
	ResourceType string

	// ConfigVal is the value representing the configuration for this
	// object, but only if it's a "desired" object. This is always a null
	// value for "orphan" instances and deposed objects, because they have
	// no configuration by definition.
	ConfigVal cty.Value
	// PriorStateVal is the value representing this object in the prior
	// state, or a null value if this object didn't previously exist and
	// is therefore presumably being created.
	PriorStateVal cty.Value
	// PlannedVal is the value returned by the provider when it was asked
	// to produce a plan. This is an approximation of the final result
	// with unknown values as placeholders for anything that won't be known
	// until after the change has been applied.
	PlannedVal cty.Value
	// TODO: The "Private" value that the provider returned in its planning
	// response.
	// TODO: Anything else we'd need to populate an "ApplyResourceChanges"
	// request to the associated provider.
}
