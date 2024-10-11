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
type nodeAbstractProviderInstance struct {
	Addr addrs.ConfigProviderInstance

	// The fields below will be automatically set using the Attach
	// interfaces if you're running those transforms, but also be explicitly
	// set if you already have that information.

	Config *configs.Provider
	Schema *configschema.Block
}

var (
	_ GraphNodeModulePath                 = (*nodeAbstractProviderInstance)(nil)
	_ GraphNodeReferencer                 = (*nodeAbstractProviderInstance)(nil)
	_ GraphNodeProviderInstance           = (*nodeAbstractProviderInstance)(nil)
	_ GraphNodeAttachProvider             = (*nodeAbstractProviderInstance)(nil)
	_ GraphNodeAttachProviderConfigSchema = (*nodeAbstractProviderInstance)(nil)
	_ dag.GraphNodeDotter                 = (*nodeAbstractProviderInstance)(nil)
)

func (n *nodeAbstractProviderInstance) Name() string {
	return n.Addr.String()
}

// GraphNodeModuleInstance
func (n *nodeAbstractProviderInstance) Path() addrs.ModuleInstance {
	// Providers cannot be contained inside an expanded module, so this shim
	// converts our module path to the correct ModuleInstance.
	return n.Addr.Module.UnkeyedInstanceShim()
}

// GraphNodeModulePath
func (n *nodeAbstractProviderInstance) ModulePath() addrs.Module {
	return n.Addr.Module
}

// GraphNodeReferencer
func (n *nodeAbstractProviderInstance) References() []*addrs.Reference {
	if n.Config == nil || n.Schema == nil {
		return nil
	}

	return ReferencesFromConfig(n.Config.Config, n.Schema)
}

// GraphNodeProvider
func (n *nodeAbstractProviderInstance) ProviderAddr() addrs.ConfigProviderInstance {
	return n.Addr
}

// GraphNodeProvider
func (n *nodeAbstractProviderInstance) ProviderConfig() *configs.Provider {
	if n.Config == nil {
		return nil
	}

	return n.Config
}

// GraphNodeAttachProvider
func (n *nodeAbstractProviderInstance) AttachProvider(c *configs.Provider) {
	n.Config = c
}

// GraphNodeAttachProviderConfigSchema impl.
func (n *nodeAbstractProviderInstance) AttachProviderConfigSchema(schema *configschema.Block) {
	n.Schema = schema
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
