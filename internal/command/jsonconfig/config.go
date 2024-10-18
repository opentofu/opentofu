// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonconfig

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/tofu"
)

// Config represents the complete configuration source
type config struct {
	ProviderConfigs map[string]providerConfig `json:"provider_config,omitempty"`
	RootModule      module                    `json:"root_module,omitempty"`
}

// ProviderConfig describes all of the provider configurations throughout the
// configuration tree, flattened into a single map for convenience since
// provider configurations are the one concept in OpenTofu that can span across
// module boundaries.
type providerConfig struct {
	Name              string                 `json:"name,omitempty"`
	FullName          string                 `json:"full_name,omitempty"`
	Alias             string                 `json:"alias,omitempty"`
	VersionConstraint string                 `json:"version_constraint,omitempty"`
	ModuleAddress     string                 `json:"module_address,omitempty"`
	Expressions       map[string]interface{} `json:"expressions,omitempty"`
	parentKeys        []string
}

type module struct {
	Outputs map[string]output `json:"outputs,omitempty"`
	// Resources are sorted in a user-friendly order that is undefined at this
	// time, but consistent.
	Resources   []resource            `json:"resources,omitempty"`
	ModuleCalls map[string]moduleCall `json:"module_calls,omitempty"`
	Variables   variables             `json:"variables,omitempty"`
}

type moduleCall struct {
	Source            string                 `json:"source,omitempty"`
	Expressions       map[string]interface{} `json:"expressions,omitempty"`
	CountExpression   *expression            `json:"count_expression,omitempty"`
	ForEachExpression *expression            `json:"for_each_expression,omitempty"`
	Module            module                 `json:"module,omitempty"`
	VersionConstraint string                 `json:"version_constraint,omitempty"`
	DependsOn         []string               `json:"depends_on,omitempty"`
}

// variables is the JSON representation of the variables provided to the current
// plan.
type variables map[string]*variable

type variable struct {
	Default     json.RawMessage `json:"default,omitempty"`
	Description string          `json:"description,omitempty"`
	Sensitive   bool            `json:"sensitive,omitempty"`
}

// Resource is the representation of a resource in the config
type resource struct {
	// Address is the absolute resource address
	Address string `json:"address,omitempty"`

	// Mode can be "managed" or "data"
	Mode string `json:"mode,omitempty"`

	Type string `json:"type,omitempty"`
	Name string `json:"name,omitempty"`

	// ProviderConfigKeys is a list of keys into "provider_configs" (shown above)
	// for the provider configuration that this resource is associated with.
	// This field becomes ProviderConfigKey when marshaling if there is only one item.
	ProviderConfigKeys []string `json:"provider_config_keys,omitempty"`

	// Provisioners is an optional field which describes any provisioners.
	// Connection info will not be included here.
	Provisioners []provisioner `json:"provisioners,omitempty"`

	// Expressions" describes the resource-type-specific  content of the
	// configuration block.
	Expressions map[string]interface{} `json:"expressions,omitempty"`

	// SchemaVersion indicates which version of the resource type schema the
	// "values" property conforms to.
	SchemaVersion uint64 `json:"schema_version"`

	// CountExpression and ForEachExpression describe the expressions given for
	// the corresponding meta-arguments in the resource configuration block.
	// These are omitted if the corresponding argument isn't set.
	CountExpression   *expression `json:"count_expression,omitempty"`
	ForEachExpression *expression `json:"for_each_expression,omitempty"`

	DependsOn []string `json:"depends_on,omitempty"`
}

func (r resource) MarshalJSON() ([]byte, error) {
	type resourceNoMarshal resource

	res := struct {
		resourceNoMarshal
		// ProviderConfigKey stays here for compatibility with previous
		// versions. It is set when there is only one provider used.
		ProviderConfigKey string `json:"provider_config_key,omitempty"`
	}{
		resourceNoMarshal: resourceNoMarshal(r),
	}

	if len(res.ProviderConfigKeys) == 1 {
		res.ProviderConfigKey = res.ProviderConfigKeys[0]
		res.ProviderConfigKeys = nil
	}

	return json.Marshal(res)
}

