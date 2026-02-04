// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
)

// ensureProviderInstanceDependencies ensures that the given execution graph
// has the necessary entries to satisfy the dependencies for the given provider
// instance address, and records those dependencies for use by a subsequent
// call to [execGraphBuilder.ProviderInstance].
//
// This MUST be called at least once before calling
// [execGraphBuilder.ProviderInstanceSubgraph] with the same provider address,
// or the call to the latter function will panic.
//
// If the configuration for the given provider instance is invalid then this
// function only promises to do enough for a successful subsequent call to
// [execGraphBuilder.ProviderInstanceSubgraph], but with the possibility that
// the result will have incomplete dependency information. It's the evaluator's
// responsibility to report any errors in a provider instance's configuration,
// which will then in turn make the generated execution graph irrelevant because
// an errored plan cannot be applied. Our goal here, then, is to just provide
// sufficient information to allow the rest of the planning process to run to
// completion so we can capture as much information as possible to return
// a partial plan to help operators debug the problem.
func (p *planGlue) ensureProviderInstanceDependencies(ctx context.Context, addr addrs.AbsProviderInstanceCorrect, egb *execGraphBuilder) {
	var requiredResourceInsts addrs.Set[addrs.AbsResourceInstance]
	config := p.oracle.ProviderInstanceConfig(ctx, addr)
	if config != nil {
		requiredResourceInsts = config.RequiredResourceInstances
	}
	egb.SetProviderInstanceDependencies(addr, requiredResourceInsts)
}
