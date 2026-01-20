// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"fmt"
	"sync"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
)

// Builder is a helper for multiple codepaths to collaborate to build an
// execution graph.
//
// The methods of this type each cause something to be added to the graph
// and then return an opaque reference to what was added which can then be
// used as an argument to another method. The opaque reference values are
// specific to the builder that returned them; using a reference returned by
// some other builder will at best cause a nonsense graph and at worst could
// cause panics.
type Builder struct {
	// must hold mu when accessing any part of any other fields
	mu sync.Mutex

	graph *Graph

	// During construction we treat certain items as singletons so that
	// we can do the associated work only once while providing it to
	// multiple callers, and so these maps track those singletons but
	// we throw these away after building is complete because the graph
	// becomes immutable at that point.
	resourceInstAddrRefs addrs.Map[addrs.AbsResourceInstance, ResultRef[addrs.AbsResourceInstance]]
	providerInstAddrRefs addrs.Map[addrs.AbsProviderInstanceCorrect, ResultRef[addrs.AbsProviderInstanceCorrect]]
	openProviderRefs     addrs.Map[addrs.AbsProviderInstanceCorrect, resultWithCloseBlockers[*exec.ProviderClient]]
	emptyWaiterRef       ResultRef[struct{}]
}

func NewBuilder() *Builder {
	return &Builder{
		graph: &Graph{
			resourceInstanceResults: addrs.MakeMap[addrs.AbsResourceInstance, ResourceInstanceResultRef](),
		},
		resourceInstAddrRefs: addrs.MakeMap[addrs.AbsResourceInstance, ResultRef[addrs.AbsResourceInstance]](),
		providerInstAddrRefs: addrs.MakeMap[addrs.AbsProviderInstanceCorrect, ResultRef[addrs.AbsProviderInstanceCorrect]](),
		openProviderRefs:     addrs.MakeMap[addrs.AbsProviderInstanceCorrect, resultWithCloseBlockers[*exec.ProviderClient]](),
		emptyWaiterRef:       nil, // will be populated on first request for an empty waiter
	}
}

// Finish returns the graph that has been built, which is then immutable.
//
// After calling this function the Builder is invalid and must not be used
// anymore.
func (b *Builder) Finish() *Graph {
	b.mu.Lock()
	ret := b.graph
	b.graph = nil
	b.mu.Unlock()
	return ret
}

// ConstantValue adds a constant [cty.Value] as a source node. The result
// can be used as an operand to a subsequent operation.
//
// Each call to this method adds a new constant value to the graph, even if
// a previously-registered value was equal to the given value.
func (b *Builder) ConstantValue(v cty.Value) ResultRef[cty.Value] {
	b.mu.Lock()
	defer b.mu.Unlock()

	idx := appendIndex(&b.graph.constantVals, v)
	return valueResultRef{idx}
}

// ConstantResourceInstAddr adds a constant [addrs.AbsResourceInstance]
// address as a source node. The result can be used as an operand to a
// subsequent operation.
//
// Multiple calls with the same address all return the same result, so in
// practice each distinct address is stored only once.
func (b *Builder) ConstantResourceInstAddr(addr addrs.AbsResourceInstance) ResultRef[addrs.AbsResourceInstance] {
	b.mu.Lock()
	defer b.mu.Unlock()

	if existing, ok := b.resourceInstAddrRefs.GetOk(addr); ok {
		return existing
	}
	idx := appendIndex(&b.graph.resourceInstAddrs, addr)
	ret := resourceInstAddrResultRef{idx}
	b.resourceInstAddrRefs.Put(addr, ret)
	return ret
}

// ConstantProviderInstAddr adds a constant [addrs.AbsProviderInstanceCorrect]
// address as a source node. The result can be used as an operand to a
// subsequent operation.
//
// Multiple calls with the same address all return the same result, so in
// practice each distinct address is stored only once.
func (b *Builder) ConstantProviderInstAddr(addr addrs.AbsProviderInstanceCorrect) ResultRef[addrs.AbsProviderInstanceCorrect] {
	b.mu.Lock()
	defer b.mu.Unlock()

	if existing, ok := b.providerInstAddrRefs.GetOk(addr); ok {
		return existing
	}
	idx := appendIndex(&b.graph.providerInstAddrs, addr)
	ret := providerInstAddrResultRef{idx}
	b.providerInstAddrRefs.Put(addr, ret)
	return ret
}

func (b *Builder) ProviderInstanceConfig(addrRef ResultRef[addrs.AbsProviderInstanceCorrect], waitFor AnyResultRef) ResultRef[*exec.ProviderInstanceConfig] {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.providerInstanceConfigLocked(addrRef, waitFor)
}

