// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"github.com/zclconf/go-cty/cty"
)

type ctyMark rune

// deferredMark is a cty Mark we use to represent that a value is derived from
// something whose planning has been deferred to a later plan/apply round
// for any reason.
//
// We use this to recognize when a resource instance wouldn't need to be
// deferred itself except that its configuration is based on something else that
// was previously deferred, and therefore the downstream must be transitively
// deferred to because whatever outcome it's relying on won't actually happen
// in the current plan/apply round.
//
// Using marks for this means that our analysis of deferrals is based on
// dynamic analysis, so and e.g. a conditional expression where only one arm
// is derived from something deferred will only be treated as deferred if
// that arm were selected.
const deferredMark = ctyMark('…')

// deferredVal returns a value equivalent to the given value except that
// the result and anything derived from it would cause [derivedFromDeferredVal]
// to return true.
func deferredVal(v cty.Value) cty.Value {
	return v.Mark(deferredMark)
}

// derivedFromDeferredVal returns true if any part of the given value is
// derived from something that was previously produced by a call to
// deferredVal.
func derivedFromDeferredVal(v cty.Value) bool {
	return v.HasMarkDeep(deferredMark)
}
