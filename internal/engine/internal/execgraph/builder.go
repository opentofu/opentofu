// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"fmt"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/states"
)

// Builder is a helper for gradually constructing an execution graph.
//
// Builder is not concurrency-safe, and so it's the caller's responsibility that
// at most one method of this type is running at a time across all goroutines.
//
// The methods of this type each add exactly one item to the execution graph,
// returning an opaque reference representing its resulting value which can
// then be used as an argument to other methods. These opaque reference values
// are specific to the builder that returned them; using a reference returned
// by some other builder will at best cause a nonsense graph and at worse could
// cause panics.
type Builder struct {
	graph          *Graph
	emptyWaiterRef ResultRef[struct{}]

	// Note that we intentionally don't have any other singletons here beyond
	// the simple emptyWaiterRef, because this builder is intentionally
	// low-level with minimal "magic". The planning engine has its own
	// higher-level wrapper around this type that deals with problems such as
	// making sure that each provider instance only has one set of nodes in the
	// execution graph, etc, because the planning engine has access to more
	// context about how the various different higher-level objects relate to
	// each other.
}

func NewBuilder() *Builder {
	return &Builder{
		graph: &Graph{
			resourceInstanceResults: addrs.MakeMap[addrs.AbsResourceInstance, ResourceInstanceResultRef](),
		},
		emptyWaiterRef: nil, // will be populated on first request for an empty waiter
	}
}

// Finish returns the graph that has been built, which is then immutable.
//
// After calling this function the Builder is invalid and must not be used
// anymore.
func (b *Builder) Finish() *Graph {
	ret := b.graph
	b.graph = nil
	return ret
}

// ConstantValue adds a constant [cty.Value] as a source node. The result
// can be used as an operand to a subsequent operation.
func (b *Builder) ConstantValue(v cty.Value) ResultRef[cty.Value] {
	idx := appendIndex(&b.graph.constantVals, v)
	return valueResultRef{idx}
}

// ConstantResourceInstAddr adds a constant [addrs.AbsResourceInstance]
// address as a source node. The result can be used as an operand to a
// subsequent operation.
func (b *Builder) ConstantResourceInstAddr(addr addrs.AbsResourceInstance) ResultRef[addrs.AbsResourceInstance] {
	idx := appendIndex(&b.graph.resourceInstAddrs, addr)
	ret := resourceInstAddrResultRef{idx}
	return ret
}

// ConstantDeposedKey adds a constant [states.DeposedKey] as a source node.
// The result can be used as an operand to a subsequent operation.
func (b *Builder) ConstantDeposedKey(key states.DeposedKey) ResultRef[states.DeposedKey] {
	idx := appendIndex(&b.graph.deposedKeys, key)
	ret := deposedKeyResultRef{idx}
	return ret
}

// ConstantProviderInstAddr adds a constant [addrs.AbsProviderInstanceCorrect]
// address as a source node. The result can be used as an operand to a
// subsequent operation.
func (b *Builder) ConstantProviderInstAddr(addr addrs.AbsProviderInstanceCorrect) ResultRef[addrs.AbsProviderInstanceCorrect] {
	idx := appendIndex(&b.graph.providerInstAddrs, addr)
	ret := providerInstAddrResultRef{idx}
	return ret
}

// ProviderInstanceConfig registers an operation for evaluating the
// configuration for a provider instance.
func (b *Builder) ProviderInstanceConfig(addrRef ResultRef[addrs.AbsProviderInstanceCorrect], waitFor AnyResultRef) ResultRef[*exec.ProviderInstanceConfig] {
	waiter := b.ensureWaiterRef(waitFor)
	return operationRef[*exec.ProviderInstanceConfig](b, operationDesc{
		opCode:   opProviderInstanceConfig,
		operands: []AnyResultRef{addrRef, waiter},
	})
}

// ProviderInstanceOpen registers an operation for opening a client for a
// particular provider instance.
func (b *Builder) ProviderInstanceOpen(config ResultRef[*exec.ProviderInstanceConfig]) ResultRef[*exec.ProviderClient] {
	return operationRef[*exec.ProviderClient](b, operationDesc{
		opCode:   opProviderInstanceOpen,
		operands: []AnyResultRef{config},
	})
}

// ProviderInstanceClose registers an operation for closing a provider client
// that was previously opened through [Builder.OpenProviderInstance].
func (b *Builder) ProviderInstanceClose(client ResultRef[*exec.ProviderClient], waitFor AnyResultRef) ResultRef[struct{}] {
	waiter := b.ensureWaiterRef(waitFor)
	return operationRef[struct{}](b, operationDesc{
		opCode:   opProviderInstanceClose,
		operands: []AnyResultRef{client, waiter},
	})
}

func (b *Builder) ResourceInstanceDesired(
	addr ResultRef[addrs.AbsResourceInstance],
	waitFor AnyResultRef,
) ResultRef[*eval.DesiredResourceInstance] {
	waiter := b.ensureWaiterRef(waitFor)
	return operationRef[*eval.DesiredResourceInstance](b, operationDesc{
		opCode:   opResourceInstanceDesired,
		operands: []AnyResultRef{addr, waiter},
	})
}

