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
		var ephemeralAsNull func(cty.Value) cty.Value
		// Inspired heavily by https://github.com/zclconf/go-cty/blob/v1.16.3/cty/unknown_as_null.go#L10
		ephemeralAsNull = func(val cty.Value) cty.Value {
			ty := val.Type()

			// Preserve non-ephemeral marks
			nonEphemeralMarks := val.Marks()
			delete(nonEphemeralMarks, marks.Ephemeral)

			switch {
			case val.IsNull():
				return cty.NullVal(ty).WithMarks(nonEphemeralMarks)
			case !val.IsKnown():
				// TODO ensure we are handling unknown properly
				// See comments in sensitive.go
				return cty.UnknownVal(val.Type()).WithMarks(nonEphemeralMarks)
			case val.HasMark(marks.Ephemeral):
				// This whole value is marked as ephemeral and should be null
				return cty.NullVal(val.Type()).WithMarks(nonEphemeralMarks)
			case ty.IsListType() || ty.IsTupleType() || ty.IsSetType():
				unmarkedVal, marks := val.Unmark()
				oldVals := unmarkedVal.AsValueSlice()
				if len(oldVals) == 0 {
					return val
				}

				newVals := make([]cty.Value, len(oldVals))
				for i, v := range oldVals {
					newVals[i] = ephemeralAsNull(v)
				}
				switch {
				case ty.IsListType():
					return cty.ListVal(newVals).WithMarks(marks)
				case ty.IsTupleType():
					return cty.TupleVal(newVals).WithMarks(marks)
				default:
					return cty.SetVal(newVals).WithMarks(marks)
				}
			case ty.IsMapType() || ty.IsObjectType():
				unmarkedVal, marks := val.Unmark()
				oldVals := unmarkedVal.AsValueMap()
				if len(oldVals) == 0 {
					return val
				}

				newVals := make(map[string]cty.Value, len(oldVals))
				for k, v := range oldVals {
					newVals[k] = ephemeralAsNull(v)
				}

				switch {
				case ty.IsMapType():
					return cty.MapVal(newVals).WithMarks(marks)
				default:
					return cty.ObjectVal(newVals).WithMarks(marks)
				}
			case val.CanIterateElements():
				// The cases above should be exhaustive, but we should be *absolutely* sure
				_, allMarks := val.UnmarkDeep()
				if _, ok := allMarks[marks.Ephemeral]; ok {
					return cty.NullVal(val.Type()).WithSameMarks(val)
				}
				return val
			default:
				// Not marked
				return val
			}
		}

		return ephemeralAsNull(args[0]), nil
	},
})