func (b *Builder) providerInstanceConfigLocked(addrRef ResultRef[addrs.AbsProviderInstanceCorrect], waitFor AnyResultRef) ResultRef[*exec.ProviderInstanceConfig] {
	waiter := b.ensureWaiterRef(waitFor)
	return operationRef[*exec.ProviderInstanceConfig](b, operationDesc{
		opCode:   opProviderInstanceConfig,
		operands: []AnyResultRef{addrRef, waiter},
	})
}

// ProviderInstanceOpen registers an operation for opening a client for a
// particular provider instance.
//
// Direct callers should typically use [Builder.ProviderInstance] instead,
// because it automatically deduplicates requests for the same provider
// instance and registers the associated "close" operation for the provider
// instance. This method is here primarily for the benefit of [UnmarshalGraph],
// which needs to be able to work with the individual operation nodes when
// reconstructing a previously-marshaled graph.
func (b *Builder) ProviderInstanceOpen(config ResultRef[*exec.ProviderInstanceConfig]) ResultRef[*exec.ProviderClient] {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.providerInstanceOpenLocked(config)
}

func (b *Builder) providerInstanceOpenLocked(config ResultRef[*exec.ProviderInstanceConfig]) ResultRef[*exec.ProviderClient] {
	return operationRef[*exec.ProviderClient](b, operationDesc{
		opCode:   opProviderInstanceOpen,
		operands: []AnyResultRef{config},
	})
}

// ProviderInstanceClose registers an operation for closing a provider client
// that was previously opened through [Builder.OpenProviderInstance].
//
// Direct callers should typically use [Builder.ProviderInstance] instead,
// because it deals with all of the ceremony of opening and closing provider
// clients. This method is here primarily for the benefit of [UnmarshalGraph],
// which needs to be able to work with the individual operation nodes when
// reconstructing a previously-marshaled graph.
func (b *Builder) ProviderInstanceClose(client ResultRef[*exec.ProviderClient], waitFor AnyResultRef) ResultRef[struct{}] {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.providerInstanceCloseLocked(client, waitFor)
}

func (b *Builder) providerInstanceCloseLocked(clientResult ResultRef[*exec.ProviderClient], waitFor AnyResultRef) ResultRef[struct{}] {
	waiter := b.ensureWaiterRef(waitFor)
	return operationRef[struct{}](b, operationDesc{
		opCode:   opProviderInstanceClose,
		operands: []AnyResultRef{clientResult, waiter},
	})
}

// ProviderInstance encapsulates everything required to obtain a configured
// client for a provider instance and ensure that the client stays open long
// enough to handle one or more other operations registered afterwards.
//
// The returned [RegisterCloseBlockerFunc] MUST be called with a reference to
// the result of the final operation in any linear chain of operations that
// depends on the provider to ensure that the provider will stay open at least
// long enough to perform those operations.
//
// This is a compound build action that adds a number of different items to
// the graph at once, although each distinct provider instance address gets
// only one set of nodes added and then subsequent calls get references to
// the same operation results.
func (b *Builder) ProviderInstance(addr addrs.AbsProviderInstanceCorrect, waitFor AnyResultRef) (ResultRef[*exec.ProviderClient], RegisterCloseBlockerFunc) {
	addrResult := b.ConstantProviderInstAddr(addr)

	b.mu.Lock()
	defer b.mu.Unlock()

	// We only register one index for each distinct provider instance address.
	if existing, ok := b.openProviderRefs.GetOk(addr); ok {
		return existing.Result, existing.CloseBlockerFunc
	}
	configResult := b.providerInstanceConfigLocked(addrResult, waitFor)
	openResult := b.providerInstanceOpenLocked(configResult)
	closeWait, registerCloseBlocker := b.makeCloseBlocker()
	// Nothing actually depends on the result of the "close" operation, but
	// eventual execution of the graph will still wait for it to complete
	// because _all_ operations must complete before execution is considered
	// to be finished.
	_ = b.providerInstanceCloseLocked(openResult, closeWait)
	b.openProviderRefs.Put(addr, resultWithCloseBlockers[*exec.ProviderClient]{
		Result:             openResult,
		CloseBlockerResult: closeWait,
		CloseBlockerFunc:   registerCloseBlocker,
	})
	return openResult, registerCloseBlocker
}

func (b *Builder) ResourceInstanceDesired(
	addr ResultRef[addrs.AbsResourceInstance],
	waitFor AnyResultRef,
) ResultRef[*eval.DesiredResourceInstance] {
	b.mu.Lock()
	defer b.mu.Unlock()

	waiter := b.ensureWaiterRef(waitFor)
	return operationRef[*eval.DesiredResourceInstance](b, operationDesc{
		opCode:   opResourceInstanceDesired,
		operands: []AnyResultRef{addr, waiter},
	})
}

