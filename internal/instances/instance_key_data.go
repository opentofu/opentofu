// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package instances

import (
	"maps"

	"github.com/zclconf/go-cty/cty"
)

// RepetitionData represents the values available to identify individual
// repetitions of a particular object.
//
// This corresponds to the each.key, each.value, and count.index symbols in
// the configuration language.
type RepetitionData struct {
	// CountIndex is the value for count.index, or cty.NilVal if evaluating
	// in a context where the "count" argument is not active.
	//
	// For correct operation, this should always be of type cty.Number if not
	// nil.
	CountIndex cty.Value

	// EachKey and EachValue are the values for each.key and each.value
	// respectively, or cty.NilVal if evaluating in a context where the
	// "for_each" argument is not active. These must either both be set
	// or neither set.
	//
	// For correct operation, EachKey must always be either of type cty.String
	// or cty.Number if not nil.
	EachKey, EachValue cty.Value
}

// AllValueMarks returns a mark set containing the full set of marks across
// all values inside the receiver.
//
// Use this instead of accessing each field in turn because future changes
// might add new fields to [RepetitionData] which will be included in this
// function's result without having to update every caller.
func (rd *RepetitionData) AllValueMarks() cty.ValueMarks {
	var ret cty.ValueMarks
	addMore := func(v cty.Value) {
		// We make some effort here to avoid creating additional temporary
		// cty.ValueMarks values because [cty.Value.Marks] already makes it
		// copy itself anyway, so we'd prefer to just write into the set
		// it returned than to make yet another copy of it.
		marks := v.Marks()
		if len(marks) == 0 {
			return
		}
		if ret == nil {
			ret = marks
			return
		}
		maps.Copy(ret, marks)
	}
	addMore(rd.CountIndex)
	addMore(rd.EachKey)
	addMore(rd.EachValue)
	return ret
}