func (b *Builder) ResourceInstancePrior(
	addr ResultRef[addrs.AbsResourceInstance],
) ResourceInstanceResultRef {
	return operationRef[*exec.ResourceInstanceObject](b, operationDesc{
		opCode:   opResourceInstancePrior,
		operands: []AnyResultRef{addr},
	})
}

// ManagedFinalPlan registers an operation to decide the "final plan" for a
// managed resource instance object, which may or may not be "desired".
//
// If the object is not "desired" then the desiredInst result is a nil pointer.
// The underlying provider API represents that situation by setting the
// "configuration value" to null.
//
// Similarly, if the object did not previously exist but is now desired then
// the priorState result is a nil pointer, which should be represented in the
// provider API by setting the prior state value to null.
//
// If the planning phase learned that the provider needs to handle a change
// as a "replace" then in the execution graph there should be two separate
// "final plan" and "apply changes" chains, where one has a nil desiredInst
// and the other has a nil priorState. desiredInst and priorState should only
// both be set when handling an in-place update.
func (b *Builder) ManagedFinalPlan(
	desiredInst ResultRef[*eval.DesiredResourceInstance],
	priorState ResourceInstanceResultRef,
	plannedVal ResultRef[cty.Value],
	providerClient ResultRef[*exec.ProviderClient],
) ResultRef[*exec.ManagedResourceObjectFinalPlan] {
	return operationRef[*exec.ManagedResourceObjectFinalPlan](b, operationDesc{
		opCode:   opManagedFinalPlan,
		operands: []AnyResultRef{desiredInst, priorState, plannedVal, providerClient},
	})
}

// ManagedApply registers an operation to apply a "final plan" for a managed
// resource instance object.
//
// The finalPlan argument should typically be something returned by a previous
// call to [Builder.ManagedFinalPlan] with the same provider client.
//
// fallbackObj is usually a [NilResultRef], but should be set for the "create"
// leg of a "create then destroy" replace operation to be the result of a
// call to [Builder.ManagedDepose] so that the deposed object can be restored
// to current if the create call completely fails to create a new object.
func (b *Builder) ManagedApply(
	finalPlan ResultRef[*exec.ManagedResourceObjectFinalPlan],
	fallbackObj ResourceInstanceResultRef,
	providerClient ResultRef[*exec.ProviderClient],
	waitFor AnyResultRef,
) ResourceInstanceResultRef {
	return operationRef[*exec.ResourceInstanceObject](b, operationDesc{
		opCode:   opManagedApply,
		operands: []AnyResultRef{finalPlan, fallbackObj, providerClient, waitFor},
	})
}

func (b *Builder) ManagedDepose(
	currentObj ResourceInstanceResultRef,
	waitFor AnyResultRef,
) ResourceInstanceResultRef {
	return operationRef[*exec.ResourceInstanceObject](b, operationDesc{
		opCode:   opManagedDepose,
		operands: []AnyResultRef{currentObj, waitFor},
	})
}

func (b *Builder) ManagedAlreadyDeposed(
	instAddr ResultRef[addrs.AbsResourceInstance],
	deposedKey ResultRef[states.DeposedKey],
) ResourceInstanceResultRef {
	return operationRef[*exec.ResourceInstanceObject](b, operationDesc{
		opCode:   opManagedAlreadyDeposed,
		operands: []AnyResultRef{instAddr, deposedKey},
	})
}

func (b *Builder) ManagedChangeAddr(
	currentObj ResourceInstanceResultRef,
	newAddr ResultRef[addrs.AbsResourceInstance],
) ResourceInstanceResultRef {
	return operationRef[*exec.ResourceInstanceObject](b, operationDesc{
		opCode:   opManagedChangeAddr,
		operands: []AnyResultRef{currentObj, newAddr},
	})
}

func (b *Builder) DataRead(
	desiredInst ResultRef[*eval.DesiredResourceInstance],
	plannedVal ResultRef[cty.Value],
	providerClient ResultRef[*exec.ProviderClient],
) ResourceInstanceResultRef {
	return operationRef[*exec.ResourceInstanceObject](b, operationDesc{
		opCode:   opDataRead,
		operands: []AnyResultRef{desiredInst, plannedVal, providerClient},
	})
}

func (b *Builder) EphemeralOpen(
	desiredInst ResultRef[*eval.DesiredResourceInstance],
	providerClient ResultRef[*exec.ProviderClient],
) ResultRef[*exec.OpenEphemeralResourceInstance] {
	return operationRef[*exec.OpenEphemeralResourceInstance](b, operationDesc{
		opCode:   opEphemeralOpen,
		operands: []AnyResultRef{desiredInst, providerClient},
	})
}

func (b *Builder) EphemeralState(
	ephemeralInst ResultRef[*exec.OpenEphemeralResourceInstance],
) ResourceInstanceResultRef {
	return operationRef[*exec.ResourceInstanceObject](b, operationDesc{
		opCode:   opEphemeralState,
		operands: []AnyResultRef{ephemeralInst},
	})
}

