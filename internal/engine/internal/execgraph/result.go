// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
)

// ResultRef represents a result of type T that will be produced by
// some other operation that is opaque to the recipients of the result.
type ResultRef[T any] interface {
	resultPlaceholderSigil(T)
	AnyResultRef
}

// ResourceInstanceResultRef is an alias for the [ResultRef] type used when
// reporting the final result of applying changes to a resource instance
// object.
//
// We give this its own name just because this particular result type tends
// to be named in function signatures elsewhere in the system and the
// simple name is (subjectively) easier to read than the generic name.
type ResourceInstanceResultRef = ResultRef[*exec.ResourceInstanceObject]

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

// resourceInstAddrResultRef is a [ResultRef] referring to an item in the a
// graph's table of constant resource instance addresses.
type resourceInstAddrResultRef struct {
	index int
}

var _ ResultRef[addrs.AbsResourceInstance] = resourceInstAddrResultRef{}

// anyResultPlaceholderSigil implements ResultPlaceholder.
func (v resourceInstAddrResultRef) anyResultPlaceholderSigil() {}

// resultPlaceholderSigil implements ResultPlaceholder.
func (v resourceInstAddrResultRef) resultPlaceholderSigil(addrs.AbsResourceInstance) {}

// providerInstAddrResultRef is a [ResultRef] referring to an item in the a
// graph's table of constant provider instance addresses.
type providerInstAddrResultRef struct {
	index int
}

var _ ResultRef[addrs.AbsProviderInstanceCorrect] = providerInstAddrResultRef{}

// anyResultPlaceholderSigil implements ResultPlaceholder.
func (v providerInstAddrResultRef) anyResultPlaceholderSigil() {}

// resultPlaceholderSigil implements ResultPlaceholder.
func (v providerInstAddrResultRef) resultPlaceholderSigil(addrs.AbsProviderInstanceCorrect) {}

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
