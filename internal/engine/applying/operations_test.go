// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// operationsForTesting is a test helper for constructing an [execOperations]
// directly from its component parts, for use in unit tests of individual
// operations where we want to just directly describe a fake scenario for
// running the operation under test.
//
// If priorState is nil, this function will use a newly-constructed empty
// state. If workingState is nil, it begins as a deep copy of the prior state.
// If the given workingState is not nil then it will be modified in-place by
// any operations that perform state updates.
//
// If plugins is nil, any request for a plugin instance will fail with a
// generic error. If plugins is non-nil then configOracle should use the same
// plugins collection for its own provider-related functionlity.
//
// If configOracle is nil, this function will use a fake oracle that produces
// negative or empty responses to all questions, except that if asked for
// a provider instance it will attempt to instantiate the requested provider
// from "plugins" and configure it with an empty configuration. Whether that
// works or not depends on the provider's schema and implementation.
func operationsForTesting(
	priorState *states.State,
	configOracle configOracle,
	workingState *states.State,
	plugins plugins.Plugins,
) *execOperations {
	// Right now this is pretty trivial but it's factored out like this because
	// the fields of execOperations already have several TODO comments
	// suggesting that we'll tweak the unexported fields in future, at which
	// point this helper will hopefully minimize the cross-cutting updates we
	// need to make to tests to suit the new representations.

	if priorState == nil {
		priorState = states.NewState()
	}
	if workingState == nil {
		workingState = priorState.DeepCopy()
	}
	if plugins == nil {
		plugins = emptyPlugins{}
	}
	if configOracle == nil {
		configOracle = emptyConfigOracle{
			plugins: plugins,
		}
	}
	return &execOperations{
		priorState:   priorState.SyncWrapper(),
		configOracle: configOracle,
		workingState: workingState.SyncWrapper(),
		plugins:      plugins,
	}
}

type emptyConfigOracle struct {
	plugins plugins.Plugins
}

// ResourceInstanceObjectMeta implements [configOracle].
func (e emptyConfigOracle) ResourceInstanceObjectMeta(ctx context.Context, addr addrs.AbsResourceInstanceObject) []eval.ModuleConfiguredResourceInstanceObjectMeta {
	return nil
}

// DesiredResourceInstance implements [configOracle].
func (e emptyConfigOracle) DesiredResourceInstance(ctx context.Context, addr addrs.AbsResourceInstance) (*eval.DesiredResourceInstance, tfdiags.Diagnostics) {
	return nil, nil
}

// ProviderInstance implements [configOracle].
func (e emptyConfigOracle) ProviderInstance(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) (providers.Interface, tfdiags.Diagnostics) {
	// This is a "best effort" implementation which is designed only for
	// simple stub plugins which don't require any configuration. Tests which
	// involve more complicated plugins would need to provide a different
	// implementation of [configOracle].
	return e.plugins.NewConfiguredProvider(ctx, addr.Config.Config.Provider, cty.EmptyObjectVal)
}

// RootOutputs implements [configOracle].
func (e emptyConfigOracle) RootOutputs(ctx context.Context) eval.RootModuleOutputs {
	return nil
}

// Close implements [configOracle].
func (e emptyConfigOracle) Close(ctx context.Context) tfdiags.Diagnostics {
	return nil
}

type testResourceConfig struct {
	Addr    addrs.AbsResourceInstanceObject
	Meta    []eval.ModuleConfiguredResourceInstanceObjectMeta
	Desired *eval.DesiredResourceInstance
}

type resourceConfigOracle struct {
	fakeConfigs addrs.Map[addrs.AbsResourceInstanceObject, *testResourceConfig]
	plugins     plugins.Plugins
}

// resourceConfigsForTesting returns a fake [configOracle] that answers
// questions about resource instance objects using the given items directly,
// and which attempts to instantiate providers using the given [plugins.Plugins]
// assuming that no configuration arguments are required.
//
// if "plugins" is nil then requests for provider instances will fail with
// an error.
func resourceConfigsForTesting(t *testing.T, configs []*testResourceConfig, plugins plugins.Plugins) configOracle {
	fakeConfigs := addrs.MakeMap[addrs.AbsResourceInstanceObject, *testResourceConfig]()
	for _, fakeConfig := range configs {
		fakeConfigs.Put(fakeConfig.Addr, fakeConfig)
	}
	return &resourceConfigOracle{
		fakeConfigs: fakeConfigs,
		plugins:     plugins,
	}
}

