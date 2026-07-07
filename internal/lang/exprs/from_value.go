// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exprs

import (
	"fmt"
	"maps"
	"reflect"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"
)

// FromValue is a wrapper type that adds some of the special abilities of
// [cty.Value] to other types whose values we might derive from a [cty.Value].
//
// Specifically, this generalizes the idea of unknown values and of value
// marks, which are both cross-cutting concerns that can apply to cty values
// of any type and can therefore end up applying to anything derived from a
// cty value too.
//
// As with [cty.Value], marked FromValue instances must be unmarked before
// their values can be extracted to reduce the risk of marks being silently
// lost due to failing to check for them. Handle possibly-marked instances of
// this type using similar patterns as with possibly-marked [cty.Value]
// instances.
//
// Although it's technically possible to use FromValue[cty.Value] as a type,
// that is confusing and not a useful thing to do. Use [cty.Value] directly
// instead.
type FromValue[T any] struct {
	knownValue T
	known      bool
	marks      cty.ValueMarks
}

// Known constructs an unmarked [FromValue] representing a known value of type T.
func Known[T any](knownValue T) FromValue[T] {
	return FromValue[T]{
		knownValue: knownValue,
		known:      true,
	}
}

// Known constructs an unmarked [FromValue] representing an unknown value of type T.
func Unknown[T any]() FromValue[T] {
	return FromValue[T]{
		known: false,
	}
}

// DeriveFromValue passes an unmarked version of the given value to the given
// function only if it is known, and then returns the result.
//
// If the given value is unknown the given function is not called at all and
// instead an unknown [FromValue] is returned without any error.
// Any marks from the given value are automatically tranferred to the result,
// regardless of whether the value is known.
//
// This is a helper only for the simple case where marks anywhere in the value
// are to be taken verbatim and where the known-ness of the top-level value is
// the sole decider for whether the result is known. For other situations,
// implement the rules directly in calling code and then call either [Known] or
// [Unknown].
//
// Note that the given function will still be called if the given value is null,
// has unknown values nested within it, or has marked values nested within it.
// It's the given function's responsibility to represent those situations
// somehow, such as by using a nilable T and returning nil when null, or by
// returning a complex type with other [FromValue] instances nested inside it
// to represent unknown-ness and markedness in a more detailed way.
func DeriveFromValue[T any](v cty.Value, f func(cty.Value) (T, error)) (FromValue[T], error) {
	v, marks := v.Unmark()
	if !v.IsKnown() {
		return Unknown[T]().WithMarks(marks), nil
	}
	knownValue, err := f(v)
	return Known(knownValue).WithMarks(marks), err
}

// DeriveFromDerived is like [DeriveFromValue] except that it starts with a
// [FromValue] instead of from a [cty.Value].
//
// The given function is called only if fv is known, in which case its return
// value is wrapped in another [FromValue] with the same marks.
//
// If fv is unknown then the result is an unknown value with no errors, but
// still preserving the marks from the input.
//
// FIXME: Once we're using Go 1.27, change this into a generic method called
// FromValue.Derive instead, with fv becoming the receiver. "DeriveFromDerived"
// is just a temporary placeholder name until then.
func DeriveFromDerived[T, U any](fv FromValue[T], f func(T) (U, error)) (FromValue[U], error) {
	if !fv.IsKnown() {
		return Unknown[U]().WithMarks(fv.marks), nil
	}
	newKnownValue, err := f(fv.knownValue)
	return Known(newKnownValue).WithMarks(fv.marks), err
}

// KnownValue returns the known value from the reciever, or panics if the
// receiver represents an unknown value.
//
// If you need to test dynamically whether the value is known, use either
// [FromValue.IsKnown] or [FromValue.ValueOk].
//
// This panics if the value is marked, to force callers to use
// [FromValue.Unmark] first to explicitly handle any marks.
func (fv FromValue[T]) KnownValue() T {
	if len(fv.marks) != 0 {
		panic("value is marked, so must be unmarked first")
	}
	if !fv.known {
		panic("value is not known")
	}
	return fv.knownValue
}

// IsKnown returns true if and only if the receiver represents a known value.
//
// If this function returns true and the value is not marked then
// [FromValue.KnownValue] will not panic.
//
// This function can be used on both marked and unmarked values, but if it's
// used to decide whether the caller should return an unknown value of a
// different type then the caller should consider whether it's necessary
// to copy marks to that newly-constructed value.
func (fv FromValue[T]) IsKnown() bool {
	return fv.known
}

// ValueOk combines [FromValue.IsKnown] and [FromValue.KnownValue], returning
// both the known value (if any) and a flag indicating the knownness.
//
// If the second result is false then the first result is the zero value of T.
//
// This panics if the value is marked, to force callers to use
// [FromValue.Unmark] first to explicitly handle any marks.
func (fv FromValue[T]) ValueOk() (T, bool) {
	if len(fv.marks) != 0 {
		panic("value is marked, so must be unmarked first")
	}
	if !fv.known {
		var zero T
		return zero, false
	}
	return fv.knownValue, true
}

