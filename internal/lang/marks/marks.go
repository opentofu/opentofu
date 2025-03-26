// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package marks

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/zclconf/go-cty/cty"
)

// valueMarks allow creating strictly typed values for use as cty.Value marks.
// Each distinct mark value must be a constant in this package whose value
// is a valueMark whose underlying string matches the name of the variable.
type valueMark string

func (m valueMark) GoString() string {
	return "marks." + string(m)
}

// Has returns true if and only if the cty.Value has the given mark.
func Has(val cty.Value, mark valueMark) bool {
	return val.HasMark(mark)
}

// Contains returns true if the cty.Value or any any value within it contains
// the given mark.
func Contains(val cty.Value, mark valueMark) bool {
	ret := false
	cty.Walk(val, func(_ cty.Path, v cty.Value) (bool, error) {
		if v.HasMark(mark) {
			ret = true
			return false, nil
		}
		return true, nil
	})
	return ret
}

// Sensitive indicates that this value is marked as sensitive in the context of
// OpenTofu.
const Sensitive = valueMark("Sensitive")

// TypeType is used to indicate that the value contains a representation of
// another value's type. This is part of the implementation of the console-only
// `type` function.
const TypeType = valueMark("TypeType")

type DeprecationCause struct {
	By      addrs.Referenceable
	Message string
}

type deprecationMark struct {
	Cause DeprecationCause
}

func (m deprecationMark) GoString() string {
	return "marks." + string("Deprecated")
}

// Deprecated marks a given value as deprecated with specified DeprecationCause.
func Deprecated(v cty.Value, cause DeprecationCause) cty.Value {
	for m := range v.Marks() {
		dm, ok := m.(deprecationMark)
		if !ok {
			continue
		}

		// Already marked as deprecated for this cause.
		if addrs.Equivalent(dm.Cause.By, cause.By) {
			return v
		}
	}

	return v.Mark(deprecationMark{
		Cause: cause,
	})
}

// DeprecatedOutput marks a given values as deprecated constructing a DeprecationCause
// from module output specific data.
func DeprecatedOutput(v cty.Value, addr addrs.AbsOutputValue, msg string) cty.Value {
	_, callOutAddr := addr.ModuleCallOutput()
	return Deprecated(v, DeprecationCause{
		By:      callOutAddr,
		Message: msg,
	})
}

// ContainsDeprecated returns true if the cty.Value or any any value within it
// contains the deprecation mark.
func ContainsDeprecated(v cty.Value) bool {
	_, marks := v.UnmarkDeep()

	for m := range marks {
		if _, ok := m.(deprecationMark); ok {
			return true
		}
	}

	return false
}

// ListDeprecationCauses iterates over all the marks for a given value to
// extract all the DeprecationCauses. A single cty.Value could be constructed
// from a multiple references to deprecated values, so this is a list.
func ListDeprecationCauses(v cty.Value) []DeprecationCause {
	var causes []DeprecationCause

	_, marks := v.UnmarkDeep()

	for m := range marks {
		dm, ok := m.(deprecationMark)
		if !ok {
			continue
		}

		causes = append(causes, dm.Cause)
	}

	return causes
}
