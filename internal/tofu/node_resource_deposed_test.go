// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/refactoring"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func TestNodePlanDeposedResourceInstanceObject_Execute(t *testing.T) {
	tests := []struct {
		description           string
		nodeAddress           string
		nodeEndpointsToRemove []*refactoring.RemoveStatement
		wantAction            plans.Action
		wantDiags             tfdiags.Diagnostics
	}{
		{
			description:           "no remove blocks",
			nodeAddress:           "test_instance.foo",
			nodeEndpointsToRemove: make([]*refactoring.RemoveStatement, 0),
			wantAction:            plans.Delete,
		},
		{
			description: "remove block is targeting another resource name of same type",
			nodeAddress: "test_instance.foo",
			nodeEndpointsToRemove: []*refactoring.RemoveStatement{
				{
					From: mustConfigResourceAddr("test_instance.bar"),
				},
			},
			wantAction: plans.Delete,
		},
		{
			description: "remove block is targeting a module but current node is from root module",
			nodeAddress: "test_instance.foo",
			nodeEndpointsToRemove: []*refactoring.RemoveStatement{
				{
					From: addrs.Module{"boop"},
				},
			},
			wantAction: plans.Delete,
		},
		{
			description: "remove block is targeting current node",
			nodeAddress: "test_instance.foo",
			nodeEndpointsToRemove: []*refactoring.RemoveStatement{
				{
					From: mustConfigResourceAddr("test_instance.foo"),
				},
			},
			wantAction: plans.Forget,
			wantDiags: tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  "Resource going to be removed from the state",
				Detail:   fmt.Sprintf("After this plan gets applied, the resource %s will not be managed anymore by OpenTofu.\n\nIn case you want to manage the resource again, you will have to import it.", "test_instance.foo"),
			}),
		},
		{
			description: "remove block is targeting current node and required to get it destroyed",
			nodeAddress: "test_instance.foo",
			nodeEndpointsToRemove: []*refactoring.RemoveStatement{
				{
					From:    mustConfigResourceAddr("test_instance.foo"),
					Destroy: true,
				},
			},
			wantAction: plans.Delete,
		},
		{
			description: "remove block is targeting a resource and the current node is an instance of that",
			nodeAddress: "test_instance.foo[1]",
			nodeEndpointsToRemove: []*refactoring.RemoveStatement{
				{
					From: mustConfigResourceAddr("test_instance.foo"),
				},
			},
			wantAction: plans.Forget,
			wantDiags: tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  "Resource going to be removed from the state",
				Detail:   fmt.Sprintf("After this plan gets applied, the resource %s will not be managed anymore by OpenTofu.\n\nIn case you want to manage the resource again, you will have to import it.", "test_instance.foo[1]"),
			}),
		},
		{
			description: "remove block is targeting a resource to be destroyed and the current node is an instance of that",
			nodeAddress: "test_instance.foo[1]",
			nodeEndpointsToRemove: []*refactoring.RemoveStatement{
				{
					From:    mustConfigResourceAddr("test_instance.foo"),
					Destroy: true,
				},
			},
			wantAction: plans.Delete,
		},
		{
			description: "remove block is targeting a resource from a module which is the current node",
			nodeAddress: "module.boop.test_instance.foo",
			nodeEndpointsToRemove: []*refactoring.RemoveStatement{
				{
					From: mustConfigResourceAddr("module.boop.test_instance.foo"),
				},
			},
			wantAction: plans.Forget,
			wantDiags: tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  "Resource going to be removed from the state",
				Detail:   fmt.Sprintf("After this plan gets applied, the resource %s will not be managed anymore by OpenTofu.\n\nIn case you want to manage the resource again, you will have to import it.", "module.boop.test_instance.foo"),
			}),
		},
		{
			description: "remove block is targeting a resource from a module to be destroyed which is the current node",
			nodeAddress: "module.boop.test_instance.foo",
			nodeEndpointsToRemove: []*refactoring.RemoveStatement{
				{
					From:    mustConfigResourceAddr("module.boop.test_instance.foo"),
					Destroy: true,
				},
			},
			wantAction: plans.Delete,
		},
		{
			description: "remove block is targeting a resource from a module to be destroyed which is the current node",
			nodeAddress: "module.boop.test_instance.foo",
			nodeEndpointsToRemove: []*refactoring.RemoveStatement{
				{
					From:    mustConfigResourceAddr("module.boop.test_instance.foo"),
					Destroy: true,
				},
			},
			wantAction: plans.Delete,
		},
		{
			description: "remove block is targeting a resource from a module of which the current node is an instance of",
			nodeAddress: "module.boop[1].test_instance.foo[1]",
			nodeEndpointsToRemove: []*refactoring.RemoveStatement{
				{
					From: mustConfigResourceAddr("module.boop.test_instance.foo"),
				},
			},
			wantAction: plans.Forget,
			wantDiags: tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  "Resource going to be removed from the state",
				Detail:   fmt.Sprintf("After this plan gets applied, the resource %s will not be managed anymore by OpenTofu.\n\nIn case you want to manage the resource again, you will have to import it.", "module.boop[1].test_instance.foo[1]"),
			}),
		},
		{
			description: "remove block is targeting a module and the current node is a resource of that module",
			nodeAddress: "module.boop.test_instance.foo",
			nodeEndpointsToRemove: []*refactoring.RemoveStatement{
				{
					From: addrs.Module{"boop"},
				},
			},
			wantAction: plans.Forget,
			wantDiags: tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  "Resource going to be removed from the state",
				Detail:   fmt.Sprintf("After this plan gets applied, the resource %s will not be managed anymore by OpenTofu.\n\nIn case you want to manage the resource again, you will have to import it.", "module.boop.test_instance.foo"),
			}),
		},
		{
			description: "remove block is targeting a module and the current node is a resource of one of the module instances",
			nodeAddress: "module.boop[1].test_instance.foo",
			nodeEndpointsToRemove: []*refactoring.RemoveStatement{
				{
					From: addrs.Module{"boop"},
				},
			},
			wantAction: plans.Forget,
			wantDiags: tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  "Resource going to be removed from the state",
				Detail:   fmt.Sprintf("After this plan gets applied, the resource %s will not be managed anymore by OpenTofu.\n\nIn case you want to manage the resource again, you will have to import it.", "module.boop[1].test_instance.foo"),
			}),
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s %s", test.wantAction, test.description), func(t *testing.T) {
			deposedKey := states.NewDeposedKey()
			absResource := mustResourceInstanceAddr(test.nodeAddress)

			evalCtx, p := initMockEvalContext(t.Context(), test.nodeAddress, deposedKey)

			node := NodePlanDeposedResourceInstanceObject{
				NodeAbstractResourceInstance: &NodeAbstractResourceInstance{
					Addr: absResource,
					NodeAbstractResource: NodeAbstractResource{
						ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`)},
					},
				},
				DeposedKey:       deposedKey,
				RemoveStatements: test.nodeEndpointsToRemove,
			}

			gotDiags := node.Execute(t.Context(), evalCtx, walkPlan)
			assertDiags(t, gotDiags, test.wantDiags)

			if !p.UpgradeResourceStateCalled {
				t.Errorf("UpgradeResourceState wasn't called; should've been called to upgrade the previous run's object")
			}
			if !p.ReadResourceCalled {
				t.Errorf("ReadResource wasn't called; should've been called to refresh the deposed object")
			}

			change := evalCtx.Changes().GetResourceInstanceChange(absResource, deposedKey)
			if got, want := change.ChangeSrc.Action, test.wantAction; got != want {
				t.Fatalf("wrong planned action\ngot:  %s\nwant: %s", got, want)
			}
		})
	}
}

func TestNodeDestroyDeposedResourceInstanceObject_Execute(t *testing.T) {
	deposedKey := states.NewDeposedKey()
	state := states.NewState()
	absResourceAddr := "test_instance.foo"
	evalCtx, _ := initMockEvalContext(t.Context(), absResourceAddr, deposedKey)

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
	err := node.Execute(t.Context(), evalCtx, walkApply)

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if !state.Empty() {
		t.Fatalf("resources left in state after destroy")
	}
}

func TestNodeDestroyDeposedResourceInstanceObject_WriteResourceInstanceState(t *testing.T) {
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
	evalCtx.ProviderProvider = mockProvider
	evalCtx.ProviderSchemaSchema = mockProvider.GetProviderSchema(t.Context())

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
	err := node.writeResourceInstanceState(t.Context(), evalCtx, obj)
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
	evalCtx := &MockEvalContext{
		StateState:           states.NewState().SyncWrapper(),
		ProviderProvider:     simpleMockProvider(),
		ProviderSchemaSchema: p.GetProviderSchema(t.Context()),
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
	err := node.Execute(t.Context(), evalCtx, walkApply)

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNodeForgetDeposedResourceInstanceObject_Execute(t *testing.T) {
	deposedKey := states.NewDeposedKey()
	state := states.NewState()
	absResourceAddr := "test_instance.foo"
	evalCtx, _ := initMockEvalContext(t.Context(), absResourceAddr, deposedKey)

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
	err := node.Execute(t.Context(), evalCtx, walkApply)

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if !state.Empty() {
		t.Fatalf("resources left in state after forget")
	}
}

func initMockEvalContext(ctx context.Context, resourceAddrs string, deposedKey states.DeposedKey) (*MockEvalContext, *MockProvider) {
	state := states.NewState()
	absResource := mustResourceInstanceAddr(resourceAddrs)

	if !absResource.Module.IsRoot() {
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
	p.ConfigureProvider(ctx, providers.ConfigureProviderRequest{})
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

func assertDiags(t *testing.T, got, want tfdiags.Diagnostics) {
	if len(got) != len(want) {
		t.Errorf("invalid number of diags wanted %d but got %d.\nwant:\n\t%s\ngot:\n\t%s", len(want), len(got), want, got)
		return
	}
	for _, gd := range got {
		var found bool
		for _, wd := range want {
			if !gd.Description().Equal(wd.Description()) {
				continue
			}
			if !gd.Source().Equal(wd.Source()) {
				continue
			}
			if gd.Severity() != wd.Severity() {
				continue
			}
			if gd.ExtraInfo() != wd.ExtraInfo() {
				continue
			}
			if gd.FromExpr() != wd.FromExpr() {
				continue
			}
			found = true
		}
		if !found {
			t.Errorf("got a diagnostic that is not expected.\nwanted:\n\t%s\ngot:\n\t%s", want, gd)
		}
	}
}