type output struct {
	Sensitive   bool       `json:"sensitive,omitempty"`
	Expression  expression `json:"expression,omitempty"`
	DependsOn   []string   `json:"depends_on,omitempty"`
	Description string     `json:"description,omitempty"`
}

type provisioner struct {
	Type        string                 `json:"type,omitempty"`
	Expressions map[string]interface{} `json:"expressions,omitempty"`
}

// Marshal returns the json encoding of tofu configuration.
func Marshal(c *configs.Config, schemas *tofu.Schemas) ([]byte, error) {
	var output config

	pcs := make(map[string]providerConfig)
	marshalProviderConfigs(c, schemas, pcs)

	rootModule, err := marshalModule(c, schemas, "")
	if err != nil {
		return nil, err
	}
	output.RootModule = rootModule

	normalizeModuleProviderKeys(&rootModule, pcs)

	for name, pc := range pcs {
		if len(pc.parentKeys) != 0 {
			delete(pcs, name)
		}
	}
	output.ProviderConfigs = pcs

	ret, err := json.Marshal(output)
	return ret, err
}

func marshalProviderConfigs(
	c *configs.Config,
	schemas *tofu.Schemas,
	m map[string]providerConfig,
) {
	if c == nil {
		return
	}

	// We want to determine only the provider requirements from this module,
	// ignoring any descendants.  Disregard any diagnostics when determining
	// requirements because we want this marshalling to succeed even if there
	// are invalid constraints.
	reqs, _ := c.ProviderRequirementsShallow()

	// Add an entry for each provider configuration block in the module.
	for k, pc := range c.Module.ProviderConfigs {
		providerFqn := c.ProviderForConfigAddr(addrs.LocalProviderConfig{LocalName: pc.Name})
		schema := schemas.ProviderConfig(providerFqn)

		p := providerConfig{
			Name:          pc.Name,
			FullName:      providerFqn.String(),
			Alias:         pc.Alias,
			ModuleAddress: c.Path.String(),
			Expressions:   marshalExpressions(pc.Config, schema),
		}

		// Store the fully resolved provider version constraint, rather than
		// using the version argument in the configuration block. This is both
		// future proof (for when we finish the deprecation of the provider config
		// version argument) and more accurate (as it reflects the full set of
		// constraints, in case there are multiple).
		if vc, ok := reqs[providerFqn]; ok {
			p.VersionConstraint = getproviders.VersionConstraintsString(vc)
		}

		key := opaqueProviderKey(k, c.Path.String())

		m[key] = p
	}

	// Ensure that any required providers with no associated configuration
	// block are included in the set.
	for k, pr := range c.Module.ProviderRequirements.RequiredProviders {
		// If a provider has aliases defined, process those first.
		for _, alias := range pr.Aliases {
			// If there exists a value for this provider, we have nothing to add
			// to it, so skip.
			key := opaqueProviderKey(alias.StringCompact(), c.Path.String())
			if _, exists := m[key]; exists {
				continue
			}
			// Given no provider configuration block exists, the only fields we can
			// fill here are the local name, FQN, module address, and version
			// constraints.
			p := providerConfig{
				Name:          pr.Name,
				FullName:      pr.Type.String(),
				ModuleAddress: c.Path.String(),
			}

			if vc, ok := reqs[pr.Type]; ok {
				p.VersionConstraint = getproviders.VersionConstraintsString(vc)
			}

			m[key] = p
		}

		// If there exists a value for this provider, we have nothing to add
		// to it, so skip.
		key := opaqueProviderKey(k, c.Path.String())
		if _, exists := m[key]; exists {
			continue
		}

		// Given no provider configuration block exists, the only fields we can
		// fill here are the local name, module address, and version
		// constraints.
		p := providerConfig{
			Name:          pr.Name,
			FullName:      pr.Type.String(),
			ModuleAddress: c.Path.String(),
		}

		if vc, ok := reqs[pr.Type]; ok {
			p.VersionConstraint = getproviders.VersionConstraintsString(vc)
		}

		if c.Parent != nil {
			parentKey := opaqueProviderKey(pr.Name, c.Parent.Path.String())
			p.parentKeys = findSourceProviderKeys([]string{parentKey}, p.FullName, m)
		}

		m[key] = p
	}

	// Providers could be implicitly created or inherited from the parent module
	// when no requirements and configuration block defined.
	for req := range reqs {
		// Only default providers could implicitly exist,
		// so the provider name must be same as the provider type.
		key := opaqueProviderKey(req.Type, c.Path.String())
		if _, exists := m[key]; exists {
			continue
		}

		p := providerConfig{
			Name:          req.Type,
			FullName:      req.String(),
			ModuleAddress: c.Path.String(),
		}

		// In child modules, providers defined in the parent module can be implicitly used.
		if c.Parent != nil {
			parentKey := opaqueProviderKey(req.Type, c.Parent.Path.String())
			p.parentKeys = findSourceProviderKeys([]string{parentKey}, p.FullName, m)
		}

		m[key] = p
	}

	// Must also visit our child modules, recursively.
	for name, mc := range c.Module.ModuleCalls {
		// Keys in c.Children are guaranteed to match those in c.Module.ModuleCalls
		cc := c.Children[name]

		// Add provider config map entries for passed provider configs,
		// pointing at the passed configuration
		for _, ppc := range mc.Providers {
			// These provider names include aliases, if set
			moduleProviderName := ppc.InChild.String()
			parentKeys := providerAddrsToConfigKeys(cc.Parent.Path.String(), ppc.InParentMapping.AllAddrs())

			// Look up the provider FQN from the module context, using the non-aliased local name
			providerFqn := cc.ProviderForConfigAddr(addrs.LocalProviderConfig{LocalName: ppc.InChild.Name})

			// The presence of passed provider configs means that we cannot have
			// any configuration expressions or version constraints here
			p := providerConfig{
				Name:          moduleProviderName,
				FullName:      providerFqn.String(),
				ModuleAddress: cc.Path.String(),
			}

			p.parentKeys = findSourceProviderKeys(parentKeys, p.FullName, m)

			key := opaqueProviderKey(moduleProviderName, cc.Path.String())
			m[key] = p
		}

		// Finally, marshal any other provider configs within the called module.
		// It is safe to do this last because it is invalid to configure a
		// provider which has passed provider configs in the module call.
		marshalProviderConfigs(cc, schemas, m)
	}
}

