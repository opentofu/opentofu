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

	// DecisionMarks describes any cty marks that were present on values that
	// were directly involved in the decision to include the instance that this
	// RepetitionData object belongs to.
	//
	// For example, in the case of for_each with a map this would include any
	// marks from the map itself but not marks from the elements inside the map.
	// The elements' own marks would appear on EachValue instead.
	//
	// NOTE: The old language runtime in "package tofu" does not populate or
	// make use of this field. It is used only by the new runtime's evaluator,
	// in the packages under "internal/lang/eval".
	DecisionMarks cty.ValueMarks
}

// HasSymbolValues returns true if any of the fields that correspond to
// additional symbols that would describe the current item in a repetition
// are set to non-nil values.
//
// This is a somewhat-imprecise signal for "is using repetition at all", since
// in the singleton or conditional enabled cases none of the value fields are
// populated, but this should only be used for debug-related information like
// logging and not for making any real behavioral decisions.
func (rd *RepetitionData) HasSymbolValues() bool {
	return rd != nil && rd.CountIndex != cty.NilVal || rd.EachKey != cty.NilVal || rd.EachValue != cty.NilVal
}

// AllValueMarks returns a mark set containing the full set of marks across
// all values inside the receiver.
//
// Use this instead of accessing each field in turn because future changes
// might add new fields to [RepetitionData] which will be included in this
// function's result without having to update every caller.
//
// Note that this only reports marks directly on the enclosed values and not
// marks on values nested within them.
func (rd *RepetitionData) AllValueMarks() cty.ValueMarks {
	if rd == nil {
		return nil
	}
	var ret cty.ValueMarks
	if len(rd.DecisionMarks) != 0 {
		ret = make(cty.ValueMarks)
		maps.Copy(ret, rd.DecisionMarks)
	}
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
