// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"math"
	"math/big"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

var AssumeNotNullFunc = function.New(makeRefineFunc(
	cty.DynamicPseudoType,
	"Assume that the given value will never be null.",
	func(val cty.Value, _ []cty.Value) error {
		if val.Type().Equals(cty.DynamicPseudoType) {
			// cty does not support refinement of unknown values that have
			// unknown type as a pragmatic concession to the
			// fact that lots of existing code assumes that cty.DynamicVal is
			// the only possible representation of that, and adding refinements
			// would make direct comparison with that value fail.
			//
			// Therefore we require that the value given to this function
			// always has at least a partially-known type, which callers can
			// achieve by calling another function that performs implicit or
			// explicit type conversion, such as "convert" or
			// "assumestringprefix".
			//
			// We intentionally check only for dynamic type and not whether
			// the value is unknown. This means that if someone passes in an
			// untyped null then we'll return this type-related error in
			// preference to the "assumption not upheld" error to help authors
			// get their call set up correctly before dealing with an unexpected
			// dynamic null.
			return function.NewArgErrorf(0, `given value must have a known type; consider using the \"convert\" function to specify a type to assume`)
		}
		return nil
	},
	func(args []cty.Value, b *cty.RefinementBuilder) *cty.RefinementBuilder {
		return b.NotNull()
	},
))

var AssumeStringPrefixFunc = function.New(makeRefineFunc(
	cty.String,
	"Assume that the given string will definitely have the specified prefix.",
	nil,
	func(args []cty.Value, b *cty.RefinementBuilder) *cty.RefinementBuilder {
		prefix := args[0].AsString()
		return b.StringPrefix(prefix)
	},
	function.Parameter{
		Name:        "prefix",
		Type:        cty.String,
		Description: "The prefix to assume.",
	},
))

var AssumeListLengthFunc = function.New(makeCollectionLengthBoundsFunc(cty.List, "list"))
var AssumeListLengthMinFunc = function.New(makeCollectionLengthLowerBoundFunc(cty.List, "list"))
var AssumeListLengthMaxFunc = function.New(makeCollectionLengthUpperBoundFunc(cty.List, "list"))
var AssumeSetLengthFunc = function.New(makeCollectionLengthBoundsFunc(cty.Set, "set"))
var AssumeSetLengthMinFunc = function.New(makeCollectionLengthLowerBoundFunc(cty.Set, "set"))
var AssumeSetLengthMaxFunc = function.New(makeCollectionLengthUpperBoundFunc(cty.Set, "set"))
var AssumeMapLengthFunc = function.New(makeCollectionLengthBoundsFunc(cty.Map, "map"))
var AssumeMapLengthMinFunc = function.New(makeCollectionLengthLowerBoundFunc(cty.Map, "map"))
var AssumeMapLengthMaxFunc = function.New(makeCollectionLengthUpperBoundFunc(cty.Map, "map"))

func makeRefineFunc(typeConstraint cty.Type, desc string, checkArgs func(cty.Value, []cty.Value) error, refine func(args []cty.Value, b *cty.RefinementBuilder) *cty.RefinementBuilder, params ...function.Parameter) *function.Spec {
	spec := &function.Spec{
		Description: desc,
		Params: []function.Parameter{
			{
				Name:             "value",
				Type:             typeConstraint,
				Description:      "The value to make the assumption about.",
				AllowNull:        true,
				AllowUnknown:     true,
				AllowDynamicType: true,
			},
		},
		Type: function.StaticReturnType(typeConstraint),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			v := args[0]
			if checkArgs != nil {
				err := checkArgs(v, args[1:])
				if err != nil {
					return cty.UnknownVal(typeConstraint), err
				}
			}
			realRefine := func(b *cty.RefinementBuilder) *cty.RefinementBuilder {
				return refine(args[1:], b)
			}
			ret, ok := tryApplyRefinement(v, realRefine)
			if !ok {
				return cty.UnknownVal(typeConstraint), function.NewArgErrorf(0, "assumption was not upheld")
			}
			return ret, nil
		},
	}
	spec.Params = append(spec.Params, params...)
	return spec
}

func makeCollectionLengthBoundsFunc(kind func(cty.Type) cty.Type, noun string) *function.Spec {
	return makeRefineFunc(
		kind(cty.DynamicPseudoType),
		"Assume that the given "+noun+" will have a length in the given bounds.",
		func(_ cty.Value, args []cty.Value) error {
			for i, v := range args {
				if v, acc := v.AsBigFloat().Int64(); acc != big.Exact || v >= math.MaxInt {
					return function.NewArgErrorf(i+1, "must be a whole number between 0 and %d", math.MaxInt)
				}
			}
			return nil
		},
		func(args []cty.Value, b *cty.RefinementBuilder) *cty.RefinementBuilder {
			// Our argument validator above already guaranteed that the two
			// arguments are whole numbers that can fit into an int.
			lower, _ := args[0].AsBigFloat().Int64()
			upper, _ := args[1].AsBigFloat().Int64()
			return b.CollectionLengthLowerBound(int(lower)).CollectionLengthUpperBound(int(upper))
		},
		function.Parameter{
			Name:        "min_length",
			Type:        cty.Number,
			Description: "The minimum possible " + noun + " length.",
		},
		function.Parameter{
			Name:        "max_length",
			Type:        cty.Number,
			Description: "The maximum possible " + noun + " length.",
		},
	)
}

