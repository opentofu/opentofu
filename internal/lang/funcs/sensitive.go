// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// SensitiveFunc returns a value identical to its argument except that
// OpenTofu will consider it to be sensitive.
var SensitiveFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{
			Name:             "value",
			Type:             cty.DynamicPseudoType,
			AllowUnknown:     true,
			AllowNull:        true,
			AllowMarked:      true,
			AllowDynamicType: true,
		},
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		// This function only affects the value's marks, so the result
		// type is always the same as the argument type.
		return args[0].Type(), nil
	},
	Impl: func(args []cty.Value, retType cty.Type) (ret cty.Value, err error) {
		return args[0].Mark(marks.Sensitive), nil
	},
})

// NonsensitiveFunc takes a sensitive value and returns the same value without
// the sensitive marking, effectively exposing the value.
var NonsensitiveFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{
			Name:             "value",
			Type:             cty.DynamicPseudoType,
			AllowUnknown:     true,
			AllowNull:        true,
			AllowMarked:      true,
			AllowDynamicType: true,
		},
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		// This function only affects the value's marks, so the result
		// type is always the same as the argument type.
		return args[0].Type(), nil
	},
	Impl: func(args []cty.Value, retType cty.Type) (ret cty.Value, err error) {
		v, m := args[0].Unmark()
		delete(m, marks.Sensitive) // remove the sensitive marking
		return v.WithMarks(m), nil
	},
})

// IsSensitiveFunc returns whether or not the value is sensitive.
var IsSensitiveFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{
			Name:             "value",
			Type:             cty.DynamicPseudoType,
			AllowUnknown:     true,
			AllowNull:        true,
			AllowMarked:      true,
			AllowDynamicType: true,
		},
	},
	RefineResult: refineNotNull,
	Type: func(args []cty.Value) (cty.Type, error) {
		return cty.Bool, nil
	},
	Impl: func(args []cty.Value, retType cty.Type) (ret cty.Value, err error) {
		if !args[0].IsKnown() {
			// When a value is unknown its sensitivity is also not yet
			// finalized, because authors can write expressions where the
			// sensitivity of the result is decided based on some other
			// value that isn't yet known itself. As a simple example:
			//    var.unknown_bool ? sensitive("a") : "b"
			//
			// The above would conservatively return a sensitive unknown
			// string when var.unknown_bool is not known, but then that
			// variable could become false once finalized and cause the
			// sensitivity to change.
			//
			// Therefore we must report that we can't predict whether an
			// unknown value will be sensitive or not. For more information,
			// refer to:
			//    https://github.com/opentofu/opentofu/issues/2415
			return cty.UnknownVal(cty.Bool), nil
		}
		return cty.BoolVal(args[0].HasMark(marks.Sensitive)), nil
	},
})

func Sensitive(v cty.Value) (cty.Value, error) {
	return SensitiveFunc.Call([]cty.Value{v})
}

func Nonsensitive(v cty.Value) (cty.Value, error) {
	return NonsensitiveFunc.Call([]cty.Value{v})
}

func IsSensitive(v cty.Value) (cty.Value, error) {
	return IsSensitiveFunc.Call([]cty.Value{v})
}
