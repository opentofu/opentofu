// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"fmt"
	"reflect"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/customdecode"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// TypeConversionConstraint describes the intended result of a type conversion,
// typically specified by an author using the OpenTofu language's type
// constraint syntax.
//
// To accept an object of this type from a parameter to a built-in function in
// the OpenTofu language, declare the parameter as expecting
// [TypeConversionType]. That capsule type has a custom decode implementation
// that expects the expression syntax implemented by
// [ParseTypeConversionConstraint].
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

// makeConvertFunc returns an implementation of the built-in function "convert".
//
// It lives in here rather than in package funcs mainly to avoid import cycles,
// but it also means this function implementation lives close to the Go types
// and functions that it's thinly wrapping so it should be easier to maintain
// this along with everything else if our handling of type conversion targets
// changes in future.
func makeConvertFunc() function.Function {
	// (This is a function rather than just a package-level variable because
	// we're expecting that future work will cause it to take an argument
	// representing which user-defined type aliases are in scope. For now
	// we just always return the same result, though.)

	var typeConversionType cty.Type
	typeConversionType = cty.CapsuleWithOps("type", reflect.TypeFor[TypeConversionConstraint](), &cty.CapsuleOps{
		ExtensionData: func(key any) any {
			switch key {
			case customdecode.CustomExpressionDecoder:
				return customdecode.CustomExpressionDecoderFunc(
					func(expr hcl.Expression, _ *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
						convertTarget, diags := ParseTypeConversionConstraint(expr)
						ret := cty.CapsuleVal(typeConversionType, &convertTarget)
						return ret, diags.ToHCL()
					},
				)
			default:
				return nil
			}
		},
		TypeGoString: func(_ reflect.Type) string {
			// This type doesn't actually have a real GoString, because it
			// can't be used independently from just being a special parameter
			// type for the convert function.
			return "typeConversionType"
		},
		GoString: func(raw any) string {
			convertTargetPtr := raw.(*TypeConversionConstraint)
			return fmt.Sprintf(
				"lang.NewTypeConversionConstraint(%#v, %#v)",
				convertTargetPtr.ConvertTarget,
				convertTargetPtr.DefaultAttrVals,
			)
		},
	})

	convertMain := func(args []cty.Value) (cty.Value, error) {
		// The Type and Impl for convert are essentially the same except that
		// Type just throws the resulting value away and returns its type,
		// so this is the common part of both.
		convertTargetPtr := args[1].EncapsulatedValue().(*TypeConversionConstraint)
		got, err := convertTargetPtr.ConvertValue(args[0])
		if err != nil {
			return cty.NilVal, function.NewArgError(0, err)
		}
		return got, nil
	}
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name:             "value",
				Type:             cty.DynamicPseudoType,
				AllowDynamicType: true,
				AllowNull:        true,
				AllowUnknown:     true,
				AllowMarked:      true,
			},
			{
				Name: "type",
				Type: typeConversionType,
			},
		},
		Type: func(args []cty.Value) (cty.Type, error) {
			got, err := convertMain(args)
			if err != nil {
				return cty.NilType, err
			}
			return got.Type(), nil
		},
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			return convertMain(args)
		},
	})
}
