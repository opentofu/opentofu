// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package marks

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
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
	return "marks.Deprecated"
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

// DeprecatedDiagnosticsInBody composes deprecation diagnostics based on deprecation marks inside
// the cty.Value. It uses hcl.Body to properly reference deprecated attributes in final diagnostics.
func DeprecatedDiagnosticsInBody(val cty.Value, body hcl.Body) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	unmarked, pathMarks := val.UnmarkDeepWithPaths()

	// Locate deprecationMarks and filter them out
	for _, pm := range pathMarks {
		for m := range pm.Marks {
			dm, ok := m.(deprecationMark)
			if !ok {
				continue
			}

			// Remove mark
			delete(pm.Marks, m)

			cause := dm.Cause
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Warning,
				"Value derived from a deprecated source",
				fmt.Sprintf("This value is derived from %v, which is deprecated with the following message:\n\n%s", cause.By, cause.Message),
				pm.Path,
			))
		}
		// TODO If all marks are removed, should we remove it from pathMarks?
	}

	return unmarked.MarkWithPaths(pathMarks), diags.InConfigBody(body, "")
}

// DeprecatedDiagnosticsInExpr composes deprecation diagnostics based on deprecation marks inside
// the cty.Value. It uses hcl.Expression to properly reference deprecated attributes in final diagnostics.
func DeprecatedDiagnosticsInExpr(val cty.Value, expr hcl.Expression) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	unmarked, pathMarks := val.UnmarkDeepWithPaths()

	// Locate deprecationMarks and filter them out
	for _, pm := range pathMarks {
		for m := range pm.Marks {
			dm, ok := m.(deprecationMark)
			if !ok {
				continue
			}

			// Remove mark
			delete(pm.Marks, m)

			attr := tfdiags.FormatCtyPath(pm.Path)
			// FormatCtyPath call could result in ".fieldA.fieldB" in some
			// cases, so we want to remove the first dot for a friendlier message.
			if len(attr) > 1 && attr[0] == '.' {
				attr = attr[1:]
			}

			source := "This value"
			if attr != "" {
				source += "'s attribute " + attr
			}

			cause := dm.Cause
			diags = diags.Append(&hcl.Diagnostic{
				Severity:   hcl.DiagWarning,
				Summary:    "Value derived from a deprecated source",
				Detail:     fmt.Sprintf("%s is derived from %v, which is deprecated with the following message:\n\n%s", source, cause.By, cause.Message),
				Subject:    expr.Range().Ptr(),
				Expression: expr,
			})

		}
		// TODO If all marks are removed, should we remove it from pathMarks?
	}

	return unmarked.MarkWithPaths(pathMarks), diags
}
