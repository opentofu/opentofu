// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
)

// ensureProviderInstanceExecgraph ensures that the given execution graph
// has the necessary entries to produce a client for the given provider
// instance address, and returns a result reference for that client.
//
// If the configuration for the given provider instance is invalid then this
// function only promises to do enough to produce a placeholder result that
// can be used elsewhere in execgraph construction, but with the possibility
// that the result will have incomplete dependency information. It's the
// evaluator's responsibility to report any errors in a provider instance's
// configuration, which will then in turn make the generated execution graph
// irrelevant because an errored plan cannot be applied. Our goal here, then, is
// to just provide sufficient information to allow the rest of the planning
// process to run to completion so we can capture as much information as
// possible to return a partial plan to help operators debug the problem.
func (p *planGlue) ensureProviderInstanceExecgraph(ctx context.Context, addr *addrs.AbsProviderInstanceCorrect, egb *execGraphBuilder) (execgraph.ResultRef[*exec.ProviderClient], registerExecCloseBlockerFunc) {
	if addr == nil {
		// A nil address currently represents that the configuration wasn't
		// complete, valid, or known enough to actually decide a specific
		// provider instance address, in which case we'll just return a
		// placeholder and assume that the reason for that will be dealt with
		// elsewhere, just so that the planning process can run to completion
		// to produce any relevant error messages.
		//
		// FIXME: Using a pointer that might be nil to represent "maybe unknown"
		// is confusing and hard to work with robustly. Maybe we can move the
		// [configgraph.Maybe] generic type into a different place so that we can
		// use that as our cross-cutting representation of "possibly unknown",
		// or find some other better way to represent this.
		return execgraph.NilResultRef[*exec.ProviderClient](), func(arr execgraph.AnyResultRef) {}
	}
	config := p.oracle.ProviderInstanceConfig(ctx, *addr)
	if config == nil {
		// We should only get here if the configuration is invalid, so in this
		// case we'll return a placeholder result that should be just enough
		// for the rest of the planning process to run to completion so that
		// we can then report to the user whatever errors caused this to be nil.
		return execgraph.NilResultRef[*exec.ProviderClient](), func(arr execgraph.AnyResultRef) {}
	}
	return egb.ProviderInstanceSubgraph(config)
}
