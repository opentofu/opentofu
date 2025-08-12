// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package marks

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
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
	// We never return an error, so we can ignore the value here
	_ = cty.Walk(val, func(_ cty.Path, v cty.Value) (bool, error) {
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

// Ephemeral indicates that this value is marked as ephemeral in the context of
// OpenTofu.
const Ephemeral = valueMark("Ephemeral")

// TypeType is used to indicate that the value contains a representation of
// another value's type. This is part of the implementation of the console-only
// `type` function.
const TypeType = valueMark("TypeType")

var (
	_ diagnosticExtraDeprecationCause = DeprecationCause{}
)

type DeprecationCause struct {
	By      addrs.Referenceable
	Key     string
	Message string

	// IsFromRemoteModule indicates if the cause of deprecation is coming from a remotely
	// imported module relative to the root module.
	// This is useful when the user wants to control the type of deprecation warnings OpenTofu will show.
	IsFromRemoteModule bool
}

// ExtraInfoKey returns the key used for consolidation of deprecation diagnostics.
func (dc DeprecationCause) ExtraInfoKey() string {
	return dc.Key
}

type deprecationMark struct {
	Cause DeprecationCause
}

func (m deprecationMark) GoString() string {
	return "marks.Deprecated"
}

func HasDeprecated(v cty.Value) bool {
	for m := range v.Marks() {
		if _, ok := m.(deprecationMark); ok {
			return true
		}
	}
	return false
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
func DeprecatedOutput(v cty.Value, addr addrs.AbsOutputValue, msg string, isFromRemoteModule bool) cty.Value {
	_, callOutAddr := addr.ModuleCallOutput()
	return Deprecated(v, DeprecationCause{
		IsFromRemoteModule: isFromRemoteModule,
		By:                 callOutAddr,
		// Used to identify the output on the consolidation diagnostics and
		// make sure they are consolidated correctly. We use:
		// output value + deprecated message as the key
		Key:     fmt.Sprintf("%s\n%s", addr.OutputValue.Name, msg),
		Message: msg,
	})
}

// ExtractDeprecationDiagnosticsWithBody composes deprecation diagnostics based on deprecation marks inside
// the cty.Value. It uses hcl.Body to properly reference deprecated attributes in final diagnostics.
// The returned cty.Value has no deprecation marks inside, since all the relevant diagnostics has been collected.
func ExtractDeprecationDiagnosticsWithBody(val cty.Value, body hcl.Body) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	val, deprecatedPathMarks := unmarkDeepWithPathsDeprecated(val)

	for _, pm := range deprecatedPathMarks {
		for m := range pm.Marks {
			cause := m.(deprecationMark).Cause
			diag := tfdiags.AttributeValue(
				tfdiags.Warning,
				"Value derived from a deprecated source",
				fmt.Sprintf("This value is derived from %v, which is deprecated with the following message:\n\n%s", cause.By, cause.Message),
				pm.Path,
			)
			diags = diags.Append(tfdiags.Override(diag, tfdiags.Warning, func() tfdiags.DiagnosticExtraWrapper {
				return &deprecatedOutputDiagnosticExtra{
					Cause: cause,
				}
			}))
		}
	}

	return val, diags.InConfigBody(body, "")
}

// ExtractDeprecatedDiagnosticsWithExpr composes deprecation diagnostics based on deprecation marks inside
// the cty.Value. It uses hcl.Expression to properly reference deprecated attributes in final diagnostics.
// The returned cty.Value has no deprecation marks inside, since all the relevant diagnostics has been collected.
func ExtractDeprecatedDiagnosticsWithExpr(val cty.Value, expr hcl.Expression) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	val, deprecatedPathMarks := unmarkDeepWithPathsDeprecated(val)

	// Locate deprecationMarks and filter them out
	for _, pm := range deprecatedPathMarks {
		for m := range pm.Marks {
			attr := strings.TrimPrefix(tfdiags.FormatCtyPath(pm.Path), ".")
			source := "This value"
			if attr != "" {
				source += "'s attribute " + attr
			}

			cause := m.(deprecationMark).Cause
			diags = diags.Append(&hcl.Diagnostic{
				Severity:   hcl.DiagWarning,
				Summary:    "Value derived from a deprecated source",
				Detail:     fmt.Sprintf("%s is derived from %v, which is deprecated with the following message:\n\n%s", source, cause.By, cause.Message),
				Subject:    expr.Range().Ptr(),
				Expression: expr,
				Extra:      cause,
			})
		}
	}

	return val, diags
}

// unmarkDeepWithPathsDeprecated removes all deprecation marks from a value and returns them separately.
// It returns both the input value with all deprecation marks removed whilst preserving other marks, and a slice of PathValueMarks where marks were removed.
func unmarkDeepWithPathsDeprecated(val cty.Value) (cty.Value, []cty.PathValueMarks) {
	unmarked, pathMarks := val.UnmarkDeepWithPaths()

	var deprecationMarks []cty.PathValueMarks
	var filteredPathMarks []cty.PathValueMarks

	// Locate deprecationMarks and filter them out
	for _, pm := range pathMarks {
		deprecationPM := cty.PathValueMarks{
			Path:  pm.Path,
			Marks: make(cty.ValueMarks),
		}

		for m := range pm.Marks {
			_, ok := m.(deprecationMark)
			if !ok {
				continue
			}

			// Remove deprecated mark from value marks
			delete(pm.Marks, m)

			// Add mark to deprecation marks to keep track of what we're removing
			deprecationPM.Marks[m] = struct{}{}
		}

		if len(pm.Marks) > 0 {
			filteredPathMarks = append(filteredPathMarks, pm)
		}

		if len(deprecationPM.Marks) != 0 {
			deprecationMarks = append(deprecationMarks, deprecationPM)
		}
	}

	return unmarked.MarkWithPaths(filteredPathMarks), deprecationMarks
}

func RemoveDeepDeprecated(val cty.Value) cty.Value {
	val, _ = unmarkDeepWithPathsDeprecated(val)
	return val
}

// EnsureNoEphemeralMarks checks all the given paths for the Ephemeral mark.
// If there is at least one path marked as such, this method will return
// an error containing the marked paths.
func EnsureNoEphemeralMarks(pvms []cty.PathValueMarks) error {
	var res []string
	for _, pvm := range pvms {
		if _, ok := pvm.Marks[Ephemeral]; ok {
			res = append(res, tfdiags.FormatCtyPath(pvm.Path))
		}
	}

	if len(res) > 0 {
		return fmt.Errorf("ephemeral marks found at the following paths:\n%s", strings.Join(res, "\n"))
	}
	return nil
}
