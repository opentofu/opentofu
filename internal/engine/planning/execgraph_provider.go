// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
	"github.com/opentofu/opentofu/internal/lang/eval"
)

////////////////////////////////////////////////////////////////////////////////
// This file contains methods of [execGraphBuilder] that are related to the
// parts of an execution graph that deal with provider instances.
////////////////////////////////////////////////////////////////////////////////

// ProviderInstanceSubgraph generates the execution graph operations needed to
// obtain a configured client for a provider instance and ensure that the client
// stays open long enough to handle one or more other operations registered
// afterwards.
//
// This should be called ONLY by [planGlue.ensureProviderInstanceExecgraph],
// which therefore ensures that all calls will consistently pass the same config
// for each distinct provider instance address.
//
// Each distinct provider instance address gets only one set of operations
// added, so future calls with the same provider instance recieve references to
// the same operations. This means that the resource instance planning code must
// call this only once it's definitely intending to add side-effects to the
// execution graph then the resulting graph will refer to only the subset of
// provider instances needed to perform planned changes.
func (b *execGraphBuilder) ProviderInstanceSubgraph(config *eval.ProviderInstanceConfig) (execgraph.ResultRef[*exec.ProviderClient], registerExecCloseBlockerFunc) {
	// We only register one index for each distinct provider instance address.
	if existing, ok := b.openProviderRefs.GetOk(config.Addr); ok {
		return existing.Result, existing.CloseBlockerFunc
	}

	resourceInstDeps := config.RequiredResourceInstances
	dependencyWaiter, closeDependencyAfter := b.waiterForResourceInstances(resourceInstDeps.All())

	addrResult := b.lower.ConstantProviderInstAddr(config.Addr)
	configResult := b.lower.ProviderInstanceConfig(addrResult, dependencyWaiter)
	openResult := b.lower.ProviderInstanceOpen(configResult)
	closeWait, registerCloseBlocker := b.makeCloseBlocker()

	closeRef := b.lower.ProviderInstanceClose(openResult, closeWait)
	closeDependencyAfter(closeRef)

	b.openProviderRefs.Put(config.Addr, execResultWithCloseBlockers[*exec.ProviderClient]{
		Result:             openResult,
		CloseBlockerResult: closeWait,
		CloseBlockerFunc:   registerCloseBlocker,
	})
	return openResult, registerCloseBlocker
}
