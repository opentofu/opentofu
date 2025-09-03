// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"github.com/zclconf/go-cty/cty"
)

// Maybe is a generalization of cty's concept of "unknown values" to other
// types in Go, to help with modelling situations where something other
// than a [cty.Value] is derived from such a value but only when the original
// value is known.
//
// The only implementation of this interface is [KnownValue], representing a known
// value. An unknown value is represented as a nil value of this type. Use
// a conditional type assertion to [KnownValue] to check whether the value is
// known, or use the other helper functions in this package to derive new
// Maybe values from existing Maybe values.
type Maybe[T any] interface {
	maybeImpl(T)
}

// MapValue takes a possibly-unknown [cty.Value] and, if it is known, passes
// it to the given function to return a Known[T] wrapping the result of
// the function.
//
// If v is unknown then MapValue immediately returns Unknown[T] without
// calling f. This function only checks the "known-ness" of the top-level
// value, so the function must still be ready to deal with unknown values
// in nested parts of a data structure; if the callback needs a guarantee
// that no unknown values will be present anywhere in the given value,
// use [MapValueDeep] instead.
func MapValue[T any](v cty.Value, f func(cty.Value) T) Maybe[T] {
	if !v.IsKnown() {
		return nil
	}
	return KnownValue[T]{f(v)}
}

// MapValueDeep is like [MapValue] except that it returns an unknown
// Maybe if there are unknown values in any part of the given value,
// rather than only checking the top-level value for "known-ness".
func MapValueDeep[T any](v cty.Value, f func(cty.Value) T) Maybe[T] {
	if !v.IsWhollyKnown() {
		return nil
	}
	return KnownValue[T]{f(v)}
}

// MapMaybe takes a [Maybe] value of one type and, if it's known, uses
// the given function to derive a new known Maybe that's possibly of a
// different type.
//
// If the given Maybe is unknown then this immediately returns a new
// maybe of the target type, without calling f.
func MapMaybe[T, R any](input Maybe[T], f func(T) R) Maybe[R] {
	known, ok := input.(KnownValue[T])
	if !ok {
		return nil
	}
	return KnownValue[R]{f(known.Value)}
}

// GetKnown checks whether the given Maybe is known and if so returns the
// value inside it and true. If the value is unknown then it returns the
// zero value of T and false.
//
// This is essentially the same as performing a conditional type assertion
// on the given value, but more convenient when T is a complicated type
// because the Go compiler can infer T automatically based on the type of
// input.
func GetKnown[T any](input Maybe[T]) (T, bool) {
	known, ok := input.(KnownValue[T])
	return known.Value, ok
}

func Known[T any](v T) KnownValue[T] {
	return KnownValue[T]{v}
}

type KnownValue[T any] struct {
	Value T
}

// maybeImpl implements Maybe.
func (k KnownValue[T]) maybeImpl(T) {}