func (b *Builder) ResourceInstancePrior(
	addr ResultRef[addrs.AbsResourceInstance],
) ResourceInstanceResultRef {
	b.mu.Lock()
	defer b.mu.Unlock()

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
	b.mu.Lock()
	defer b.mu.Unlock()

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
) ResourceInstanceResultRef {
	b.mu.Lock()
	defer b.mu.Unlock()

	return operationRef[*exec.ResourceInstanceObject](b, operationDesc{
		opCode:   opManagedApply,
		operands: []AnyResultRef{finalPlan, fallbackObj, providerClient},
	})
}

func (b *Builder) ManagedDepose(
	instAddr ResultRef[addrs.AbsResourceInstance],
) ResourceInstanceResultRef {
	b.mu.Lock()
	defer b.mu.Unlock()

	return operationRef[*exec.ResourceInstanceObject](b, operationDesc{
		opCode:   opManagedDepose,
		operands: []AnyResultRef{instAddr},
	})
}

func (b *Builder) ManagedAlreadyDeposed(
	instAddr ResultRef[addrs.AbsResourceInstance],
) ResourceInstanceResultRef {
	b.mu.Lock()
	defer b.mu.Unlock()

	return operationRef[*exec.ResourceInstanceObject](b, operationDesc{
		opCode:   opManagedAlreadyDeposed,
		operands: []AnyResultRef{instAddr},
	})
}

func (b *Builder) DataRead(
	desiredInst ResultRef[*eval.DesiredResourceInstance],
	plannedVal ResultRef[cty.Value],
	providerClient ResultRef[*exec.ProviderClient],
) ResourceInstanceResultRef {
	b.mu.Lock()
	defer b.mu.Unlock()

	return operationRef[*exec.ResourceInstanceObject](b, operationDesc{
		opCode:   opDataRead,
		operands: []AnyResultRef{desiredInst, plannedVal, providerClient},
	})
}

func (b *Builder) EphemeralOpen(
	desiredInst ResultRef[*eval.DesiredResourceInstance],
	providerClient ResultRef[*exec.ProviderClient],
) ResourceInstanceResultRef {
	b.mu.Lock()
	defer b.mu.Unlock()

	return operationRef[*exec.ResourceInstanceObject](b, operationDesc{
		opCode:   opEphemeralOpen,
		operands: []AnyResultRef{desiredInst, providerClient},
	})
}

func (b *Builder) EphemeralClose(
	obj ResourceInstanceResultRef,
	providerClient ResultRef[*exec.ProviderClient],
	waitFor AnyResultRef,
) ResultRef[struct{}] {
	b.mu.Lock()
	defer b.mu.Unlock()

	waiter := b.ensureWaiterRef(waitFor)
	return operationRef[struct{}](b, operationDesc{
		opCode:   opEphemeralClose,
		operands: []AnyResultRef{obj, providerClient, waiter},
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
	b.mu.Lock()
	defer b.mu.Unlock()

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
// an argument to [Builder.Waiter] when explicitly representing the dependencies
// between different resource instances. The actual final state result for
// the source instance travels indirectly through the evaluator rather than
// directly within the execution graph.
//
// This function panics if a result reference for the given resource instance
// was not previously registered, because that suggests a bug elsewhere in the
// system that caused the construction of subgraphs for different resource
// instances to happen in the wrong order.
func (b *Builder) ResourceInstanceFinalStateResult(addr addrs.AbsResourceInstance) AnyResultRef {
	b.mu.Lock()
	defer b.mu.Unlock()

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
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.makeWaiter(dependencies)
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

// makeCloseBlocker is a helper used by [Builder] methods that produce
// open/close node pairs.
//
// Callers MUST hold a lock on b.mu throughout any call to this method.
func (b *Builder) makeCloseBlocker() (ResultRef[struct{}], RegisterCloseBlockerFunc) {
	idx := appendIndex(&b.graph.waiters, []AnyResultRef{})
	ref := waiterResultRef{idx}
	registerFunc := RegisterCloseBlockerFunc(func(ref AnyResultRef) {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.graph.waiters[idx] = append(b.graph.waiters[idx], ref)
	})
	return ref, registerFunc
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

type resultWithCloseBlockers[T any] struct {
	Result             ResultRef[T]
	CloseBlockerFunc   RegisterCloseBlockerFunc
	CloseBlockerResult ResultRef[struct{}]
}

// RegisterCloseBlockerFunc is the signature of a function that adds a given
// result references as a blocker for something to be "closed".
//
// Exactly what means to be a "close blocker" depends on context. Refer to the
// documentation of whatever function is returning a value of this type.
type RegisterCloseBlockerFunc func(AnyResultRef)