func (b *Builder) EphemeralClose(
	ephemeralInst ResultRef[*exec.OpenEphemeralResourceInstance],
	waitFor AnyResultRef,
) ResultRef[struct{}] {
	waiter := b.ensureWaiterRef(waitFor)
	return operationRef[struct{}](b, operationDesc{
		opCode:   opEphemeralClose,
		operands: []AnyResultRef{ephemeralInst, waiter},
	})
}

// SetResourceInstanceFinalStateResult records which result should be treated
// as the "final state" for the given resource instance, for purposes such as
// propagating the result value back into the evaluation system to allow
// downstream expressions to derive from it.
//
// Only one call is allowed per distinct [addrs.AbsResourceInstance] value. If
// two callers try to register for the same address then the second call will
// panic.
func (b *Builder) SetResourceInstanceFinalStateResult(addr addrs.AbsResourceInstance, result ResourceInstanceResultRef) {
	if b.graph.resourceInstanceResults.Has(addr) {
		panic(fmt.Sprintf("duplicate registration for %s final state result", addr))
	}
	b.graph.resourceInstanceResults.Put(addr, result)
}

// ResourceInstanceFinalStateResult returns the result reference for the given
// resource instance that should previously have been registered using
// [Builder.SetResourceInstanceFinalStateResult].
//
// The return type is [AnyResultRef] because this is intended for use as
// an argument to [Builder.Waiter] or to a function returned by
// [Builder.MutableWaiter] when explicitly representing the dependencies
// between different resource and provider instances. The actual final state
// result for the source instance travels indirectly through the evaluator
// rather than directly within the execution graph.
//
// This function panics if a result reference for the given resource instance
// was not previously registered, because that suggests a bug elsewhere in the
// system that caused the construction of subgraphs for different resource
// instances to happen in the wrong order.
func (b *Builder) ResourceInstanceFinalStateResult(addr addrs.AbsResourceInstance) AnyResultRef {
	ret, ok := b.graph.resourceInstanceResults.GetOk(addr)
	if !ok {
		panic(fmt.Sprintf("requested result for %s, which has not yet been registered", addr))
	}
	return ret
}

// Waiter creates a "fan-in" node where a single result depends on the
// completion of an arbitrary number of other results.
//
// The values produced by the dependencies are discarded; this only creates
// a "must happen after" relationship with the given dependencies.
func (b *Builder) Waiter(dependencies ...AnyResultRef) AnyResultRef {
	return b.makeWaiter(dependencies)
}

// MutableWaiter is like [Builder.Waiter] except that the returned waiter
// initially has no dependencies and then dependencies can be added to it
// separately by calling the returned function.
//
// This is intended for situations where an item with dependencies must be
// added to the graph before its dependencies are known, and then the caller
// gradually discovers all of the dependencies in later work.
//
// The registration function is not concurrency safe, so callers are responsible
// for ensuring that there is only at most one call to each returned distinct
// registration function across all goroutines.
func (b *Builder) MutableWaiter() (AnyResultRef, func(AnyResultRef)) {
	idx := appendIndex(&b.graph.waiters, []AnyResultRef{})
	ref := waiterResultRef{idx}
	registerFunc := func(ref AnyResultRef) {
		b.graph.waiters[idx] = append(b.graph.waiters[idx], ref)
	}
	return ref, registerFunc
}

// operationRef is a helper used by all of the [Builder] methods that produce
// "operation" nodes, dealing with the common registration part.
//
// Callers MUST ensure all of the following before calling this function:
// - They already hold a lock on builder.mu and retain it throughout the call.
// - The specified T is the correct result type for the operation being described.
//
// This is effectively a method on [Builder], but written as a package-level
// function just so it can have a type parameter.
func operationRef[T any](builder *Builder, op operationDesc) ResultRef[T] {
	idx := appendIndex(&builder.graph.ops, op)
	return operationResultRef[T]{idx}
}

// ensureWaiterRef exists to make it more convenient for callers of Builder
// to populate "waitFor" arguments, by normalizing whatever was provided
// so that it's definitely a waiter reference.
//
// The given ref can be nil to represent waiting for nothing, in which case
// the result is a reference to an empty waiter. If the given ref is not nil
// but is also not of the correct result type for a waiter then it'll be
// wrapped in a waiter with only a single dependency and the result will
// be a reference to that waiter.
func (b *Builder) ensureWaiterRef(given AnyResultRef) ResultRef[struct{}] {
	if ret, ok := given.(waiterResultRef); ok {
		return ret
	}
	if given == nil {
		return b.makeWaiter(nil)
	}
	return b.makeWaiter([]AnyResultRef{given})
}

func (b *Builder) makeWaiter(waitFor []AnyResultRef) ResultRef[struct{}] {
	if len(waitFor) == 0 {
		// Empty waiters tend to appear in multiple places, so we'll just
		// allocate a single one on request and reuse it.
		if b.emptyWaiterRef == nil {
			idx := appendIndex(&b.graph.waiters, nil)
			b.emptyWaiterRef = waiterResultRef{idx}
		}
		return b.emptyWaiterRef
	}
	idx := appendIndex(&b.graph.waiters, waitFor)
	return waiterResultRef{idx}
}
