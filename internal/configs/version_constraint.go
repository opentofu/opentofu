// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"

	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// VersionConstraint represents a version constraint on some resource
// (e.g. OpenTofu Core, a provider, a module, ...) that carries with it
// a source range so that a helpful diagnostic can be printed in the event
// that a particular constraint does not match.
type VersionConstraint struct {
	Required  version.Constraints
	DeclRange hcl.Range
}

func decodeVersionConstraint(attr *hcl.Attribute) (VersionConstraint, hcl.Diagnostics) {
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		return VersionConstraint{}, diags
	}
	return decodeVersionConstraintValue(attr, val)
}

func decodeVersionConstraintValue(attr *hcl.Attribute, val cty.Value) (VersionConstraint, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	ret := VersionConstraint{
		DeclRange: attr.Range,
	}

	if val.HasMark(marks.Sensitive) {
		return ret, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid version constraint",
			Detail:   fmt.Sprintf("Sensitive values, or values derived from sensitive values, cannot be used as %s arguments.", attr.Name),
			Subject:  attr.Expr.Range().Ptr(),
		})
	}
	if val.HasMark(marks.Ephemeral) {
		return ret, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid version constraint",
			Detail:   fmt.Sprintf("Ephemeral values, or values derived from ephemeral values, cannot be used as %s arguments.", attr.Name),
			Subject:  attr.Expr.Range().Ptr(),
		})
	}

	var err error
	val, err = convert.Convert(val, cty.String)
	if err != nil {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid version constraint",
			Detail:   fmt.Sprintf("A string value is required for %s.", attr.Name),
			Subject:  attr.Expr.Range().Ptr(),
		})
		return ret, diags
	}

	if val.IsNull() {
		// A null version constraint is strange, but we'll just treat it
		// like an empty constraint set.
		return ret, diags
	}

	if !val.IsWhollyKnown() {
		// If there is a syntax error, HCL sets the value of the given attribute
		// to cty.DynamicVal. A diagnostic for the syntax error will already
		// bubble up, so we will move forward gracefully here.
		return ret, diags
	}

	constraintStr := val.AsString()
	constraints, err := version.NewConstraint(constraintStr)
	if err != nil {
		// NewConstraint doesn't return user-friendly errors, so we'll just
		// ignore the provided error and produce our own generic one.
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid version constraint",
			Detail:   "This string does not use correct version constraint syntax.", // Not very actionable :(
			Subject:  attr.Expr.Range().Ptr(),
		})
		return ret, diags
	}

	ret.Required = constraints
	return ret, diags
}
