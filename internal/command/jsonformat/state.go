// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonformat

import (
	"fmt"
	"sort"

	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/command/jsonformat/computed"
	"github.com/opentofu/opentofu/internal/command/jsonformat/differ"
	"github.com/opentofu/opentofu/internal/command/jsonformat/structured"
	"github.com/opentofu/opentofu/internal/command/jsonprovider"
	"github.com/opentofu/opentofu/internal/command/jsonstate"
)

type State struct {
	StateFormatVersion string                      `json:"state_format_version"`
	RootModule         jsonstate.Module            `json:"root"`
	RootModuleOutputs  map[string]jsonstate.Output `json:"root_module_outputs"`

	ProviderFormatVersion string                            `json:"provider_format_version"`
	ProviderSchemas       map[string]*jsonprovider.Provider `json:"provider_schemas"`
}

func (state State) Empty() bool {
	return len(state.RootModuleOutputs) == 0 && len(state.RootModule.Resources) == 0 && len(state.RootModule.ChildModules) == 0
}

func (state State) GetSchema(resource jsonstate.Resource) *jsonprovider.Schema {
	switch resource.Mode {
	case jsonstate.ManagedResourceMode:
		return state.ProviderSchemas[resource.ProviderName].ResourceSchemas[resource.Type]
	case jsonstate.DataResourceMode:
		return state.ProviderSchemas[resource.ProviderName].DataSourceSchemas[resource.Type]
	case jsonstate.EphemeralResourceMode:
		panic(fmt.Errorf("ephemeral resources are not meant to be stored in the state file but schema for ephemeral %s.%s has been requested", resource.Type, resource.Name))
	default:
		panic("found unrecognized resource mode: " + resource.Mode)
	}
}

func (state State) renderHumanStateModule(renderer Renderer, module jsonstate.Module, opts computed.RenderHumanOpts, first bool) {
	if len(module.Resources) > 0 && !first {
		renderer.Streams.Println()
	}

	for _, resource := range module.Resources {

		if !first {
			renderer.Streams.Println()
		}

		if first {
			first = false
		}

		if len(resource.DeposedKey) > 0 {
			renderer.Streams.Printf("# %s: (deposed object %s)", resource.Address, resource.DeposedKey)
		} else if resource.Tainted {
			renderer.Streams.Printf("# %s: (tainted)", resource.Address)
		} else {
			renderer.Streams.Printf("# %s:", resource.Address)
		}

		renderer.Streams.Println()

		schema := state.GetSchema(resource)
		switch resource.Mode {
		case jsonstate.ManagedResourceMode:
			change := structured.FromJsonResource(resource)
			renderer.Streams.Printf("resource %q %q %s", resource.Type, resource.Name, differ.ComputeDiffForBlock(change, schema.Block).RenderHuman(0, opts))
		case jsonstate.DataResourceMode:
			change := structured.FromJsonResource(resource)
			renderer.Streams.Printf("data %q %q %s", resource.Type, resource.Name, differ.ComputeDiffForBlock(change, schema.Block).RenderHuman(0, opts))
		case jsonstate.EphemeralResourceMode:
			panic(fmt.Errorf("ephemeral resource %s %s not allowed to be stored in the state", resource.Type, resource.Name))
		default:
			panic("found unrecognized resource mode: " + resource.Mode)
		}

		renderer.Streams.Println()
	}

	for _, child := range module.ChildModules {
		state.renderHumanStateModule(renderer, child, opts, first)
	}
}

func (state State) renderHumanStateOutputs(renderer Renderer, opts computed.RenderHumanOpts) {

	if len(state.RootModuleOutputs) > 0 {
		renderer.Streams.Printf("\n\nOutputs:\n\n")

		var keys []string
		for key := range state.RootModuleOutputs {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			output := state.RootModuleOutputs[key]
			change := structured.FromJsonOutput(output)
			ctype, err := ctyjson.UnmarshalType(output.Type)
			if err != nil {
				// We can actually do this without the type, so even if we fail
				// to work out the type let's just render this anyway.
				renderer.Streams.Printf("%s = %s\n", key, differ.ComputeDiffForOutput(change).RenderHuman(0, opts))
			} else {
				renderer.Streams.Printf("%s = %s\n", key, differ.ComputeDiffForType(change, ctype).RenderHuman(0, opts))
			}
		}
	}
}