// DesiredResourceInstance implements [configOracle].
func (r *resourceConfigOracle) DesiredResourceInstance(ctx context.Context, addr addrs.AbsResourceInstance) (*eval.DesiredResourceInstance, tfdiags.Diagnostics) {
	fake, ok := r.fakeConfigs.GetOk(addr.CurrentObject())
	if !ok {
		return nil, nil
	}
	return fake.Desired, nil
}

// ResourceInstanceObjectMeta implements [configOracle].
func (r *resourceConfigOracle) ResourceInstanceObjectMeta(ctx context.Context, addr addrs.AbsResourceInstanceObject) []eval.ModuleConfiguredResourceInstanceObjectMeta {
	fake, ok := r.fakeConfigs.GetOk(addr)
	if !ok {
		return nil
	}
	return fake.Meta

}

// ProviderInstance implements [configOracle].
func (r *resourceConfigOracle) ProviderInstance(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) (providers.Interface, tfdiags.Diagnostics) {
	// This is a "best effort" implementation which is designed only for
	// simple stub plugins which don't require any configuration. Tests which
	// involve more complicated plugins would need to provide a different
	// implementation of [configOracle].
	return r.plugins.NewConfiguredProvider(ctx, addr.Config.Config.Provider, cty.EmptyObjectVal)
}

// RootOutputs implements [configOracle].
func (r *resourceConfigOracle) RootOutputs(ctx context.Context) eval.RootModuleOutputs {
	return nil
}

// Close implements [configOracle].
func (r *resourceConfigOracle) Close(ctx context.Context) tfdiags.Diagnostics {
	return nil
}

type emptyPlugins struct{}

// BuildFunction implements [plugins.Plugins].
func (e emptyPlugins) BuildFunction(ctx context.Context, provider addrs.Provider, pf addrs.ProviderFunction, stubMissing bool, rng hcl.Range) (function.Function, tfdiags.Diagnostics) {
	return function.Function{}, tfdiags.New(fmt.Errorf("no providers are available in this test"))
}

// Close implements [plugins.Plugins].
func (e emptyPlugins) Close(ctx context.Context) error {
	return nil
}

// NewConfiguredProvider implements [plugins.Plugins].
func (e emptyPlugins) NewConfiguredProvider(ctx context.Context, provider addrs.Provider, configVal cty.Value) (providers.Configured, tfdiags.Diagnostics) {
	return nil, tfdiags.New(fmt.Errorf("no providers are available in this test"))
}

// ProviderConfigSchema implements [plugins.Plugins].
func (e emptyPlugins) ProviderConfigSchema(ctx context.Context, provider addrs.Provider) (*providers.Schema, tfdiags.Diagnostics) {
	return nil, tfdiags.New(fmt.Errorf("no providers are available in this test"))
}

// ResourceTypeSchema implements [plugins.Plugins].
func (e emptyPlugins) ResourceTypeSchema(ctx context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string) (*providers.Schema, tfdiags.Diagnostics) {
	return nil, tfdiags.New(fmt.Errorf("no providers are available in this test"))
}

// ValidateProviderConfig implements [plugins.Plugins].
func (e emptyPlugins) ValidateProviderConfig(ctx context.Context, provider addrs.Provider, configVal cty.Value) tfdiags.Diagnostics {
	return tfdiags.New(fmt.Errorf("no providers are available in this test"))
}

// ValidateResourceConfig implements [plugins.Plugins].
func (e emptyPlugins) ValidateResourceConfig(ctx context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string, configVal cty.Value) tfdiags.Diagnostics {
	return tfdiags.New(fmt.Errorf("no providers are available in this test"))
}

// ProvisionResource implements [plugins.Plugins].
func (e emptyPlugins) ProvisionResource(ctx context.Context, typ string, config cty.Value, connection cty.Value, output plugins.ProvisionerOutput) tfdiags.Diagnostics {
	return tfdiags.New(fmt.Errorf("no provisioners are available in this test"))
}

// ProvisionerConfigSchema implements [plugins.Plugins].
func (e emptyPlugins) ProvisionerConfigSchema(ctx context.Context, typeName string) (*configschema.Block, tfdiags.Diagnostics) {
	return nil, tfdiags.New(fmt.Errorf("no provisioners are available in this test"))
}

// ValidateProvisionerConfig implements [plugins.Plugins].
func (e emptyPlugins) ValidateProvisionerConfig(ctx context.Context, typ string, config cty.Value) tfdiags.Diagnostics {
	return tfdiags.New(fmt.Errorf("no provisioners are available in this test"))
}
