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

// HasDeprecated returns true if the cty.Value contains a deprecated mark.
func HasDeprecated(v cty.Value) bool {
	for m := range v.Marks() {
		if _, ok := m.(deprecationMark); ok {
			return true
		}
	}
	return false
}

// Contains returns true if the cty.Value or any value within it contains
// the given mark.
func Contains(val cty.Value, mark valueMark) bool {
	return val.HasMarkDeep(mark)
}

// ContainsAnyMark returns true if the cty.Value or any value within it contains
// any of the given mark.
func ContainsAnyMark(val cty.Value, marks ...valueMark) bool {
	ret := false
	// We never return an error, so we can ignore the value here
	_ = cty.Walk(val, func(_ cty.Path, v cty.Value) (bool, error) {
		for _, mark := range marks {
			if v.HasMark(mark) {
				ret = true
				return false, nil
			}
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
	_ tfdiags.Keyable                 = DeprecationCause{}
)

type DeprecationCause struct {
	module  string
	subject string
	message string
}

func DeprecationCauseResource(res addrs.AbsResourceInstance, path cty.Path, message string) DeprecationCause {
	return DeprecationCause{
		module:  res.Module.String(),
		subject: res.Resource.String() + tfdiags.FormatCtyPath(path),
		message: message,
	}
}
func DeprecationCauseOutput(out addrs.AbsOutputValue, message string) DeprecationCause {
	return DeprecationCause{
		module:  out.Module.String(),
		subject: out.OutputValue.Name,
		message: message,
	}
}
func DeprecationCauseVariable(vaddr addrs.AbsInputVariableInstance, message string) DeprecationCause {
	return DeprecationCause{
		module:  vaddr.Module.String(),
		subject: vaddr.Variable.Name,
		message: message,
	}
}

// ExtraInfoKey returns the key used for consolidation of deprecation diagnostics.
func (dc DeprecationCause) ExtraInfoKey() string {
	if dc.module == "" {
		return dc.subject
	}
	return dc.module + "." + dc.subject
}

func (dc DeprecationCause) ModuleInstance() addrs.ModuleInstance {
	if dc.module == "" {
		return addrs.RootModuleInstance // Root module
	}
	mod, _ := addrs.ParseModuleInstanceStr(dc.module)
	// This is best effort, we ignore diags here
	return mod
}

type deprecationMark struct {
	Cause DeprecationCause
}

func (m deprecationMark) GoString() string {
	return "marks.Deprecated"
}

func DeprecationMark(cause DeprecationCause) any {
	return deprecationMark{Cause: cause}
}

// Deprecated marks a given value as deprecated with specified DeprecationCause.
func Deprecated(v cty.Value, cause DeprecationCause) cty.Value {
	for m := range cty.ValueMarksOfType[deprecationMark](v) {
		if m.Cause.ExtraInfoKey() == cause.ExtraInfoKey() {
			// Already marked as deprecated for this cause.
			return v
		}
	}
	return v.Mark(deprecationMark{
		Cause: cause,
	})
}

// DeprecatedOutput marks a given value as deprecated constructing a DeprecationCause
// from module output specific data.
func DeprecatedOutput(v cty.Value, addr addrs.AbsOutputValue, msg string) cty.Value {
	if addr.Module.IsRoot() {
		// Marking a root output as deprecated has no impact.
		// We hit this case when using the test framework on a module however.
		// This is requried as the ModuleCallOutput() below will panic on the root module.
		return v
	}
	return Deprecated(v, DeprecationCauseOutput(addr, msg))
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
		var msg string
		if cause.message == "" {
			msg = fmt.Sprintf("This value is derived from %s, which is deprecated.", cause.ExtraInfoKey())
		} else {
			msg = fmt.Sprintf("This value is derived from %s, which is deprecated with the following message:\n\n%s", cause.ExtraInfoKey(), cause.message)
		}
		diag := tfdiags.AttributeValue(
			tfdiags.Warning,
			"Value derived from a deprecated source",
			msg,
			path,
		)
		diags = diags.Append(tfdiags.Override(diag, tfdiags.Warning, func() tfdiags.DiagnosticExtraWrapper {
			return &deprecatedDiagnosticExtra{
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
		var msg string
		if cause.message == "" {
			msg = fmt.Sprintf("%s is derived from %s, which is deprecated.", source, cause.ExtraInfoKey())
		} else {
			msg = fmt.Sprintf("%s is derived from %s, which is deprecated with the following message:\n\n%s", source, cause.ExtraInfoKey(), cause.message)
		}
		diags = diags.Append(&hcl.Diagnostic{
			Severity:   hcl.DiagWarning,
			Summary:    "Value derived from a deprecated source",
			Detail:     msg,
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
