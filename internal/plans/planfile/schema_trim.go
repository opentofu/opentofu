// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planfile

import (
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
)

// referencedTypes tracks, for a single provider, which managed resource type
// names and data source type names are actually referenced somewhere in a
// plan (its configuration, prior state, or proposed changes).
// We dont need ephemeral as they dont get stored in the config
type referencedTypes struct {
	managed map[string]struct{}
	data    map[string]struct{}
}

func newReferencedTypes() *referencedTypes {
	return &referencedTypes{
		managed: make(map[string]struct{}),
		data:    make(map[string]struct{}),
	}
}

func (rt *referencedTypes) addType(mode addrs.ResourceMode, typeName string) {
	switch mode {
	case addrs.ManagedResourceMode:
		rt.managed[typeName] = struct{}{}
	case addrs.DataResourceMode:
		rt.data[typeName] = struct{}{}
	default:
	}
}

// referencedResourceTypes walks the given plan and configuration and
// determines, for each provider involved, which managed resource type names
// and data source type names are actually needed in order to fully render
// the plan (and the state snapshots it carries) as JSON.
func referencedResourceTypes(plan *plans.Plan, config *configs.Config) map[addrs.Provider]*referencedTypes {
	result := make(map[addrs.Provider]*referencedTypes)

	get := func(provider addrs.Provider) *referencedTypes {
		rt, ok := result[provider]
		if !ok {
			rt = newReferencedTypes()
			result[provider] = rt
		}
		return rt
	}

	if config != nil {
		for _, provider := range config.ProviderTypes() {
			get(provider)
		}

		// Referenced from the configuration snapshot embedded in the plan.
		for _, c := range config.AllModules() {
			for _, r := range c.Module.ManagedResources {
				get(r.Provider).addType(r.Mode, r.Type)
			}
			for _, r := range c.Module.DataResources {
				get(r.Provider).addType(r.Mode, r.Type)
			}
		}
	}

	addProviderAddrs := func(state *states.State) {
		if state == nil {
			return
		}
		for _, providerAddr := range state.ProviderAddrs() {
			get(providerAddr.Provider)
		}
	}
	addProviderAddrs(plan.PriorState)
	addProviderAddrs(plan.PrevRunState)

	addFromState := func(state *states.State) {
		if state == nil {
			return
		}
		for _, mod := range state.Modules {
			for _, res := range mod.Resources {
				get(res.ProviderConfig.Provider).addType(res.Addr.Resource.Mode, res.Addr.Resource.Type)
			}
		}
	}
	addFromState(plan.PriorState)
	addFromState(plan.PrevRunState)

	if plan.Changes != nil {
		for _, rc := range plan.Changes.Resources {
			get(rc.ProviderAddr.Provider).addType(rc.Addr.Resource.Resource.Mode, rc.Addr.Resource.Resource.Type)
		}
	}

	return result
}

// trimSchemas takes the full set of provider schemas that were used to
// build the given plan and produces a reduced copy containing only the
// parts that are needed to render that specific plan as JSON: each
// referenced provider's own configuration schema, plus the schemas (and
// resource identity schemas) for whichever managed resource types and data
// sources are actually referenced.
//
// trimSchemas is all-or-nothing: if schemas is nil, or doesn't fully cover
// everything referencedResourceTypes says is needed (e.g. a provider is
// missing entirely, or one of its referenced resource/data source types
// has no schema), trimSchemas returns nil rather than a partial result.
func trimSchemas(plan *plans.Plan, config *configs.Config, schemas map[addrs.Provider]providers.ProviderSchema) map[addrs.Provider]providers.ProviderSchema {
	if schemas == nil {
		return nil
	}

	needed := referencedResourceTypes(plan, config)
	if len(needed) == 0 {
		return nil
	}

	var missing []string
	result := make(map[addrs.Provider]providers.ProviderSchema, len(needed))
	for provider, rt := range needed {
		full, ok := schemas[provider]
		if !ok {
			missing = append(missing, fmt.Sprintf("provider %s", provider))
			continue
		}

		trimmed := providers.ProviderSchema{
			Provider:      full.Provider,
			ResourceTypes: make(map[string]providers.Schema, len(rt.managed)),
			DataSources:   make(map[string]providers.Schema, len(rt.data)),
		}

		for typeName := range rt.managed {
			trimmed.ResourceTypes[typeName], ok = full.ResourceTypes[typeName]
			if !ok {
				missing = append(missing, fmt.Sprintf("managed resource type %q of provider %s", typeName, provider))
			}
		}
		for typeName := range rt.data {
			trimmed.DataSources[typeName], ok = full.DataSources[typeName]
			if !ok {
				missing = append(missing, fmt.Sprintf("data source %q of provider %s", typeName, provider))
			}
		}

		result[provider] = trimmed
	}

	if len(missing) > 0 {
		for _, m := range missing {
			log.Printf("[TRACE] planfile: no schema for %s; can't embed schemas for this plan", m)
		}
		return nil
	}

	return result
}
