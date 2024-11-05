// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/opentofu/opentofu/internal/configs/configschema"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/zclconf/go-cty/cty"
)

func TestNodeResourcePlanOrphan_Execute(t *testing.T) {
	tests := []struct {
		description           string
		nodeAddress           string
		nodeEndpointsToRemove []addrs.ConfigRemovable
		wantAction            plans.Action
	}{
		{
			nodeAddress:           "test_instance.foo",
			nodeEndpointsToRemove: make([]addrs.ConfigRemovable, 0),
			wantAction:            plans.Delete,
		},
		{
			nodeAddress: "test_instance.foo",
			nodeEndpointsToRemove: []addrs.ConfigRemovable{
				interface{}(mustConfigResourceAddr("test_instance.bar")).(addrs.ConfigRemovable),
			},
			wantAction: plans.Delete,
		},
		{
			nodeAddress: "test_instance.foo",
			nodeEndpointsToRemove: []addrs.ConfigRemovable{
				interface{}(addrs.Module{"boop"}).(addrs.ConfigRemovable),
			},
			wantAction: plans.Delete,
		},
		{
			nodeAddress: "test_instance.foo",
			nodeEndpointsToRemove: []addrs.ConfigRemovable{
				interface{}(mustConfigResourceAddr("test_instance.foo")).(addrs.ConfigRemovable),
			},
			wantAction: plans.Forget,
		},
		{
			nodeAddress: "test_instance.foo[1]",
			nodeEndpointsToRemove: []addrs.ConfigRemovable{
				interface{}(mustConfigResourceAddr("test_instance.foo")).(addrs.ConfigRemovable),
			},
			wantAction: plans.Forget,
		},
		{
			nodeAddress: "module.boop.test_instance.foo",
			nodeEndpointsToRemove: []addrs.ConfigRemovable{
				interface{}(mustConfigResourceAddr("module.boop.test_instance.foo")).(addrs.ConfigRemovable),
			},
			wantAction: plans.Forget,
		},
		{
			nodeAddress: "module.boop[1].test_instance.foo[1]",
			nodeEndpointsToRemove: []addrs.ConfigRemovable{
				interface{}(mustConfigResourceAddr("module.boop.test_instance.foo")).(addrs.ConfigRemovable),
			},
			wantAction: plans.Forget,
		},
		{
			nodeAddress: "module.boop.test_instance.foo",
			nodeEndpointsToRemove: []addrs.ConfigRemovable{
				interface{}(addrs.Module{"boop"}).(addrs.ConfigRemovable),
			},
			wantAction: plans.Forget,
		},
		{
			nodeAddress: "module.boop[1].test_instance.foo",
			nodeEndpointsToRemove: []addrs.ConfigRemovable{
				interface{}(addrs.Module{"boop"}).(addrs.ConfigRemovable),
			},
			wantAction: plans.Forget,
		},
	}

	for _, test := range tests {
		state := states.NewState()
		absResource := mustResourceInstanceAddr(test.nodeAddress)

		if !absResource.Module.Module().Equal(addrs.RootModule) {
			state.EnsureModule(addrs.RootModuleInstance.Child(absResource.Module[0].Name, absResource.Module[0].InstanceKey))
		}

		state.Module(absResource.Module).SetResourceInstanceCurrent(
			absResource.Resource,
			&states.ResourceInstanceObjectSrc{
				AttrsFlat: map[string]string{
					"test_string": "foo",
				},
				Status: states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)

		schema := providers.ProviderSchema{
			ResourceTypes: map[string]providers.Schema{
				"test_instance": {
					Block: &configschema.Block{
						Attributes: map[string]*configschema.Attribute{
							"id": {
								Type:     cty.String,
								Computed: true,
							},
						},
					},
				},
			},
		}

		p := simpleMockProvider()
		p.ConfigureProvider(providers.ConfigureProviderRequest{})
		p.GetProviderSchemaResponse = &schema

		ctx := &MockEvalContext{
			StateState:               state.SyncWrapper(),
			RefreshStateState:        state.DeepCopy().SyncWrapper(),
			PrevRunStateState:        state.DeepCopy().SyncWrapper(),
			InstanceExpanderExpander: instances.NewExpander(),
			ProviderProvider:         p,
			ProviderSchemaSchema:     schema,
			ChangesChanges:           plans.NewChanges().SyncWrapper(),
		}

		node := NodePlannableResourceInstanceOrphan{
			NodeAbstractResourceInstance: &NodeAbstractResourceInstance{
				NodeAbstractResource: NodeAbstractResource{
					ResolvedProvider: ResolvedProvider{ProviderConfig: addrs.AbsProviderConfig{
						Provider: addrs.NewDefaultProvider("test"),
						Module:   addrs.RootModule,
					}},
				},
				Addr: absResource,
			},
			EndpointsToRemove: test.nodeEndpointsToRemove,
		}

		err := node.Execute(ctx, walkPlan)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		change := ctx.Changes().GetResourceInstanceChange(absResource, states.NotDeposed)
		if got, want := change.ChangeSrc.Action, test.wantAction; got != want {
			t.Fatalf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}

		if !state.Empty() {
			t.Fatalf("expected empty state, got %s", state.String())
		}
	}
}

func TestNodeResourcePlanOrphanExecute_alreadyDeleted(t *testing.T) {
	addr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test_object",
		Name: "foo",
	}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)

	state := states.NewState()
	state.Module(addrs.RootModuleInstance).SetResourceInstanceCurrent(
		addr.Resource,
		&states.ResourceInstanceObjectSrc{
			AttrsFlat: map[string]string{
				"test_string": "foo",
			},
			Status: states.ObjectReady,
		},
		addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("test"),
			Module:   addrs.RootModule,
		},
		addrs.NoKey,
	)
	refreshState := state.DeepCopy()
	prevRunState := state.DeepCopy()
	changes := plans.NewChanges()

	p := simpleMockProvider()
	p.ConfigureProvider(providers.ConfigureProviderRequest{})
	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.NullVal(p.GetProviderSchemaResponse.ResourceTypes["test_string"].Block.ImpliedType()),
	}
	ctx := &MockEvalContext{
		StateState:               state.SyncWrapper(),
		RefreshStateState:        refreshState.SyncWrapper(),
		PrevRunStateState:        prevRunState.SyncWrapper(),
		InstanceExpanderExpander: instances.NewExpander(),
		ProviderProvider:         p,
		ProviderSchemaSchema: providers.ProviderSchema{
			ResourceTypes: map[string]providers.Schema{
				"test_object": {
					Block: simpleTestSchema(),
				},
			},
		},
		ChangesChanges: changes.SyncWrapper(),
	}

	node := NodePlannableResourceInstanceOrphan{
		NodeAbstractResourceInstance: &NodeAbstractResourceInstance{
			NodeAbstractResource: NodeAbstractResource{
				ResolvedProvider: ResolvedProvider{ProviderConfig: addrs.AbsProviderConfig{
					Provider: addrs.NewDefaultProvider("test"),
					Module:   addrs.RootModule,
				}},
			},
			Addr: mustResourceInstanceAddr("test_object.foo"),
		},
	}
	diags := node.Execute(ctx, walkPlan)
	if diags.HasErrors() {
		t.Fatalf("unexpected error: %s", diags.Err())
	}
	if !state.Empty() {
		t.Fatalf("expected empty state, got %s", state.String())
	}

	if got := prevRunState.ResourceInstance(addr); got == nil {
		t.Errorf("no entry for %s in the prev run state; should still be present", addr)
	}
	if got := refreshState.ResourceInstance(addr); got != nil {
		t.Errorf("refresh state has entry for %s; should've been removed", addr)
	}
	if got := changes.ResourceInstance(addr); got != nil {
		t.Errorf("there should be no change for the %s instance, got %s", addr, got.Action)
	}
}

