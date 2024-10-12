// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"

	"github.com/opentofu/opentofu/internal/dag"
)

// concreteProviderInstanceNodeFunc is a callback type used to convert an
// abstract provider to a concrete one of some type.
type concreteProviderInstanceNodeFunc func(*nodeAbstractProviderInstance) dag.Vertex

// nodeAbstractProviderInstance represents a provider that has no associated operations.
// It registers all the common interfaces across operations for providers.
//
// This should be instantiated only by [nodeProvider.DynamicExpand], which is where we
// have the context required to know how to populate all of the fields consistently.
type nodeAbstractProviderInstance struct {
	Addr addrs.AbsProviderInstance

	// Config is the configuration block this instance was declared by.
	//
	// Many different instances can potentially share the same configuration block
	// if either the provider configuration is inside a dynamic-expanded module
	// or the provider block itself uses for_each.
	Config *configs.Provider

	// Schema is the configuration for the provider-specific configuration that
	// should be written inside the HCL body inside field Config.
	Schema *configschema.Block
}

var (
	_ GraphNodeModulePath       = (*nodeAbstractProviderInstance)(nil)
	_ GraphNodeReferencer       = (*nodeAbstractProviderInstance)(nil)
	_ GraphNodeProviderInstance = (*nodeAbstractProviderInstance)(nil)
	_ dag.GraphNodeDotter       = (*nodeAbstractProviderInstance)(nil)
)

func (n *nodeAbstractProviderInstance) Name() string {
	return n.Addr.String()
}

// GraphNodeModuleInstance
func (n *nodeAbstractProviderInstance) Path() addrs.ModuleInstance {
	return n.Addr.Module
}

// GraphNodeModulePath
func (n *nodeAbstractProviderInstance) ModulePath() addrs.Module {
	return n.Addr.Module.Module()
}

// ProviderInstanceAddr implements GraphNodeProviderInstance.
func (n *nodeAbstractProviderInstance) ProviderInstanceAddr() addrs.AbsProviderInstance {
	return n.Addr
}

// GraphNodeReferencer
func (n *nodeAbstractProviderInstance) References() []*addrs.Reference {
	if n.Config == nil || n.Schema == nil {
		return nil
	}

	return ReferencesFromConfig(n.Config.Config, n.Schema)
}

// GraphNodeProvider
func (n *nodeAbstractProviderInstance) ProviderConfig() *configs.Provider {
	if n.Config == nil {
		return nil
	}

	return n.Config
}

// GraphNodeDotter impl.
func (n *nodeAbstractProviderInstance) DotNode(name string, opts *dag.DotOpts) *dag.DotNode {
	return &dag.DotNode{
		Name: name,
		Attrs: map[string]string{
			"label": n.Name(),
			"shape": "diamond",
		},
	}
}