func marshalModule(c *configs.Config, schemas *tofu.Schemas, addr string) (module, error) {
	var module module
	var rs []resource

	managedResources, err := marshalResources(c.Module.ManagedResources, schemas, addr)
	if err != nil {
		return module, err
	}
	dataResources, err := marshalResources(c.Module.DataResources, schemas, addr)
	if err != nil {
		return module, err
	}

	rs = append(managedResources, dataResources...)
	module.Resources = rs

	outputs := make(map[string]output)
	for _, v := range c.Module.Outputs {
		o := output{
			Sensitive:  v.Sensitive,
			Expression: marshalExpression(v.Expr),
		}
		if v.Description != "" {
			o.Description = v.Description
		}
		if len(v.DependsOn) > 0 {
			dependencies := make([]string, len(v.DependsOn))
			for i, d := range v.DependsOn {
				ref, diags := addrs.ParseRef(d)
				// we should not get an error here, because `tofu validate`
				// would have complained well before this point, but if we do we'll
				// silently skip it.
				if !diags.HasErrors() {
					dependencies[i] = ref.Subject.String()
				}
			}
			o.DependsOn = dependencies
		}

		outputs[v.Name] = o
	}
	module.Outputs = outputs

	module.ModuleCalls = marshalModuleCalls(c, schemas)

	if len(c.Module.Variables) > 0 {
		vars := make(variables, len(c.Module.Variables))
		for k, v := range c.Module.Variables {
			var defaultValJSON []byte
			if v.Default == cty.NilVal {
				defaultValJSON = nil
			} else {
				defaultValJSON, err = ctyjson.Marshal(v.Default, v.Default.Type())
				if err != nil {
					return module, err
				}
			}
			vars[k] = &variable{
				Default:     defaultValJSON,
				Description: v.Description,
				Sensitive:   v.Sensitive,
			}
		}
		module.Variables = vars
	}

	return module, nil
}

