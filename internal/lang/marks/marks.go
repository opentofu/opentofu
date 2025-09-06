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
	"github.com/zclconf/go-cty/cty/ctymarks"

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
	return val.HasMarkDeep(mark)
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
	for m := range cty.ValueMarksOfType[deprecationMark](v) {
		if addrs.Equivalent(m.Cause.By, cause.By) {
			// Already marked as deprecated for this cause.
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
	if addr.Module.IsRoot() {
		// Marking a root output as deprecated has no impact.
		// We hit this case when using the test framework on a module however.
		// This is requried as the ModuleCallOutput() below will panic on the root module.
		return v
	}
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
	// WrangleMarksDeep calls the callback for each mark it finds anywhere
	// in the value, and then produces a modified value only if we return
	// a non-nil [ctymarks.WrangleAction]. This means it's relatively fast
	// in the common case where there are no deprecation marks, since
	// then we don't ask to make any changes at all.
	//
	// We ignore the error result because our callback never returns a non-nil
	// error.
	ret, _ := val.WrangleMarksDeep(func(mark any, path cty.Path) (ctymarks.WrangleAction, error) {
		dm, ok := mark.(deprecationMark)
		if !ok {
			return nil, nil // no changes to non-deprecation marks
		}
		cause := dm.Cause
		diag := tfdiags.AttributeValue(
			tfdiags.Warning,
			"Value derived from a deprecated source",
			fmt.Sprintf("This value is derived from %v, which is deprecated with the following message:\n\n%s", cause.By, cause.Message),
			path,
		)
		diags = diags.Append(tfdiags.Override(diag, tfdiags.Warning, func() tfdiags.DiagnosticExtraWrapper {
			return &deprecatedOutputDiagnosticExtra{
				Cause: cause,
			}
		}))
		return ctymarks.WrangleDrop, nil // discard any deprecation marks
	})
	return ret, diags.InConfigBody(body, "")
}

// ExtractDeprecatedDiagnosticsWithExpr composes deprecation diagnostics based on deprecation marks inside
// the cty.Value. It uses hcl.Expression to properly reference deprecated attributes in final diagnostics.
// The returned cty.Value has no deprecation marks inside, since all the relevant diagnostics has been collected.
func ExtractDeprecatedDiagnosticsWithExpr(val cty.Value, expr hcl.Expression) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	// WrangleMarksDeep calls the callback for each mark it finds anywhere
	// in the value, and then produces a modified value only if we return
	// a non-nil [ctymarks.WrangleAction]. This means it's relatively fast
	// in the common case where there are no deprecation marks, since
	// then we don't ask to make any changes at all.
	//
	// We ignore the error result because our callback never returns a non-nil
	// error.
	ret, _ := val.WrangleMarksDeep(func(mark any, path cty.Path) (ctymarks.WrangleAction, error) {
		dm, ok := mark.(deprecationMark)
		if !ok {
			return nil, nil // no changes to non-deprecation marks
		}
		attr := strings.TrimPrefix(tfdiags.FormatCtyPath(path), ".")
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
			Extra:      cause,
		})
		return ctymarks.WrangleDrop, nil // discard any deprecation marks
	})
	return ret, diags
}

func RemoveDeepDeprecated(val cty.Value) cty.Value {
	// Okay to ignore the error result because the callback never returns errors.
	ret, _ := val.WrangleMarksDeep(func(mark any, path cty.Path) (ctymarks.WrangleAction, error) {
		if _, ok := mark.(deprecationMark); ok {
			return ctymarks.WrangleDrop, nil // discard any deprecation marks
		}
		return nil, nil // no changes to any other marks
	})
	return ret
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