// Mark returns a copy of the reciever with the given marks added to its marks
// set. This is analogous to [cty.Value.Mark].
func (fv FromValue[T]) Mark(marks ...any) FromValue[T] {
	oldMarks := fv.marks
	fv.marks = make(cty.ValueMarks, len(oldMarks)+len(marks))
	maps.Copy(fv.marks, oldMarks)
	for _, mark := range marks {
		if _, ok := mark.(cty.ValueMarks); ok {
			// This hazard arises when confusing "Mark" with "WithMarks".
			// This would panic anyway because ValueMarks is not comparable,
			// but we provide a specialized hint in the panic message.
			panic("passed cty.ValueMarks to FromValue.Mark; use FromValue.WithMarks instead")
		}
		fv.marks[mark] = struct{}{}
	}
	return fv
}

// HasMark returns true if and only if the value has the given mark.
// This is analogous to [cty.Value.HasMark].
func (fv FromValue[T]) HasMark(mark any) bool {
	_, ok := fv.marks[mark]
	return ok
}

// Mark returns a copy of the reciever with the given marks added to its marks
// set. This is analogous to [cty.Value.WithMarks].
func (fv FromValue[T]) WithMarks(marks cty.ValueMarks) FromValue[T] {
	oldMarks := fv.marks
	fv.marks = make(cty.ValueMarks, len(oldMarks)+len(marks))
	maps.Copy(fv.marks, oldMarks)
	maps.Copy(fv.marks, marks)
	return fv
}

// Unmark returns a copy of the receiver with an empty set of marks and also
// returns the receiver's marks. This is analogous to [cty.Value.Unmark].
//
// The value and "knownness" are unaffected by this operation.
func (fv FromValue[T]) Unmark() (FromValue[T], cty.ValueMarks) {
	return FromValue[T]{
		knownValue: fv.knownValue,
		known:      fv.known,
	}, fv.marks
}

// Equal returns true if the reciever and "other" have both equal values and
// equal marks.
//
// If T also implements an Equal method then the values are compared using
// that. Otherwise the values are compared with [reflect.DeepEqual], which
// is not suitable for all types. If you're comparing values using [cmp],
// consider using [FromValueCmpOptions] for a better deep comparison.
func (fv FromValue[T]) Equal(other FromValue[T]) bool {
	if len(fv.marks) != len(other.marks) {
		return false
	}
	for _, mark := range fv.marks {
		if !other.HasMark(mark) {
			return false
		}
	}
	if fv.known != other.known {
		return false
	}
	if fv.known { // (and therefore other.known is also true)
		type Equaler interface {
			Equal(other T) bool
		}
		if aEq, ok := any(fv.knownValue).(Equaler); ok {
			return aEq.Equal(other.knownValue)
		}
		return reflect.DeepEqual(fv.knownValue, other.knownValue)
	}
	return true // two unknown values with the same marks are equal
}

// GoString returns a debug-oriented string representation of the receiver,
// formatted as if it were a Go expression constructing the same value.
func (fv FromValue[T]) GoString() string {
	if len(fv.marks) != 0 {
		var buf strings.Builder
		fv, marks := fv.Unmark()
		buf.WriteString(fv.GoString())
		buf.WriteString(".Mark(")
		first := true
		for mark := range marks {
			if !first {
				buf.WriteString(", ")
			}
			first = false
			fmt.Fprintf(&buf, "%#v", mark)
		}
		buf.WriteString(")")
		return buf.String()
	}
	if !fv.known {
		return fmt.Sprintf("exprs.Unknown[%T]()", fv.knownValue)
	}
	return fmt.Sprintf("exprs.Known(%#v)", fv.knownValue)
}

// forGoCmp is a helper used as part of [FromValueCmpOptions].
//
// It serves both as the sigil allowing us to match any instantiation of
// [FromValue] _and_ as the transformer for producing something that go-cmp
// can compare using [reflect].
func (fv FromValue[T]) forGoCmp() any {
	type FromValue struct {
		KnownValue any
		Known      bool
		Marks      cty.ValueMarks
	}
	return FromValue{
		KnownValue: fv.knownValue,
		Known:      fv.known,
		Marks:      fv.marks,
	}
}

// anyFromValue is an interface implemented by all instantiations of [FromValue],
// and by no other types.
type anyFromValue interface {
	forGoCmp() any
}

// FromValueCmpOptions is a set of options for [cmp] that make it possible
// to deep-compare inside instances of [FromValue] types.
var FromValueCmpOptions = cmp.Options{
	// This is pretty awkward because go-cmp is reflect-based and reflect
	// can't work generically over all instantiations of a generic type.
	cmp.FilterValues(
		func(a, b any) bool {
			_, aOK := a.(anyFromValue)
			_, bOK := b.(anyFromValue)
			return aOK && bOK
		},
		cmp.Transformer("FromValue.forGoCmp", func(v any) any {
			vI := v.(anyFromValue)
			return vI.forGoCmp()
		}),
	),
}
