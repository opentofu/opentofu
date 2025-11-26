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
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
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
	desiredStateRefs       addrs.Map[addrs.AbsResourceInstance, ResultRef[*eval.DesiredResourceInstance]]
	priorStateRefs         addrs.Map[addrs.AbsResourceInstance, ResultRef[*states.ResourceInstanceObjectFull]]
	providerAddrRefs       map[addrs.Provider]ResultRef[addrs.Provider]
	providerInstConfigRefs addrs.Map[addrs.AbsProviderInstanceCorrect, ResultRef[cty.Value]]
	openProviderRefs       addrs.Map[addrs.AbsProviderInstanceCorrect, resultWithCloseBlockers[providers.Configured]]
	emptyWaiterRef         ResultRef[struct{}]
}

func NewBuilder() *Builder {
	return &Builder{
		graph: &Graph{
			resourceInstanceResults: addrs.MakeMap[addrs.AbsResourceInstance, ResultRef[*states.ResourceInstanceObjectFull]](),
		},
		desiredStateRefs:       addrs.MakeMap[addrs.AbsResourceInstance, ResultRef[*eval.DesiredResourceInstance]](),
		priorStateRefs:         addrs.MakeMap[addrs.AbsResourceInstance, ResultRef[*states.ResourceInstanceObjectFull]](),
		providerAddrRefs:       make(map[addrs.Provider]ResultRef[addrs.Provider]),
		providerInstConfigRefs: addrs.MakeMap[addrs.AbsProviderInstanceCorrect, ResultRef[cty.Value]](),
		openProviderRefs:       addrs.MakeMap[addrs.AbsProviderInstanceCorrect, resultWithCloseBlockers[providers.Configured]](),
		emptyWaiterRef:         nil, // will be populated on first request for an empty waiter
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

// ConstantValue adds a constant [addrs.Provider] address as a source node.
// The result can be used as an operand to a subsequent operation.
//
// Multiple calls with the same provider address all return the same result,
// so in practice each distinct provider address is stored only once.
func (b *Builder) ConstantProviderAddr(addr addrs.Provider) ResultRef[addrs.Provider] {
	b.mu.Lock()
	defer b.mu.Unlock()

	if existing, ok := b.providerAddrRefs[addr]; ok {
		return existing
	}
	idx := appendIndex(&b.graph.providerAddrs, addr)
	return providerAddrResultRef{idx}
}

func (b *Builder) DesiredResourceInstance(addr addrs.AbsResourceInstance) ResultRef[*eval.DesiredResourceInstance] {
	b.mu.Lock()
	defer b.mu.Unlock()

	// We only register one index for each distinct resource instance address.
	if existing, ok := b.desiredStateRefs.GetOk(addr); ok {
		return existing
	}
	idx := appendIndex(&b.graph.desiredStateRefs, addr)
	ret := desiredResourceInstanceResultRef{idx}
	b.desiredStateRefs.Put(addr, ret)
	return ret
}

// ResourceInstancePriorState returns a source node whose result will be
// the prior state resource instance object for the "current" (i.e. not deposed)
// object associated given resource instance address, if any.
//
// NOTE: This is currently using states.ResourceInstanceObject from our existing
// state model, but a real implementation of this might benefit from a slightly
// different model tailored to be used in isolation, without the rest of the
// state tree it came from.
func (b *Builder) ResourceInstancePriorState(addr addrs.AbsResourceInstance) ResultRef[*states.ResourceInstanceObjectFull] {
	b.mu.Lock()
	defer b.mu.Unlock()

	// We only register one index for each distinct resource instance address.
	if existing, ok := b.priorStateRefs.GetOk(addr); ok {
		return existing
	}
	idx := appendIndex(&b.graph.priorStateRefs, resourceInstanceStateRef{
		ResourceInstance: addr,
		DeposedKey:       states.NotDeposed,
	})
	ret := resourceInstancePriorStateResultRef{idx}
	b.priorStateRefs.Put(addr, ret)
	return ret
}

// ResourceDeposedObjectState is like [Builder.ResourceInstancePriorState] but
// produces the state for a deposed object currently associated with a resource
// instance, rather than its "current" object.
//
// Unlike [Builder.ResourceInstancePriorState] this registers an entirely new
// result for each call, with the expectation that there will only be one
// codepath attempting to register the chain of nodes for any deposed object,
// and no resource instance should depend on the result of applying changes
// to a deposed object.
func (b *Builder) ResourceDeposedObjectState(instAddr addrs.AbsResourceInstance, deposedKey states.DeposedKey) ResultRef[*states.ResourceInstanceObjectFull] {
	b.mu.Lock()
	defer b.mu.Unlock()

	idx := appendIndex(&b.graph.priorStateRefs, resourceInstanceStateRef{
		ResourceInstance: instAddr,
		DeposedKey:       deposedKey,
	})
	ret := resourceInstancePriorStateResultRef{idx}
	return ret
}

// ProviderInstanceConfig registers a request to obtain the configuration for
// a specific provider instance, returning a reference to its [cty.Value]
// result representing the evaluated configuration.
//
// In most cases callers should use [Builder.ProviderInstance] to obtain a
// preconfigured client for the provider instance, which deals with getting
// the provider instance configuration as part of its work.
func (b *Builder) ProviderInstanceConfig(addr addrs.AbsProviderInstanceCorrect) ResultRef[cty.Value] {
	b.mu.Lock()
	defer b.mu.Unlock()

	// We only register one index for each distinct provider instance address.
	if existing, ok := b.providerInstConfigRefs.GetOk(addr); ok {
		return existing
	}
	idx := appendIndex(&b.graph.providerInstConfigRefs, addr)
	ret := providerInstanceConfigResultRef{idx}
	b.providerInstConfigRefs.Put(addr, ret)
	return ret
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
func (b *Builder) ProviderInstance(addr addrs.AbsProviderInstanceCorrect, waitFor AnyResultRef) (ResultRef[providers.Configured], RegisterCloseBlockerFunc) {
	configResult := b.ProviderInstanceConfig(addr)
	providerAddrResult := b.ConstantProviderAddr(addr.Config.Config.Provider)
	waiter := b.ensureWaiterRef(waitFor)

	b.mu.Lock()
	defer b.mu.Unlock()

	// We only register one index for each distinct provider instance address.
	if existing, ok := b.openProviderRefs.GetOk(addr); ok {
		return existing.Result, existing.CloseBlockerFunc
	}
	openResult := operationRef[providers.Configured](b, operationDesc{
		opCode:   opOpenProvider,
		operands: []AnyResultRef{providerAddrResult, configResult, waiter},
	})
	closeWait, registerCloseBlocker := b.makeCloseBlocker()
	// Nothing actually depends on the result of the "close" operation, but
	// eventual execution of the graph will still wait for it to complete
	// because _all_ operations must complete before execution is considered
	// to be finished.
	_ = operationRef[struct{}](b, operationDesc{
		opCode:   opCloseProvider,
		operands: []AnyResultRef{openResult, closeWait},
	})
	return openResult, registerCloseBlocker
}

// OpenProviderClient registers an operation for opening a client for a
// particular provider instance.
//
// Direct callers should typically use [Builder.ProviderInstance] instead,
// because it automatically deduplicates requests for the same provider
// instance and registers the associated "close" operation for the provider
// instance. This method is here primarily for the benefit of [UnmarshalGraph],
// which needs to be able to work with the individual operation nodes when
// reconstructing a previously-marshaled graph.
func (b *Builder) OpenProviderClient(providerAddr ResultRef[addrs.Provider], config ResultRef[cty.Value], waitFor AnyResultRef) ResultRef[providers.Configured] {
	b.mu.Lock()
	defer b.mu.Unlock()

	waiter := b.ensureWaiterRef(waitFor)
	return operationRef[providers.Configured](b, operationDesc{
		opCode:   opOpenProvider,
		operands: []AnyResultRef{providerAddr, config, waiter},
	})
}

// CloseProviderClient registers an operation for closing a provider client
// that was previously opened through [Builder.OpenProviderClient].
//
// Direct callers should typically use [Builder.ProviderInstance] instead,
// because it deals with all of the ceremony of opening and closing provider
// clients. This method is here primarily for the benefit of [UnmarshalGraph],
// which needs to be able to work with the individual operation nodes when
// reconstructing a previously-marshaled graph.
func (b *Builder) CloseProviderClient(clientResult ResultRef[providers.Configured], waitFor AnyResultRef) ResultRef[struct{}] {
	b.mu.Lock()
	defer b.mu.Unlock()

	waiter := b.ensureWaiterRef(waitFor)
	return operationRef[struct{}](b, operationDesc{
		opCode:   opCloseProvider,
		operands: []AnyResultRef{clientResult, waiter},
	})
}

// ManagedResourceObjectFinalPlan registers an operation to decide the "final plan" for a managed
// resource instance object, which may or may not be "desired".
//
// If the object is not "desired" then the desiredInst result is a
// [NilResultRef], producing a nil pointer. The underlying provider API
// represents that situation by setting the "configuration value" to null.
//
// Similarly, if the object did not previously exist but is now desired then
// the priorState result is a [NilResultRef] producing a nil pointer, which
// should be represented in the provider API by setting the prior state
// value to null.
//
// If the planning phase learned that the provider needs to handle a change
// as a "replace" then in the execution graph there should be two separate
// "final plan" and "apply changes" chains, where one has a nil desiredInst
// and the other has a nil priorState. desiredInst and priorState should only
// both be set when handling an in-place update.
//
// The waitFor argument captures arbitrary additional results that the
// operation should block on even though it doesn't directly consume their
// results. In practice this should refer to the final results of applying
// any resource instances that this object depends on according to the
// resource-instance-graph calculated during the planning process, thereby
// ensuring that a particular object cannot be final-planned until all of its
// resource-instance-graph dependencies have had their changes applied.
func (b *Builder) ManagedResourceObjectFinalPlan(
	desiredInst ResultRef[*eval.DesiredResourceInstance],
	priorState ResultRef[*states.ResourceInstanceObjectFull],
	plannedVal ResultRef[cty.Value],
	providerClient ResultRef[providers.Configured],
	waitFor AnyResultRef,
) ResultRef[*ManagedResourceObjectFinalPlan] {
	b.mu.Lock()
	defer b.mu.Unlock()

	waiter := b.ensureWaiterRef(waitFor)
	return operationRef[*ManagedResourceObjectFinalPlan](b, operationDesc{
		opCode:   opManagedFinalPlan,
		operands: []AnyResultRef{desiredInst, priorState, plannedVal, providerClient, waiter},
	})
}

// ApplyManagedResourceObjectChanges registers an operation to apply a "final
// plan" for a managed resource instance object.
//
// The finalPlan argument should typically be something returned by a previous
// call to [Builder.ManagedResourceObjectFinalPlan] with the same provider
// client.
func (b *Builder) ApplyManagedResourceObjectChanges(
	finalPlan ResultRef[*ManagedResourceObjectFinalPlan],
	providerClient ResultRef[providers.Configured],
) ResultRef[*states.ResourceInstanceObjectFull] {
	b.mu.Lock()
	defer b.mu.Unlock()

	return operationRef[*states.ResourceInstanceObjectFull](b, operationDesc{
		opCode:   opManagedApplyChanges,
		operands: []AnyResultRef{finalPlan, providerClient},
	})
}

func (b *Builder) DataRead(
	desiredInst ResultRef[*eval.DesiredResourceInstance],
	providerClient ResultRef[providers.Configured],
	waitFor AnyResultRef,
) ResultRef[*states.ResourceInstanceObjectFull] {
	b.mu.Lock()
	defer b.mu.Unlock()

	waiter := b.ensureWaiterRef(waitFor)
	return operationRef[*states.ResourceInstanceObjectFull](b, operationDesc{
		opCode:   opDataRead,
		operands: []AnyResultRef{desiredInst, providerClient, waiter},
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
func (b *Builder) SetResourceInstanceFinalStateResult(addr addrs.AbsResourceInstance, result ResultRef[*states.ResourceInstanceObjectFull]) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.graph.resourceInstanceResults.Has(addr) {
		panic(fmt.Sprintf("duplicate registration for %s final state result", addr))
	}
	b.graph.resourceInstanceResults.Put(addr, result)
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