func makeCollectionLengthLowerBoundFunc(kind func(cty.Type) cty.Type, noun string) *function.Spec {
	return makeRefineFunc(
		kind(cty.DynamicPseudoType),
		"Assume that the given "+noun+" will have a length of at least the given number.",
		func(_ cty.Value, args []cty.Value) error {
			if v, acc := args[0].AsBigFloat().Int64(); acc != big.Exact || v >= math.MaxInt {
				return function.NewArgErrorf(0, "must be a whole number between 0 and %d", math.MaxInt)
			}
			return nil
		},
		func(args []cty.Value, b *cty.RefinementBuilder) *cty.RefinementBuilder {
			// Our argument validator above already guaranteed that the
			// argument is a whole number that can fit into an int.
			bound, _ := args[0].AsBigFloat().Int64()
			return b.CollectionLengthLowerBound(int(bound))
		},
		function.Parameter{
			Name:        "min_length",
			Type:        cty.Number,
			Description: "The minimum possible " + noun + " length.",
		},
	)
}

func makeCollectionLengthUpperBoundFunc(kind func(cty.Type) cty.Type, noun string) *function.Spec {
	return makeRefineFunc(
		kind(cty.DynamicPseudoType),
		"Assume that the given "+noun+" will have a length of at most the given number.",
		func(_ cty.Value, args []cty.Value) error {
			if v, acc := args[0].AsBigFloat().Int64(); acc != big.Exact || v >= math.MaxInt {
				return function.NewArgErrorf(0, "must be a whole number between 0 and %d", math.MaxInt)
			}
			return nil
		},
		func(args []cty.Value, b *cty.RefinementBuilder) *cty.RefinementBuilder {
			// Our argument validator above already guaranteed that the
			// argument is a whole number that can fit into an int.
			bound, _ := args[0].AsBigFloat().Int64()
			return b.CollectionLengthUpperBound(int(bound))
		},
		function.Parameter{
			Name:        "max_length",
			Type:        cty.Number,
			Description: "The maximum possible " + noun + " length.",
		},
	)
}

func tryApplyRefinement(v cty.Value, refine func(b *cty.RefinementBuilder) *cty.RefinementBuilder) (result cty.Value, ok bool) {
	defer func() {
		if bad := recover(); bad != nil {
			result = cty.DynamicVal
			ok = false
		}
	}()

	// The following will panic if the given refinement isn't applicable to
	// the given value. Our defer function above will then recover and
	// arrange for this function to return false as its second result.
	return v.RefineWith(refine), true
}

func AssumeNotNull(v cty.Value) (cty.Value, error) {
	return AssumeNotNullFunc.Call([]cty.Value{v})
}

func AssumeStringPrefix(s, prefix cty.Value) (cty.Value, error) {
	return AssumeStringPrefixFunc.Call([]cty.Value{s, prefix})
}

func AssumeListLength(l, min, max cty.Value) (cty.Value, error) {
	return AssumeListLengthFunc.Call([]cty.Value{l, min, max})
}

func AssumeListLengthMin(l, min cty.Value) (cty.Value, error) {
	return AssumeListLengthFunc.Call([]cty.Value{l, min})
}

func AssumeListLengthMax(l, max cty.Value) (cty.Value, error) {
	return AssumeListLengthFunc.Call([]cty.Value{l, max})
}

func AssumeSetLength(l, min, max cty.Value) (cty.Value, error) {
	return AssumeSetLengthFunc.Call([]cty.Value{l, min, max})
}

func AssumeSetLengthMin(l, min cty.Value) (cty.Value, error) {
	return AssumeSetLengthFunc.Call([]cty.Value{l, min})
}

func AssumeSetLengthMax(l, max cty.Value) (cty.Value, error) {
	return AssumeSetLengthFunc.Call([]cty.Value{l, max})
}

func AssumeMapLength(l, min, max cty.Value) (cty.Value, error) {
	return AssumeMapLengthFunc.Call([]cty.Value{l, min, max})
}

func AssumeMapLengthMin(l, min cty.Value) (cty.Value, error) {
	return AssumeMapLengthFunc.Call([]cty.Value{l, min})
}

func AssumeMapLengthMax(l, max cty.Value) (cty.Value, error) {
	return AssumeMapLengthFunc.Call([]cty.Value{l, max})
}
