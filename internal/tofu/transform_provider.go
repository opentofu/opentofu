// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/dag"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func transformProviders(concrete concreteProviderInstanceNodeFunc, config *configs.Config) GraphTransformer {
	return GraphTransformMulti(
		// Add providers from the config
		&providerConfigTransformer{
			config:           config,
			concreteProvider: concrete,
		},
	)
}

// GraphNodeProviderInstance is implemented by node types where each node represents
// launching and configuring a single instance of a provider.
type GraphNodeProviderInstance interface {
	GraphNodeModuleInstance
	ProviderInstanceAddr() addrs.AbsProviderInstance
}

// GraphNodeProviderInstanceClose is implemented by node types where each node
// represents shutting down a single instance of a provider that was previously
// started by a [GraphNodeProviderInstance] implementation.
type GraphNodeProviderInstanceClose interface {
	GraphNodeModuleInstance
	CloseProviderInstanceAddr() addrs.AbsProviderInstance
}

type GraphNodeProviderInstanceConsumer interface {
	GraphNodeModuleInstance

	// ProvidedBy returns the address of the provider configuration the node
	// refers to, if available. The following value types may be returned:
	//
	//   nil + exact true: the node does not require a provider
	// * addrs.LocalProviderInstance: the provider was set in the resource config
	// * addrs.AbsProviderInstance + exact true: the provider configuration was
	//   taken from the instance state.
	// * addrs.AbsProviderInstance + exact false: no config or state; the returned
	//   value is a default provider configuration address for the resource's
	//   Provider
	ProvidedBy() (addr addrs.LocalOrAbsProvider, exact bool)

	// Provider() returns the Provider FQN for the node.
	Provider() (provider addrs.Provider)

	// Set the resolved provider instance address for this resource.
	SetProviderInstance(addrs.AbsProviderInstance)
}

type graphNodeCloseProviderInstance struct {
	Addr addrs.AbsProviderInstance
}

var (
	_ GraphNodeProviderInstanceClose = (*graphNodeCloseProviderInstance)(nil)
	_ GraphNodeExecutable            = (*graphNodeCloseProviderInstance)(nil)
)

func (n *graphNodeCloseProviderInstance) Name() string {
	return n.Addr.String() + " (close)"
}

// GraphNodeModulePath
func (n *graphNodeCloseProviderInstance) Path() addrs.ModuleInstance {
	return n.Addr.Module
}

// GraphNodeExecutable impl.
func (n *graphNodeCloseProviderInstance) Execute(ctx EvalContext, op walkOperation) (diags tfdiags.Diagnostics) {
	return diags.Append(ctx.CloseProvider(n.Addr))
}

func (n *graphNodeCloseProviderInstance) CloseProviderInstanceAddr() addrs.AbsProviderInstance {
	return n.Addr
}

// GraphNodeDotter impl.
func (n *graphNodeCloseProviderInstance) DotNode(name string, opts *dag.DotOpts) *dag.DotNode {
	if !opts.Verbose {
		return nil
	}
	return &dag.DotNode{
		Name: name,
		Attrs: map[string]string{
			"label": n.Name(),
			"shape": "diamond",
		},
	}
}
