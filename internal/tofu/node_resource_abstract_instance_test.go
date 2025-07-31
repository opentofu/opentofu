// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestNodeAbstractResourceInstanceProvider(t *testing.T) {
	tests := []struct {
		Addr                 addrs.AbsResourceInstance
		Config               *configs.Resource
		StoredProviderConfig addrs.AbsProviderConfig
		Want                 addrs.Provider
	}{
		{
			Addr: addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "null_resource",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "hashicorp",
				Type:      "null",
			},
		},
		{
			Addr: addrs.Resource{
				Mode: addrs.DataResourceMode,
				Type: "terraform_remote_state",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Want: addrs.Provider{
				// As a special case, the type prefix "terraform_" maps to
				// the builtin provider, not the default one.
				Hostname:  addrs.BuiltInProviderHost,
				Namespace: addrs.BuiltInProviderNamespace,
				Type:      "terraform",
			},
		},
		{
			Addr: addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "null_resource",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Config: &configs.Resource{
				// Just enough configs.Resource for the Provider method. Not
				// actually valid for general use.
				Provider: addrs.Provider{
					Hostname:  addrs.DefaultProviderRegistryHost,
					Namespace: "awesomecorp",
					Type:      "happycloud",
				},
			},
			// The config overrides the default behavior.
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "awesomecorp",
				Type:      "happycloud",
			},
		},
		{
			Addr: addrs.Resource{
				Mode: addrs.DataResourceMode,
				Type: "terraform_remote_state",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Config: &configs.Resource{
				// Just enough configs.Resource for the Provider method. Not
				// actually valid for general use.
				Provider: addrs.Provider{
					Hostname:  addrs.DefaultProviderRegistryHost,
					Namespace: "awesomecorp",
					Type:      "happycloud",
				},
			},
			// The config overrides the default behavior.
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "awesomecorp",
				Type:      "happycloud",
			},
		},
		{
			Addr: addrs.Resource{
				Mode: addrs.DataResourceMode,
				Type: "null_resource",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Config: nil,
			StoredProviderConfig: addrs.AbsProviderConfig{
				Module: addrs.RootModule,
				Provider: addrs.Provider{
					Hostname:  addrs.DefaultProviderRegistryHost,
					Namespace: "awesomecorp",
					Type:      "null",
				},
			},
			// The stored provider config overrides the default behavior.
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "awesomecorp",
				Type:      "null",
			},
		},
	}

	for _, test := range tests {
		var name string
		if test.Config != nil {
			name = fmt.Sprintf("%s with configured %s", test.Addr, test.Config.Provider)
		} else {
			name = fmt.Sprintf("%s with no configuration", test.Addr)
		}
		t.Run(name, func(t *testing.T) {
			node := &NodeAbstractResourceInstance{
				// Just enough NodeAbstractResourceInstance for the Provider
				// function. (This would not be valid for some other functions.)
				Addr: test.Addr,
				NodeAbstractResource: NodeAbstractResource{
					Addr:   test.Addr.ConfigResource(),
					Config: test.Config,
					storedProviderConfig: ResolvedProvider{
						ProviderConfig: test.StoredProviderConfig,
					},
				},
			}
			got := node.Provider()
			if got != test.Want {
				t.Errorf("wrong result\naddr:  %s\nconfig: %#v\ngot:   %s\nwant:  %s", test.Addr, test.Config, got, test.Want)
			}
		})
	}
}

func TestNodeAbstractResourceInstance_WriteResourceInstanceState(t *testing.T) {
	state := states.NewState()
	evalCtx := new(MockEvalContext)
	evalCtx.StateState = state.SyncWrapper()
	evalCtx.PathPath = addrs.RootModuleInstance

	mockProvider := mockProviderWithResourceTypeSchema("aws_instance", &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"id": {
				Type:     cty.String,
				Optional: true,
			},
		},
	})

	obj := &states.ResourceInstanceObject{
		Value: cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("i-abc123"),
		}),
		Status: states.ObjectReady,
	}

	node := &NodeAbstractResourceInstance{
		Addr: mustResourceInstanceAddr("aws_instance.foo"),
		// instanceState:        obj,
		NodeAbstractResource: NodeAbstractResource{
			ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
		},
	}
	evalCtx.ProviderProvider = mockProvider
	evalCtx.ProviderSchemaSchema = mockProvider.GetProviderSchema(t.Context())

	err := node.writeResourceInstanceState(t.Context(), evalCtx, obj, workingState)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	checkStateString(t, state, `
aws_instance.foo:
  ID = i-abc123
  provider = provider["registry.opentofu.org/hashicorp/aws"]
	`)
}

