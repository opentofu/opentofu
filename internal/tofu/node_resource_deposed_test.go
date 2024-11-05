// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/zclconf/go-cty/cty"
)

func TestNodePlanDeposedResourceInstanceObject_Execute(t *testing.T) {
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
		deposedKey := states.NewDeposedKey()
		absResource := mustResourceInstanceAddr(test.nodeAddress)

		ctx, p := initMockEvalContext(test.nodeAddress, deposedKey)

		node := NodePlanDeposedResourceInstanceObject{
			NodeAbstractResourceInstance: &NodeAbstractResourceInstance{
				Addr: absResource,
				NodeAbstractResource: NodeAbstractResource{
					ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`)},
				},
			},
			DeposedKey:        deposedKey,
			EndpointsToRemove: test.nodeEndpointsToRemove,
		}

		err := node.Execute(ctx, walkPlan)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if !p.UpgradeResourceStateCalled {
			t.Errorf("UpgradeResourceState wasn't called; should've been called to upgrade the previous run's object")
		}
		if !p.ReadResourceCalled {
			t.Errorf("ReadResource wasn't called; should've been called to refresh the deposed object")
		}

		change := ctx.Changes().GetResourceInstanceChange(absResource, deposedKey)
		if got, want := change.ChangeSrc.Action, test.wantAction; got != want {
			t.Fatalf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
	}
}

func TestNodeDestroyDeposedResourceInstanceObject_Execute(t *testing.T) {
	deposedKey := states.NewDeposedKey()
	state := states.NewState()
	absResourceAddr := "test_instance.foo"
	ctx, _ := initMockEvalContext(absResourceAddr, deposedKey)

	absResource := mustResourceInstanceAddr(absResourceAddr)
	node := NodeDestroyDeposedResourceInstanceObject{
		NodeAbstractResourceInstance: &NodeAbstractResourceInstance{
			Addr: absResource,
			NodeAbstractResource: NodeAbstractResource{
				ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`)},
			},
		},
		DeposedKey: deposedKey,
	}
	err := node.Execute(ctx, walkApply)

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if !state.Empty() {
		t.Fatalf("resources left in state after destroy")
	}
}

func TestNodeDestroyDeposedResourceInstanceObject_WriteResourceInstanceState(t *testing.T) {
	state := states.NewState()
	ctx := new(MockEvalContext)
	ctx.StateState = state.SyncWrapper()
	ctx.PathPath = addrs.RootModuleInstance
	mockProvider := mockProviderWithResourceTypeSchema("aws_instance", &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"id": {
				Type:     cty.String,
				Optional: true,
			},
		},
	})
	ctx.ProviderProvider = mockProvider
	ctx.ProviderSchemaSchema = mockProvider.GetProviderSchema()

	obj := &states.ResourceInstanceObject{
		Value: cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("i-abc123"),
		}),
		Status: states.ObjectReady,
	}
	node := &NodeDestroyDeposedResourceInstanceObject{
		NodeAbstractResourceInstance: &NodeAbstractResourceInstance{
			NodeAbstractResource: NodeAbstractResource{
				ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
			},
			Addr: mustResourceInstanceAddr("aws_instance.foo"),
		},
		DeposedKey: states.NewDeposedKey(),
	}
	err := node.writeResourceInstanceState(ctx, obj)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	checkStateString(t, state, `
aws_instance.foo: (1 deposed)
  ID = <not created>
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  Deposed ID 1 = i-abc123
	`)
}

func TestNodeDestroyDeposedResourceInstanceObject_ExecuteMissingState(t *testing.T) {
	p := simpleMockProvider()
	ctx := &MockEvalContext{
		StateState:           states.NewState().SyncWrapper(),
		ProviderProvider:     simpleMockProvider(),
		ProviderSchemaSchema: p.GetProviderSchema(),
		ChangesChanges:       plans.NewChanges().SyncWrapper(),
	}

	node := NodeDestroyDeposedResourceInstanceObject{
		NodeAbstractResourceInstance: &NodeAbstractResourceInstance{
			Addr: mustResourceInstanceAddr("test_object.foo"),
			NodeAbstractResource: NodeAbstractResource{
				ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`)},
			},
		},
		DeposedKey: states.NewDeposedKey(),
	}
	err := node.Execute(ctx, walkApply)

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNodeForgetDeposedResourceInstanceObject_Execute(t *testing.T) {
	deposedKey := states.NewDeposedKey()
	state := states.NewState()
	absResourceAddr := "test_instance.foo"
	ctx, _ := initMockEvalContext(absResourceAddr, deposedKey)

	absResource := mustResourceInstanceAddr(absResourceAddr)
	node := NodeForgetDeposedResourceInstanceObject{
		NodeAbstractResourceInstance: &NodeAbstractResourceInstance{
			Addr: absResource,
			NodeAbstractResource: NodeAbstractResource{
				ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`)},
			},
		},
		DeposedKey: deposedKey,
	}
	err := node.Execute(ctx, walkApply)

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if !state.Empty() {
		t.Fatalf("resources left in state after forget")
	}
}

func initMockEvalContext(resourceAddrs string, deposedKey states.DeposedKey) (*MockEvalContext, *MockProvider) {
	state := states.NewState()
	absResource := mustResourceInstanceAddr(resourceAddrs)

	if !absResource.Module.Module().Equal(addrs.RootModule) {
		state.EnsureModule(addrs.RootModuleInstance.Child(absResource.Module[0].Name, absResource.Module[0].InstanceKey))
	}

	state.Module(absResource.Module).SetResourceInstanceDeposed(
		absResource.Resource,
		deposedKey,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectTainted,
			AttrsJSON: []byte(`{"id":"bar"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
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

	p := testProvider("test")
	p.ConfigureProvider(providers.ConfigureProviderRequest{})
	p.GetProviderSchemaResponse = &schema

	p.UpgradeResourceStateResponse = &providers.UpgradeResourceStateResponse{
		UpgradedState: cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("bar"),
		}),
	}
	return &MockEvalContext{
		PrevRunStateState:    state.DeepCopy().SyncWrapper(),
		RefreshStateState:    state.DeepCopy().SyncWrapper(),
		StateState:           state.SyncWrapper(),
		ProviderProvider:     p,
		ProviderSchemaSchema: schema,
		ChangesChanges:       plans.NewChanges().SyncWrapper(),
	}, p
}
