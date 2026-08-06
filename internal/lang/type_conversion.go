// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// TypeConversionConstraint describes the intended result of a type conversion,
// typically specified by an author using the OpenTofu language's type
// constraint syntax.
type TypeConversionConstraint struct {
	// ConvertTarget is the conversion target type to pass to cty's
	// [convert.Convert] for the main type conversion step.
	ConvertTarget cty.Type

	// DefaultAttrVals represents any default values provided for optional
	// attributes in ConvertTarget, to be applied using
	// [typeexpr.Defaults.Apply] as part of the overall conversion process.
	//
	// This can be nil for a conversion target that does not include any
	// optional attributes.
	DefaultAttrVals *typeexpr.Defaults
}

// NewTypeConversionConstraint constructs a [TypeConversionConstraint] directly
// from its component parts.
//
// This is here only to accommodate some historical constraints of the config
// loader in "package configs", which prefers to store the two component parts
// separately in its representation of an input variable instead of directly
// storing a TypeConversionConstraint.
func NewTypeConversionConstraint(convertTarget cty.Type, defaultAttrVals *typeexpr.Defaults) TypeConversionConstraint {
	return TypeConversionConstraint{
		ConvertTarget:   convertTarget,
		DefaultAttrVals: defaultAttrVals,
	}
}

// ParseTypeConversionConstraint attempts to analyze the given expression to
// interpret it as the target type constraint for a type conversion.
//
// This function implements the language used in the "type" argument of a
// "variable" block in the OpenTofu language, and any other language feature
// that performs type conversion to an author-specified type.
func ParseTypeConversionConstraint(expr hcl.Expression) (TypeConversionConstraint, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ty, typeDefaults, hclDiags := typeexpr.TypeConstraintWithDefaults(expr)
	diags = diags.Append(hclDiags)

	return TypeConversionConstraint{
		ConvertTarget:   ty,
		DefaultAttrVals: typeDefaults,
	}, diags
}

// ConvertValue attempts to convert the given value to conform to the constraint
// represented by the receiver.
//
// This function just encapsulates the required interaction between
// [convert.Convert] and [typeexpr.Defaults.Apply] to ensure that type
// conversion always treats both target type and default attribute values
// consistently across all language features that perform conversion to an
// author-specified type constraint.
//
// If this function returns an error then it may be a [cty.PathError] describing
// a problem nested somewhere inside the given complex value. If presenting the
// error as part of a diagnostic message, use [tfdiags.FormatError] to include
// a user-friendly version of the relevant path, if any.
func (c TypeConversionConstraint) ConvertValue(val cty.Value) (cty.Value, error) {
	// If the type constraint has defaults, we must apply those
	// defaults to the variable default value before type conversion,
	// unless the default value is null. Null is excluded from the
	// type default application process as a special case, to allow
	// nullable variables to have a null default value.
	if c.DefaultAttrVals != nil && !val.IsNull() {
		val = c.DefaultAttrVals.Apply(val)
	}
	return convert.Convert(val, c.ConvertTarget)
}
