// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
)

////////////////////////////////////////////////////////////////////////////////
// This file contains methods of [execGraphBuilder] that are related to the
// parts of an execution graph that deal with provider instances.
////////////////////////////////////////////////////////////////////////////////

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
func (b *execGraphBuilder) ProviderInstance(addr addrs.AbsProviderInstanceCorrect, waitFor execgraph.AnyResultRef) (execgraph.ResultRef[*exec.ProviderClient], registerExecCloseBlockerFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// FIXME: This is an adaptation of an earlier attempt at this where this
	// helper was in the underlying execgraph.Builder type, built to work
	// without any help from the planning engine. But this design isn't
	// really suitable because it makes the insertion of a provider instance
	// be an implicit side-effect of planning whatever resource instance happens
	// to use the provider first, and the planning code for a resource instance
	// doesn't know information like which other resource instances the
	// provider instance's configuration depends on, etc.
	//
	// In future commits we should rework things so that we have an explicit
	// separate step in the planning process of preparing the provider instance
	// based on its representation in the configuration, and then the planning
	// of resource instances would just retrieve the result ref for the "open"
	// operation that was already registered earlier, instead of implicitly
	// causing that operation to be added. At that point this method would
	// take only the provider instance address and not the "waitFor" reference,
	// because it would only ever be returning a reference to something already
	// in the graph and never adding any new operations itself.

	addrResult := b.lower.ConstantProviderInstAddr(addr)

	// We only register one index for each distinct provider instance address.
	if existing, ok := b.openProviderRefs.GetOk(addr); ok {
		return existing.Result, existing.CloseBlockerFunc
	}
	configResult := b.lower.ProviderInstanceConfig(addrResult, waitFor)
	openResult := b.lower.ProviderInstanceOpen(configResult)
	closeWait, registerCloseBlocker := b.makeCloseBlocker()
	// Nothing actually depends on the result of the "close" operation, but
	// eventual execution of the graph will still wait for it to complete
	// because _all_ operations must complete before execution is considered
	// to be finished.
	_ = b.lower.ProviderInstanceClose(openResult, closeWait)
	b.openProviderRefs.Put(addr, execResultWithCloseBlockers[*exec.ProviderClient]{
		Result:             openResult,
		CloseBlockerResult: closeWait,
		CloseBlockerFunc:   registerCloseBlocker,
	})
	return openResult, registerCloseBlocker
}