func TestFilterResourceProvisioners(t *testing.T) {
	tests := map[string]struct {
		when               configs.ProvisionerWhen
		cfg                *configs.Resource
		removedBlocksProvs []*configs.Provisioner
		wantProvs          []*configs.Provisioner
	}{
		"config and provisioners nil": {
			when:               configs.ProvisionerWhenDestroy,
			cfg:                nil,
			removedBlocksProvs: nil,
			wantProvs:          []*configs.Provisioner{},
		},
		"config nil and provisioners contains targeted provisioners": {
			when: configs.ProvisionerWhenDestroy,
			cfg:  nil,
			removedBlocksProvs: []*configs.Provisioner{
				{Type: "local-exec", When: configs.ProvisionerWhenDestroy},
				{Type: "local-exec2", When: configs.ProvisionerWhenCreate},
			},
			wantProvs: []*configs.Provisioner{
				{Type: "local-exec", When: configs.ProvisionerWhenDestroy},
			},
		},
		"config.managed nil and provisioners contains no targeted provisioners": {
			when: configs.ProvisionerWhenDestroy,
			cfg:  &configs.Resource{},
			removedBlocksProvs: []*configs.Provisioner{
				{Type: "local-exec", When: configs.ProvisionerWhenCreate},
				{Type: "local-exec2", When: configs.ProvisionerWhenCreate},
			},
			wantProvs: []*configs.Provisioner{},
		},
		// This is expecting an empty result because we want to use the resource defined provisioners when
		// config.managed exists
		"config.managed not nil and provisioners contains targeted provisioners": {
			when: configs.ProvisionerWhenCreate,
			cfg: &configs.Resource{
				Managed: &configs.ManagedResource{},
			},
			removedBlocksProvs: []*configs.Provisioner{
				{Type: "local-exec", When: configs.ProvisionerWhenCreate},
				{Type: "local-exec2", When: configs.ProvisionerWhenCreate},
			},
			wantProvs: nil,
		},
		"config.managed is having provisioners therefore removed blocks provisioners are ignored": {
			when: configs.ProvisionerWhenCreate,
			cfg: &configs.Resource{
				Managed: &configs.ManagedResource{
					Provisioners: []*configs.Provisioner{
						{Type: "local-exec3", When: configs.ProvisionerWhenCreate},
						{Type: "local-exec4", When: configs.ProvisionerWhenCreate},
					},
				},
			},
			removedBlocksProvs: []*configs.Provisioner{
				{Type: "local-exec", When: configs.ProvisionerWhenCreate},
				{Type: "local-exec2", When: configs.ProvisionerWhenCreate},
			},
			wantProvs: []*configs.Provisioner{
				{Type: "local-exec3", When: configs.ProvisionerWhenCreate},
				{Type: "local-exec4", When: configs.ProvisionerWhenCreate},
			},
		},
	}
	for name, test := range tests {
		t.Run(fmt.Sprintf("%s-%s", test.when, name), func(t *testing.T) {
			res := filterResourceProvisioners(test.cfg, test.removedBlocksProvs, test.when)
			if diff := cmp.Diff(test.wantProvs, res); diff != "" {
				t.Errorf("expected provisioners different than what we got:\n%s", diff)
			}
		})
	}
}

func TestMaybeImproveResourceInstanceDiagnostics(t *testing.T) {
	// This test is focused mainly on whether
	// maybeImproveResourceInstanceDiagnostics is able to correctly identify
	// deferral-related diagnostics and transform them, while keeping
	// other unrelated diagnostics intact and unmodified.
	// TestContext2Plan_providerDefersPlanning tests that the effect of
	// this function is exposed externally when a provider's PlanResourceChange
	// method returns a suitable diagnostic.

	var input tfdiags.Diagnostics
	input = input.Append(tfdiags.Sourceless(
		tfdiags.Warning,
		"This is not a deferral-related diagnostic",
		"This one should not be modified at all.",
	))
	input = input.Append(providers.NewDeferralDiagnostic(providers.DeferredBecauseProviderConfigUnknown))
	input = input.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"This is not a deferral-related diagnostic",
		"This one should not be modified at all.",
	))
	input = input.Append(providers.NewDeferralDiagnostic(providers.DeferredBecauseResourceConfigUnknown))
	input = input.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"This is not a deferral-related diagnostic either",
		"Leave this one alone too.",
	))

	// We'll use ForRPC here just to make the diagnostics easier to compare,
	// since we care primarily about their description test here.
	got := maybeImproveResourceInstanceDiagnostics(input, mustAbsResourceAddr("foo.bar").Instance(addrs.IntKey(1))).ForRPC()
	var want tfdiags.Diagnostics
	want = want.Append(tfdiags.Sourceless(
		tfdiags.Warning,
		"This is not a deferral-related diagnostic",
		"This one should not be modified at all.",
	))
	want = want.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"Provider configuration is incomplete",
		`The provider was unable to work with this resource because the associated provider configuration makes use of values from other resources that will not be known until after apply.

To work around this, use the planning option -exclude="foo.bar[1]" to first apply without this object, and then apply normally to converge.`,
	))
	want = want.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"This is not a deferral-related diagnostic",
		"This one should not be modified at all.",
	))
	want = want.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"Resource configuration is incomplete",
		`The provider was unable to act on this resource configuration because it makes use of values from other resources that will not be known until after apply.

To work around this, use the planning option -exclude="foo.bar[1]" to first apply without this object, and then apply normally to converge.`,
	))
	want = want.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"This is not a deferral-related diagnostic either",
		"Leave this one alone too.",
	))
	want = want.ForRPC()

	if diff := cmp.Diff(want, got); diff != "" {
		t.Error("wrong result\n" + diff)
	}

}
