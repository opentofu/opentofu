// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
)

////////////////////////////////////////////////////////////////////////////////
// This file contains methods of [execGraphBuilder] that are related to the
// parts of an execution graph that deal with provider instances.
////////////////////////////////////////////////////////////////////////////////

// SetProviderInstanceDependencies records the given set of resource instance
// addresses as the dependencies of the specified provider instance.
//
// This must be called at most once per distinct provider instance address, with
// the dependencies that were detected by the configuration evaluator. Duplicate
// calls for the same provider will panic.
func (b *execGraphBuilder) SetProviderInstanceDependencies(addr addrs.AbsProviderInstanceCorrect, deps addrs.Set[addrs.AbsResourceInstance]) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.openProviderDeps.Has(addr) {
		panic(fmt.Sprintf("duplicate call to SetProviderInstanceDependencies for %s", addr))
	}
	log.Printf("[TRACE] %s depends on %#v", addr, deps)
	b.openProviderDeps.Put(addr, deps)
}

// providerInstanceSubgraph generates the execution graph operations needed to
// obtain a configured client for a provider instance and ensure that the client
// stays open long enough to handle one or more other operations registered
// afterwards.
//
// This must only be called while already holding a lock on
// [execGraphBuilder.mu] from the reciever, and so is only for use as a helper
// for the exported methods of this type rather than for direct use by external
// callers.
//
// The given provider instance address must previously have been used in a call
// to [execGraphBuilder.SetProviderInstanceDependencies], or this will panic.
//
// The returned [registerExecCloseBlockerFunc] MUST be called with a reference
// to the result of the final operation in any linear chain of operations that
// depends on the provider to ensure that the provider will stay open at least
// long enough to perform those operations.
//
// Each distinct provider instance address gets only one set of operations
// added, so future calls with the same provider instance recieve references to
// the same operations. This means that if the resource instance planning code
// calls this only once it's definitely intending to add side-effects to the
// execution graph then the resulting graph will refer to only the subset of
// provider instances needed to perform planned changes.
func (b *execGraphBuilder) providerInstanceSubgraph(addr addrs.AbsProviderInstanceCorrect) (execgraph.ResultRef[*exec.ProviderClient], registerExecCloseBlockerFunc) {
	// We only register one index for each distinct provider instance address.
	if existing, ok := b.openProviderRefs.GetOk(addr); ok {
		return existing.Result, existing.CloseBlockerFunc
	}

	resourceInstDeps, ok := b.openProviderDeps.GetOk(addr)
	if !ok {
		panic(fmt.Sprintf("ProviderInstanceSubgraph for %s without earlier call to SetProviderInstanceDependencies", addr))
	}
	dependencyWaiter, closeDependencyAfter := b.waiterForResourceInstances(resourceInstDeps.All())

	addrResult := b.lower.ConstantProviderInstAddr(addr)
	configResult := b.lower.ProviderInstanceConfig(addrResult, dependencyWaiter)
	openResult := b.lower.ProviderInstanceOpen(configResult)
	closeWait, registerCloseBlocker := b.makeCloseBlocker()

	closeRef := b.lower.ProviderInstanceClose(openResult, closeWait)
	closeDependencyAfter(closeRef)

	b.openProviderRefs.Put(addr, execResultWithCloseBlockers[*exec.ProviderClient]{
		Result:             openResult,
		CloseBlockerResult: closeWait,
		CloseBlockerFunc:   registerCloseBlocker,
	})
	return openResult, registerCloseBlocker
}
