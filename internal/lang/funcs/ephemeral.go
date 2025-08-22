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

var EphemeralAsNullFunc = function.New(&function.Spec{
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
		// The result type is always the same as the argument type.
		return args[0].Type(), nil
	},
	Impl: func(args []cty.Value, retType cty.Type) (ret cty.Value, err error) {
		return cty.Transform(args[0], func(_ cty.Path, val cty.Value) (cty.Value, error) {
			ty := val.Type()

			// Preserve non-ephemeral marks
			nonEphemeralMarks := val.Marks()
			delete(nonEphemeralMarks, marks.Ephemeral)

			switch {
			case val.IsNull():
				return cty.NullVal(ty).WithMarks(nonEphemeralMarks), nil
			case !val.IsKnown():
				// This mirrors the logic in IsSensitive()
				//
				// When a value is unknown its ephemerality is also not yet
				// finalized, because authors can write expressions where the
				// ephemerality of the result is decided based on some other
				// value that isn't yet known itself. As a simple example:
				//    var.unknown_bool ? var.some_ephemeral_value : "b"
				//
				// Therefore we must report that we can't predict whether an
				// unknown value will be ephemeral or not. For more information,
				// refer to https://github.com/opentofu/opentofu/issues/2415

				return cty.UnknownVal(val.Type()).WithMarks(nonEphemeralMarks), nil
			case val.HasMark(marks.Ephemeral):
				// This whole value is marked as ephemeral and should be null
				return cty.NullVal(val.Type()).WithMarks(nonEphemeralMarks), nil
			default:
				// Not marked
				return val, nil
			}
		})
	},
})