func marshalModuleCalls(c *configs.Config, schemas *tofu.Schemas) map[string]moduleCall {
	ret := make(map[string]moduleCall)

	for name, mc := range c.Module.ModuleCalls {
		mcConfig := c.Children[name]
		ret[name] = marshalModuleCall(mcConfig, mc, schemas)
	}

	return ret
}

func marshalModuleCall(c *configs.Config, mc *configs.ModuleCall, schemas *tofu.Schemas) moduleCall {
	// It is possible to have a module call with a nil config.
	if c == nil {
		return moduleCall{}
	}

	ret := moduleCall{
		// We're intentionally echoing back exactly what the user entered
		// here, rather than the normalized version in SourceAddr, because
		// historically we only _had_ the raw address and thus it would be
		// a (admittedly minor) breaking change to start normalizing them
		// now, in case consumers of this data are expecting a particular
		// non-normalized syntax.
		Source:            mc.SourceAddrRaw,
		VersionConstraint: mc.Version.Required.String(),
	}
	cExp := marshalExpression(mc.Count)
	if !cExp.Empty() {
		ret.CountExpression = &cExp
	} else {
		fExp := marshalExpression(mc.ForEach)
		if !fExp.Empty() {
			ret.ForEachExpression = &fExp
		}
	}

	schema := &configschema.Block{}
	schema.Attributes = make(map[string]*configschema.Attribute)
	for _, variable := range c.Module.Variables {
		schema.Attributes[variable.Name] = &configschema.Attribute{
			Required: variable.Default == cty.NilVal,
		}
	}

	ret.Expressions = marshalExpressions(mc.Config, schema)

	module, _ := marshalModule(c, schemas, c.Path.String())

	ret.Module = module

	if len(mc.DependsOn) > 0 {
		dependencies := make([]string, len(mc.DependsOn))
		for i, d := range mc.DependsOn {
			ref, diags := addrs.ParseRef(d)
			// we should not get an error here, because `tofu validate`
			// would have complained well before this point, but if we do we'll
			// silently skip it.
			if !diags.HasErrors() {
				dependencies[i] = ref.Subject.String()
			}
		}
		ret.DependsOn = dependencies
	}

	return ret
}

func marshalResources(resources map[string]*configs.Resource, schemas *tofu.Schemas, moduleAddr string) ([]resource, error) {
	var rs []resource
	for _, v := range resources {
		r := resource{
			Address:            v.Addr().String(),
			Type:               v.Type,
			Name:               v.Name,
			ProviderConfigKeys: providerAddrsToConfigKeys(moduleAddr, v.AllProviderConfigAddrs()),
		}

		switch v.Mode {
		case addrs.ManagedResourceMode:
			r.Mode = "managed"
		case addrs.DataResourceMode:
			r.Mode = "data"
		default:
			return rs, fmt.Errorf("resource %s has an unsupported mode %s", r.Address, v.Mode.String())
		}

		cExp := marshalExpression(v.Count)
		if !cExp.Empty() {
			r.CountExpression = &cExp
		} else {
			fExp := marshalExpression(v.ForEach)
			if !fExp.Empty() {
				r.ForEachExpression = &fExp
			}
		}

		schema, schemaVer := schemas.ResourceTypeConfig(
			v.Provider,
			v.Mode,
			v.Type,
		)
		if schema == nil {
			return nil, fmt.Errorf("no schema found for %s (in provider %s)", v.Addr().String(), v.Provider)
		}
		r.SchemaVersion = schemaVer

		r.Expressions = marshalExpressions(v.Config, schema)

		// Managed is populated only for Mode = addrs.ManagedResourceMode
		if v.Managed != nil && len(v.Managed.Provisioners) > 0 {
			var provisioners []provisioner
			for _, p := range v.Managed.Provisioners {
				schema := schemas.ProvisionerConfig(p.Type)
				prov := provisioner{
					Type:        p.Type,
					Expressions: marshalExpressions(p.Config, schema),
				}
				provisioners = append(provisioners, prov)
			}
			r.Provisioners = provisioners
		}

		if len(v.DependsOn) > 0 {
			dependencies := make([]string, len(v.DependsOn))
			for i, d := range v.DependsOn {
				ref, diags := addrs.ParseRef(d)
				// we should not get an error here, because `tofu validate`
				// would have complained well before this point, but if we do we'll
				// silently skip it.
				if !diags.HasErrors() {
					dependencies[i] = ref.Subject.String()
				}
			}
			r.DependsOn = dependencies
		}

		rs = append(rs, r)
	}
	sort.Slice(rs, func(i, j int) bool {
		return rs[i].Address < rs[j].Address
	})
	return rs, nil
}

