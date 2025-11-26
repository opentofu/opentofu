// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/states"
)

// ResultRef represents a result of type T that will be produced by
// some other operation that is opaque to the recipients of the result.
type ResultRef[T any] interface {
	resultPlaceholderSigil(T)
	AnyResultRef
}

// AnyResultRef is a type-erased [ResultRef], for data
// structures that only need to represent the relationships between results
// and not the types of those results.
type AnyResultRef interface {
	anyResultPlaceholderSigil()
}

// valueResultRef is a [ResultRef] referring to an item in the a graph's
// table of constant values.
type valueResultRef struct {
	index int
}

var _ ResultRef[cty.Value] = valueResultRef{}

// anyResultPlaceholderSigil implements ResultPlaceholder.
func (v valueResultRef) anyResultPlaceholderSigil() {}

// resultPlaceholderSigil implements ResultPlaceholder.
func (v valueResultRef) resultPlaceholderSigil(cty.Value) {}

// providerAddrResultRef is a [ResultRef] referring to an item in the a graph's
// table of constant provider addresses.
type providerAddrResultRef struct {
	index int
}

var _ ResultRef[addrs.Provider] = providerAddrResultRef{}

// anyResultPlaceholderSigil implements ResultPlaceholder.
func (v providerAddrResultRef) anyResultPlaceholderSigil() {}

// resultPlaceholderSigil implements ResultPlaceholder.
func (v providerAddrResultRef) resultPlaceholderSigil(addrs.Provider) {}

// desiredResourceInstanceResultRef is a [ResultRef] referring to an item in
// the a graph's table of desired state lookups.
type desiredResourceInstanceResultRef struct {
	index int
}

var _ ResultRef[*eval.DesiredResourceInstance] = desiredResourceInstanceResultRef{}

// anyResultPlaceholderSigil implements ResultRef.
func (d desiredResourceInstanceResultRef) anyResultPlaceholderSigil() {}

// resultPlaceholderSigil implements ResultRef.
func (d desiredResourceInstanceResultRef) resultPlaceholderSigil(*eval.DesiredResourceInstance) {}

// resourceInstancePriorStateResultRef is a [ResultRef] referring to an item in
// the a graph's table of prior state lookups.
type resourceInstancePriorStateResultRef struct {
	index int
}

var _ ResultRef[*states.ResourceInstanceObjectFull] = resourceInstancePriorStateResultRef{}

// anyResultPlaceholderSigil implements ResultRef.
func (r resourceInstancePriorStateResultRef) anyResultPlaceholderSigil() {}

// resultPlaceholderSigil implements ResultRef.
func (r resourceInstancePriorStateResultRef) resultPlaceholderSigil(*states.ResourceInstanceObjectFull) {
}

// providerInstanceConfigResultRef is a [ResultRef] referring to an item in a
// graph's table of provider instance configuration requests.
type providerInstanceConfigResultRef struct {
	index int
}

var _ ResultRef[cty.Value] = providerInstanceConfigResultRef{}

// anyResultPlaceholderSigil implements ResultPlaceholder.
func (v providerInstanceConfigResultRef) anyResultPlaceholderSigil() {}

// resultPlaceholderSigil implements ResultPlaceholder.
func (v providerInstanceConfigResultRef) resultPlaceholderSigil(cty.Value) {}

type operationResultRef[T any] struct {
	index int
}

var _ ResultRef[struct{}] = operationResultRef[struct{}]{}
var _ anyOperationResultRef = operationResultRef[struct{}]{}

// anyResultPlaceholderSigil implements ResultPlaceholder.
func (o operationResultRef[T]) anyResultPlaceholderSigil() {}

// resultPlaceholderSigil implements ResultPlaceholder.
func (o operationResultRef[T]) resultPlaceholderSigil(T) {}

// operationResultIndex implements anyOperationResultRef.
func (o operationResultRef[T]) operationResultIndex() int {
	return o.index
}

type anyOperationResultRef interface {
	operationResultIndex() int
}

type waiterResultRef struct {
	index int
}

var _ ResultRef[struct{}] = waiterResultRef{}

// anyResultPlaceholderSigil implements ResultRef.
func (w waiterResultRef) anyResultPlaceholderSigil() {}

// resultPlaceholderSigil implements ResultRef.
func (w waiterResultRef) resultPlaceholderSigil(struct{}) {}

// NilResultRef returns a special result ref which just always produces the
// zero value of type T, without doing any other work or referring to any other
// data.
//
// This should typically only be used for types whose zero value is considered
// to be the "nil" value for the type, such as pointer types, since otherwise
// the recipient cannot distinguish it from a valid result that just happens
// to be the zero value.
func NilResultRef[T any]() ResultRef[T] {
	return nil
}

func appendIndex[E any](s *[]E, new E) int {
	idx := len(*s)
	*s = append(*s, new)
	return idx
}
