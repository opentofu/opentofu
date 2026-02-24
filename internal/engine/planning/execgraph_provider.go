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

// ProviderInstanceSubgraph generates the execution graph operations needed to
// obtain a configured client for a provider instance and ensure that the client
// stays open long enough to handle one or more other operations registered
// afterwards.
//
// Each call to this method adds a new set of operations to the graph. It's
// the caller's responsibility to call this function only once per distinct
// provider instance address.
func (b *execGraphBuilder) ProviderInstanceSubgraph(
	addr addrs.AbsProviderInstanceCorrect,
) (
	clientRef execgraph.ResultRef[*exec.ProviderClient],
	addConfigDep, addCloseDep func(execgraph.AnyResultRef),
) {
	addrResult := b.lower.ConstantProviderInstAddr(addr)
	waitFor, addConfigDep := b.lower.MutableWaiter()
	configResult := b.lower.ProviderInstanceConfig(addrResult, waitFor)
	openResult := b.lower.ProviderInstanceOpen(configResult)

	closeWait, addCloseDep := b.makeCloseBlocker()
	b.lower.ProviderInstanceClose(openResult, closeWait)

	return openResult, addConfigDep, addCloseDep
}