// Flatten all resource provider keys in a module and its descendents, such
// that any resources from providers using a configuration passed through the
// module call have a direct reference to that provider configuration.
func normalizeModuleProviderKeys(m *module, pcs map[string]providerConfig) {
	for i := range m.Resources {
		res := &m.Resources[i]

		if len(res.ProviderConfigKeys) == 0 {
			continue
		}

		uniqueKeys := make(map[string]struct{})

		for _, key := range res.ProviderConfigKeys {
			if pc, exists := pcs[key]; exists {
				if len(pc.parentKeys) == 0 {
					uniqueKeys[key] = struct{}{}
					continue
				}

				for _, parentKey := range pc.parentKeys {
					if _, ok := pcs[parentKey]; ok {
						uniqueKeys[parentKey] = struct{}{}
					}
				}
			}
		}

		res.ProviderConfigKeys = make([]string, 0, len(uniqueKeys))
		for k := range uniqueKeys {
			res.ProviderConfigKeys = append(res.ProviderConfigKeys, k)
		}

		slices.Sort(res.ProviderConfigKeys)
	}

	for _, mc := range m.ModuleCalls {
		normalizeModuleProviderKeys(&mc.Module, pcs)
	}
}

// opaqueProviderKey generates a unique absProviderConfig-like string from the module
// address and provider
func opaqueProviderKey(provider string, addr string) (key string) {
	key = provider
	if addr != "" {
		key = fmt.Sprintf("%s:%s", addr, provider)
	}
	return key
}

// Traverse up the module call tree until we find the provider
// configuration which has no linked parent config. This is then
// the source of the configuration used in this module call, so
// we link to it directly
func findSourceProviderKeys(startKeys []string, fullName string, m map[string]providerConfig) []string {
	var sourceKeys []string

	type keySet struct {
		own     string
		parents []string
	}

	nextKeys := []keySet{{parents: startKeys}}
	for len(nextKeys) != 0 {
		key := nextKeys[0]
		nextKeys = nextKeys[1:]
		hasParents := false

		for _, pk := range key.parents {
			parent, exists := m[pk]
			if !exists || parent.FullName != fullName {
				continue
			}

			hasParents = true
			nextKeys = append(nextKeys, keySet{
				own:     pk,
				parents: parent.parentKeys,
			})
		}

		if !hasParents && key.own != "" {
			sourceKeys = append(sourceKeys, key.own)
		}
	}

	return sourceKeys
}

func providerAddrsToConfigKeys(moduleAddr string, localAddrs []addrs.LocalProviderConfig) []string {
	keys := make([]string, 0, len(localAddrs))

	for _, addr := range localAddrs {
		k := opaqueProviderKey(addr.StringCompact(), moduleAddr)

		keys = append(keys, k)
	}

	return keys
}