// This test describes a situation which should not be possible, as this node
// should never work on deposed instances. However, a bug elsewhere resulted in
// this code path being exercised and triggered a panic. As a result, the
// assertions at the end of the test are minimal, as the behaviour (aside from
// not panicking) is unspecified.
func TestNodeResourcePlanOrphanExecute_deposed(t *testing.T) {
	addr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test_object",
		Name: "foo",
	}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)

	state := states.NewState()
	state.Module(addrs.RootModuleInstance).SetResourceInstanceDeposed(
		addr.Resource,
		states.NewDeposedKey(),
		&states.ResourceInstanceObjectSrc{
			AttrsFlat: map[string]string{
				"test_string": "foo",
			},
			Status: states.ObjectReady,
		},
		addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("test"),
			Module:   addrs.RootModule,
		},
		addrs.NoKey,
	)
	refreshState := state.DeepCopy()
	prevRunState := state.DeepCopy()
	changes := plans.NewChanges()

	p := simpleMockProvider()
	p.ConfigureProvider(providers.ConfigureProviderRequest{})
	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.NullVal(p.GetProviderSchemaResponse.ResourceTypes["test_string"].Block.ImpliedType()),
	}
	ctx := &MockEvalContext{
		StateState:               state.SyncWrapper(),
		RefreshStateState:        refreshState.SyncWrapper(),
		PrevRunStateState:        prevRunState.SyncWrapper(),
		InstanceExpanderExpander: instances.NewExpander(),
		ProviderProvider:         p,
		ProviderSchemaSchema: providers.ProviderSchema{
			ResourceTypes: map[string]providers.Schema{
				"test_object": {
					Block: simpleTestSchema(),
				},
			},
		},
		ChangesChanges: changes.SyncWrapper(),
	}

	node := NodePlannableResourceInstanceOrphan{
		NodeAbstractResourceInstance: &NodeAbstractResourceInstance{
			NodeAbstractResource: NodeAbstractResource{
				ResolvedProvider: ResolvedProvider{ProviderConfig: addrs.AbsProviderConfig{
					Provider: addrs.NewDefaultProvider("test"),
					Module:   addrs.RootModule,
				}},
			},
			Addr: mustResourceInstanceAddr("test_object.foo"),
		},
	}
	diags := node.Execute(ctx, walkPlan)
	if diags.HasErrors() {
		t.Fatalf("unexpected error: %s", diags.Err())
	}
}
