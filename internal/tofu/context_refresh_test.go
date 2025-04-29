// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/configs/hcl2shim"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
)

func TestContext2Refresh(t *testing.T) {
	p := testProvider("aws")
	m := testModule(t, "refresh-basic")

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.web").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"foo","foo":"bar"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	schema := p.GetProviderSchemaResponse.ResourceTypes["aws_instance"].Block
	ty := schema.ImpliedType()
	readState, err := hcl2shim.HCL2ValueFromFlatmap(map[string]string{"id": "foo", "foo": "baz"}, ty)
	if err != nil {
		t.Fatal(err)
	}

	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: readState,
	}

	s, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	if !p.ReadResourceCalled {
		t.Fatal("ReadResource should be called")
	}

	mod := s.RootModule()
	fromState, err := mod.Resources["aws_instance.web"].Instances[addrs.NoKey].Current.Decode(ty)
	if err != nil {
		t.Fatal(err)
	}

	newState, err := schema.CoerceValue(fromState.Value)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(readState, newState, valueComparer) {
		t.Fatal(cmp.Diff(readState, newState, valueComparer, equateEmpty))
	}
}

func TestContext2Refresh_dynamicAttr(t *testing.T) {
	m := testModule(t, "refresh-dynamic")

	startingState := states.BuildState(func(ss *states.SyncState) {
		ss.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				Status:    states.ObjectReady,
				AttrsJSON: []byte(`{"dynamic":{"type":"string","value":"hello"}}`),
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})

	readStateVal := cty.ObjectVal(map[string]cty.Value{
		"dynamic": cty.EmptyTupleVal,
	})

	p := testProvider("test")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"test_instance": {
				Attributes: map[string]*configschema.Attribute{
					"dynamic": {Type: cty.DynamicPseudoType, Optional: true},
				},
			},
		},
	})
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		return providers.ReadResourceResponse{
			NewState: readStateVal,
		}
	}
	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
		resp.PlannedState = req.ProposedNewState
		return resp
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	schema := p.GetProviderSchemaResponse.ResourceTypes["test_instance"].Block
	ty := schema.ImpliedType()

	s, diags := ctx.Refresh(context.Background(), m, startingState, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	if !p.ReadResourceCalled {
		t.Fatal("ReadResource should be called")
	}

	mod := s.RootModule()
	newState, err := mod.Resources["test_instance.foo"].Instances[addrs.NoKey].Current.Decode(ty)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(readStateVal, newState.Value, valueComparer) {
		t.Error(cmp.Diff(newState.Value, readStateVal, valueComparer, equateEmpty))
	}
}

func TestContext2Refresh_dataComputedModuleVar(t *testing.T) {
	p := testProvider("aws")
	m := testModule(t, "refresh-data-module-var")
	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
		obj := req.ProposedNewState.AsValueMap()
		obj["id"] = cty.UnknownVal(cty.String)
		resp.PlannedState = cty.ObjectVal(obj)
		return resp
	}
	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) (resp providers.ReadDataSourceResponse) {
		resp.State = req.Config
		return resp
	}

	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{},
		ResourceTypes: map[string]*configschema.Block{
			"aws_instance": {
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.String,
						Optional: true,
					},
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
		},
		DataSources: map[string]*configschema.Block{
			"aws_data_source": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Optional: true,
					},
					"output": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
		},
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{Mode: plans.RefreshOnlyMode})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	checkStateString(t, plan.PriorState, `
<no state>
`)
}

func TestContext2Refresh_targeted(t *testing.T) {
	p := testProvider("aws")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{},
		ResourceTypes: map[string]*configschema.Block{
			"aws_elb": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"instances": {
						Type:     cty.Set(cty.String),
						Optional: true,
					},
				},
			},
			"aws_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"vpc_id": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
			"aws_vpc": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
		},
	})

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceCurrent(root, "aws_vpc.metoo", `{"id":"vpc-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.notme", `{"id":"i-bcd345"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.me", `{"id":"i-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_elb.meneither", `{"id":"lb-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)

	m := testModule(t, "refresh-targeted")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	refreshedResources := make([]string, 0, 2)
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		refreshedResources = append(refreshedResources, req.PriorState.GetAttr("id").AsString())
		return providers.ReadResourceResponse{
			NewState: req.PriorState,
		}
	}

	_, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		Targets: []addrs.Targetable{
			addrs.RootModuleInstance.Resource(
				addrs.ManagedResourceMode, "aws_instance", "me",
			),
		},
	})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	expected := []string{"vpc-abc123", "i-abc123"}
	sort.Strings(expected)
	sort.Strings(refreshedResources)
	if !reflect.DeepEqual(refreshedResources, expected) {
		t.Fatalf("expected: %#v, got: %#v", expected, refreshedResources)
	}
}

// All exclude flag tests in this file are inspired by a counterpart target flag test
// Usually that test exists right before the exclude flag test

func TestContext2Refresh_excluded(t *testing.T) {
	p := testProvider("aws")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{},
		ResourceTypes: map[string]*configschema.Block{
			"aws_elb": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"instances": {
						Type:     cty.Set(cty.String),
						Optional: true,
					},
				},
			},
			"aws_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"vpc_id": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
			"aws_vpc": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
		},
	})

	// This test uses the same setup as TestContext2Refresh_targeted, but here the resources that should be refreshed
	// are aws_vpc.metoo and aws_instance.notme. These are resources that are not aws_instance.me or dependent on it
	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceCurrent(root, "aws_vpc.metoo", `{"id":"vpc-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.notme", `{"id":"i-bcd345"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.me", `{"id":"i-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_elb.meneither", `{"id":"lb-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)

	m := testModule(t, "refresh-targeted")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	refreshedResources := make([]string, 0, 2)
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		refreshedResources = append(refreshedResources, req.PriorState.GetAttr("id").AsString())
		return providers.ReadResourceResponse{
			NewState: req.PriorState,
		}
	}

	_, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Resource(
				addrs.ManagedResourceMode, "aws_instance", "me",
			),
		},
	})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	expected := []string{"vpc-abc123", "i-bcd345"}
	sort.Strings(expected)
	sort.Strings(refreshedResources)
	if !reflect.DeepEqual(refreshedResources, expected) {
		t.Fatalf("expected: %#v, got: %#v", expected, refreshedResources)
	}
}

func TestContext2Refresh_targetedCount(t *testing.T) {
	p := testProvider("aws")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{},
		ResourceTypes: map[string]*configschema.Block{
			"aws_elb": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"instances": {
						Type:     cty.Set(cty.String),
						Optional: true,
					},
				},
			},
			"aws_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"vpc_id": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
			"aws_vpc": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
		},
	})

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceCurrent(root, "aws_vpc.metoo", `{"id":"vpc-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.notme", `{"id":"i-bcd345"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.me[0]", `{"id":"i-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.me[1]", `{"id":"i-cde567"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.me[2]", `{"id":"i-cde789"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_elb.meneither", `{"id":"lb-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)

	m := testModule(t, "refresh-targeted-count")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	refreshedResources := make([]string, 0, 2)
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		refreshedResources = append(refreshedResources, req.PriorState.GetAttr("id").AsString())
		return providers.ReadResourceResponse{
			NewState: req.PriorState,
		}
	}

	_, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		Targets: []addrs.Targetable{
			addrs.RootModuleInstance.Resource(
				addrs.ManagedResourceMode, "aws_instance", "me",
			),
		},
	})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	// Target didn't specify index, so we should get all our instances
	expected := []string{
		"vpc-abc123",
		"i-abc123",
		"i-cde567",
		"i-cde789",
	}
	sort.Strings(expected)
	sort.Strings(refreshedResources)
	if !reflect.DeepEqual(refreshedResources, expected) {
		t.Fatalf("wrong result\ngot:  %#v\nwant: %#v", refreshedResources, expected)
	}
}

func TestContext2Refresh_excludedCount(t *testing.T) {
	p := testProvider("aws")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{},
		ResourceTypes: map[string]*configschema.Block{
			"aws_elb": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"instances": {
						Type:     cty.Set(cty.String),
						Optional: true,
					},
				},
			},
			"aws_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"vpc_id": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
			"aws_vpc": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
		},
	})

	// This test uses the same setup as TestContext2Refresh_targetedCount, but here the resources that should be
	// refreshed are aws_vpc.metoo and aws_instance.notme. These are resources that are not aws_instance.me or
	// dependent on it
	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceCurrent(root, "aws_vpc.metoo", `{"id":"vpc-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.notme", `{"id":"i-bcd345"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.me[0]", `{"id":"i-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.me[1]", `{"id":"i-cde567"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.me[2]", `{"id":"i-cde789"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_elb.meneither", `{"id":"lb-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)

	m := testModule(t, "refresh-targeted-count")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	refreshedResources := make([]string, 0, 2)
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		refreshedResources = append(refreshedResources, req.PriorState.GetAttr("id").AsString())
		return providers.ReadResourceResponse{
			NewState: req.PriorState,
		}
	}

	_, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Resource(
				addrs.ManagedResourceMode, "aws_instance", "me",
			),
		},
	})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	// Target didn't specify index, so we should exclude all instances of aws_instance.me
	expected := []string{
		"vpc-abc123",
		"i-bcd345",
	}
	sort.Strings(expected)
	sort.Strings(refreshedResources)
	if !reflect.DeepEqual(refreshedResources, expected) {
		t.Fatalf("wrong result\ngot:  %#v\nwant: %#v", refreshedResources, expected)
	}
}

func TestContext2Refresh_targetedCountIndex(t *testing.T) {
	p := testProvider("aws")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{},
		ResourceTypes: map[string]*configschema.Block{
			"aws_elb": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"instances": {
						Type:     cty.Set(cty.String),
						Optional: true,
					},
				},
			},
			"aws_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"vpc_id": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
			"aws_vpc": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
		},
	})

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceCurrent(root, "aws_vpc.metoo", `{"id":"vpc-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.notme", `{"id":"i-bcd345"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.me[0]", `{"id":"i-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.me[1]", `{"id":"i-cde567"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.me[2]", `{"id":"i-cde789"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_elb.meneither", `{"id":"lb-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)

	m := testModule(t, "refresh-targeted-count")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	refreshedResources := make([]string, 0, 2)
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		refreshedResources = append(refreshedResources, req.PriorState.GetAttr("id").AsString())
		return providers.ReadResourceResponse{
			NewState: req.PriorState,
		}
	}

	_, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		Targets: []addrs.Targetable{
			addrs.RootModuleInstance.ResourceInstance(
				addrs.ManagedResourceMode, "aws_instance", "me", addrs.IntKey(0),
			),
		},
	})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	expected := []string{"vpc-abc123", "i-abc123"}
	sort.Strings(expected)
	sort.Strings(refreshedResources)
	if !reflect.DeepEqual(refreshedResources, expected) {
		t.Fatalf("wrong result\ngot:  %#v\nwant: %#v", refreshedResources, expected)
	}
}

func TestContext2Refresh_excludedCountIndex(t *testing.T) {
	p := testProvider("aws")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{},
		ResourceTypes: map[string]*configschema.Block{
			"aws_elb": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"instances": {
						Type:     cty.Set(cty.String),
						Optional: true,
					},
				},
			},
			"aws_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"vpc_id": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
			"aws_vpc": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
		},
	})

	// This test uses the same setup as TestContext2Refresh_targetedCountIndex, but here the resources that should be
	// refreshed are aws_vpc.metoo, aws_instance.notme, aws_instance.me[1] and aws_instance.me[2]. These are resources
	// that are not aws_instance.me[0] or dependent on it
	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceCurrent(root, "aws_vpc.metoo", `{"id":"vpc-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.notme", `{"id":"i-bcd345"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.me[0]", `{"id":"i-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.me[1]", `{"id":"i-cde567"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_instance.me[2]", `{"id":"i-cde789"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	testSetResourceInstanceCurrent(root, "aws_elb.meneither", `{"id":"lb-abc123"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)

	m := testModule(t, "refresh-targeted-count")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	refreshedResources := make([]string, 0, 2)
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		refreshedResources = append(refreshedResources, req.PriorState.GetAttr("id").AsString())
		return providers.ReadResourceResponse{
			NewState: req.PriorState,
		}
	}

	_, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.ResourceInstance(
				addrs.ManagedResourceMode, "aws_instance", "me", addrs.IntKey(0),
			),
		},
	})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	expected := []string{"vpc-abc123", "i-bcd345", "i-cde567", "i-cde789"}
	sort.Strings(expected)
	sort.Strings(refreshedResources)
	if !reflect.DeepEqual(refreshedResources, expected) {
		t.Fatalf("wrong result\ngot:  %#v\nwant: %#v", refreshedResources, expected)
	}
}

func TestContext2Refresh_moduleComputedVar(t *testing.T) {
	p := testProvider("aws")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{},
		ResourceTypes: map[string]*configschema.Block{
			"aws_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"value": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
		},
	})

	m := testModule(t, "refresh-module-computed-var")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	// This was failing (see GH-2188) at some point, so this test just
	// verifies that the failure goes away.
	if _, diags := ctx.Refresh(context.Background(), m, states.NewState(), &PlanOpts{Mode: plans.NormalMode}); diags.HasErrors() {
		t.Fatalf("refresh errs: %s", diags.Err())
	}
}

func TestContext2Refresh_delete(t *testing.T) {
	p := testProvider("aws")
	m := testModule(t, "refresh-basic")

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceCurrent(root, "aws_instance.web", `{"id":"foo"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.NullVal(p.GetProviderSchemaResponse.ResourceTypes["aws_instance"].Block.ImpliedType()),
	}

	s, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	mod := s.RootModule()
	if len(mod.Resources) > 0 {
		t.Fatal("resources should be empty")
	}
}

func TestContext2Refresh_ignoreUncreated(t *testing.T) {
	p := testProvider("aws")
	m := testModule(t, "refresh-basic")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("foo"),
		}),
	}

	_, diags := ctx.Refresh(context.Background(), m, states.NewState(), &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}
	if p.ReadResourceCalled {
		t.Fatal("refresh should not be called")
	}
}

func TestContext2Refresh_hook(t *testing.T) {
	h := new(MockHook)
	p := testProvider("aws")
	m := testModule(t, "refresh-basic")

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceCurrent(root, "aws_instance.web", `{"id":"foo"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)

	ctx := testContext2(t, &ContextOpts{
		Hooks: []Hook{h},
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	if _, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode}); diags.HasErrors() {
		t.Fatalf("refresh errs: %s", diags.Err())
	}
	if !h.PreRefreshCalled {
		t.Fatal("should be called")
	}
	if !h.PostRefreshCalled {
		t.Fatal("should be called")
	}
}

func TestContext2Refresh_modules(t *testing.T) {
	p := testProvider("aws")
	m := testModule(t, "refresh-modules")

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceTainted(root, "aws_instance.web", `{"id":"bar"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	child := state.EnsureModule(addrs.RootModuleInstance.Child("child", addrs.NoKey))
	testSetResourceInstanceCurrent(child, "aws_instance.web", `{"id":"baz"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		if !req.PriorState.GetAttr("id").RawEquals(cty.StringVal("baz")) {
			return providers.ReadResourceResponse{
				NewState: req.PriorState,
			}
		}

		new, _ := cty.Transform(req.PriorState, func(path cty.Path, v cty.Value) (cty.Value, error) {
			if len(path) == 1 && path[0].(cty.GetAttrStep).Name == "id" {
				return cty.StringVal("new"), nil
			}
			return v, nil
		})
		return providers.ReadResourceResponse{
			NewState: new,
		}
	}

	s, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	actual := strings.TrimSpace(s.String())
	expected := strings.TrimSpace(testContextRefreshModuleStr)
	if actual != expected {
		t.Fatalf("wrong result\n\ngot:\n%s\n\nwant:\n%s", actual, expected)
	}
}

func TestContext2Refresh_moduleInputComputedOutput(t *testing.T) {
	m := testModule(t, "refresh-module-input-computed-output")
	p := testProvider("aws")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{},
		ResourceTypes: map[string]*configschema.Block{
			"aws_instance": {
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.String,
						Optional: true,
						Computed: true,
					},
					"compute": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
		},
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	if _, diags := ctx.Refresh(context.Background(), m, states.NewState(), &PlanOpts{Mode: plans.NormalMode}); diags.HasErrors() {
		t.Fatalf("refresh errs: %s", diags.Err())
	}
}

func TestContext2Refresh_moduleVarModule(t *testing.T) {
	m := testModule(t, "refresh-module-var-module")
	p := testProvider("aws")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	if _, diags := ctx.Refresh(context.Background(), m, states.NewState(), &PlanOpts{Mode: plans.NormalMode}); diags.HasErrors() {
		t.Fatalf("refresh errs: %s", diags.Err())
	}
}

// GH-70
func TestContext2Refresh_noState(t *testing.T) {
	p := testProvider("aws")
	m := testModule(t, "refresh-no-state")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("foo"),
		}),
	}

	if _, diags := ctx.Refresh(context.Background(), m, states.NewState(), &PlanOpts{Mode: plans.NormalMode}); diags.HasErrors() {
		t.Fatalf("refresh errs: %s", diags.Err())
	}
}

func TestContext2Refresh_output(t *testing.T) {
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{},
		ResourceTypes: map[string]*configschema.Block{
			"aws_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"foo": {
						Type:     cty.String,
						Optional: true,
						Computed: true,
					},
				},
			},
		},
	})

	m := testModule(t, "refresh-output")

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceCurrent(root, "aws_instance.web", `{"id":"foo","foo":"bar"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)
	root.SetOutputValue("foo", cty.StringVal("foo"), false, "")

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	s, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	actual := strings.TrimSpace(s.String())
	expected := strings.TrimSpace(testContextRefreshOutputStr)
	if actual != expected {
		t.Fatalf("wrong result\n\ngot:\n%q\n\nwant:\n%q", actual, expected)
	}
}

func TestContext2Refresh_outputPartial(t *testing.T) {
	p := testProvider("aws")
	m := testModule(t, "refresh-output-partial")

	// Refresh creates a partial plan for any instances that don't have
	// remote objects yet, to get stub values for interpolation. Therefore
	// we need to make DiffFn available to let that complete.

	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{},
		ResourceTypes: map[string]*configschema.Block{
			"aws_instance": {
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
		},
	})

	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.NullVal(p.GetProviderSchemaResponse.ResourceTypes["aws_instance"].Block.ImpliedType()),
	}

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceCurrent(root, "aws_instance.foo", `{}`, `provider["registry.opentofu.org/hashicorp/aws"]`)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	s, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	actual := strings.TrimSpace(s.String())
	expected := strings.TrimSpace(testContextRefreshOutputPartialStr)
	if actual != expected {
		t.Fatalf("wrong result\n\ngot:\n%s\n\nwant:\n%s", actual, expected)
	}
}

func TestContext2Refresh_stateBasic(t *testing.T) {
	p := testProvider("aws")
	m := testModule(t, "refresh-basic")

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceCurrent(root, "aws_instance.web", `{"id":"bar"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	schema := p.GetProviderSchemaResponse.ResourceTypes["aws_instance"].Block
	ty := schema.ImpliedType()

	readStateVal, err := schema.CoerceValue(cty.ObjectVal(map[string]cty.Value{
		"id": cty.StringVal("foo"),
	}))
	if err != nil {
		t.Fatal(err)
	}

	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: readStateVal,
	}

	s, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	if !p.ReadResourceCalled {
		t.Fatal("read resource should be called")
	}

	mod := s.RootModule()
	newState, err := mod.Resources["aws_instance.web"].Instances[addrs.NoKey].Current.Decode(ty)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(readStateVal, newState.Value, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(readStateVal, newState.Value, valueComparer, equateEmpty))
	}
}

func TestContext2Refresh_dataCount(t *testing.T) {
	p := testProvider("test")
	m := testModule(t, "refresh-data-count")

	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
		m := req.ProposedNewState.AsValueMap()
		m["things"] = cty.ListVal([]cty.Value{cty.StringVal("foo")})
		resp.PlannedState = cty.ObjectVal(m)
		return resp
	}
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"test": {
				Attributes: map[string]*configschema.Attribute{
					"id":     {Type: cty.String, Computed: true},
					"things": {Type: cty.List(cty.String), Computed: true},
				},
			},
		},
		DataSources: map[string]*configschema.Block{
			"test": {},
		},
	})

	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
		return providers.ReadDataSourceResponse{
			State: req.Config,
		}
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	s, diags := ctx.Refresh(context.Background(), m, states.NewState(), &PlanOpts{Mode: plans.NormalMode})

	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	checkStateString(t, s, `<no state>`)
}

func TestContext2Refresh_dataState(t *testing.T) {
	m := testModule(t, "refresh-data-resource-basic")
	state := states.NewState()
	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"inputs": {
				Type:     cty.Map(cty.String),
				Optional: true,
			},
		},
	}

	p := testProvider("null")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{},
		DataSources: map[string]*configschema.Block{
			"null_data_source": schema,
		},
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("null"): testProviderFuncFixed(p),
		},
	})

	var readStateVal cty.Value

	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
		m := req.Config.AsValueMap()
		readStateVal = cty.ObjectVal(m)

		return providers.ReadDataSourceResponse{
			State: readStateVal,
		}
	}

	s, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	if !p.ReadDataSourceCalled {
		t.Fatal("ReadDataSource should have been called")
	}

	mod := s.RootModule()

	newState, err := mod.Resources["data.null_data_source.testing"].Instances[addrs.NoKey].Current.Decode(schema.ImpliedType())
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(readStateVal, newState.Value, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(readStateVal, newState.Value, valueComparer, equateEmpty))
	}
}

func TestContext2Refresh_dataStateRefData(t *testing.T) {
	p := testProvider("null")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{},
		DataSources: map[string]*configschema.Block{
			"null_data_source": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"foo": {
						Type:     cty.String,
						Optional: true,
					},
					"bar": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
		},
	})

	m := testModule(t, "refresh-data-ref-data")
	state := states.NewState()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("null"): testProviderFuncFixed(p),
		},
	})

	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
		// add the required id
		m := req.Config.AsValueMap()
		m["id"] = cty.StringVal("foo")

		return providers.ReadDataSourceResponse{
			State: cty.ObjectVal(m),
		}
	}

	s, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	actual := strings.TrimSpace(s.String())
	expected := strings.TrimSpace(testTofuRefreshDataRefDataStr)
	if actual != expected {
		t.Fatalf("wrong result\n\ngot:\n%s\n\nwant:\n%s", actual, expected)
	}
}

func TestContext2Refresh_tainted(t *testing.T) {
	p := testProvider("aws")
	m := testModule(t, "refresh-basic")

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceTainted(root, "aws_instance.web", `{"id":"bar"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		// add the required id
		m := req.PriorState.AsValueMap()
		m["id"] = cty.StringVal("foo")

		return providers.ReadResourceResponse{
			NewState: cty.ObjectVal(m),
		}
	}

	s, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}
	if !p.ReadResourceCalled {
		t.Fatal("ReadResource was not called; should have been")
	}

	actual := strings.TrimSpace(s.String())
	expected := strings.TrimSpace(testContextRefreshTaintedStr)
	if actual != expected {
		t.Fatalf("wrong result\n\ngot:\n%s\n\nwant:\n%s", actual, expected)
	}
}

// Doing a Refresh (or any operation really, but Refresh usually
// happens first) with a config with an unknown provider should result in
// an error. The key bug this found was that this wasn't happening if
// Providers was _empty_.
func TestContext2Refresh_unknownProvider(t *testing.T) {
	m := testModule(t, "refresh-unknown-provider")

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceCurrent(root, "aws_instance.web", `{"id":"foo"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)

	c, diags := NewContext(&ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{},
	})
	assertNoDiagnostics(t, diags)

	_, diags = c.Refresh(context.Background(), m, states.NewState(), &PlanOpts{Mode: plans.NormalMode})
	if !diags.HasErrors() {
		t.Fatal("successfully refreshed; want error")
	}

	if got, want := diags.Err().Error(), "Missing required provider"; !strings.Contains(got, want) {
		t.Errorf("missing expected error\nwant substring: %s\ngot:\n%s", want, got)
	}
}

func TestContext2Refresh_vars(t *testing.T) {
	p := testProvider("aws")

	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"ami": {
				Type:     cty.String,
				Optional: true,
			},
			"id": {
				Type:     cty.String,
				Computed: true,
			},
		},
	}

	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider:      &configschema.Block{},
		ResourceTypes: map[string]*configschema.Block{"aws_instance": schema},
	})

	m := testModule(t, "refresh-vars")
	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceCurrent(root, "aws_instance.web", `{"id":"foo"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	readStateVal, err := schema.CoerceValue(cty.ObjectVal(map[string]cty.Value{
		"id": cty.StringVal("foo"),
	}))
	if err != nil {
		t.Fatal(err)
	}

	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: readStateVal,
	}

	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
		return providers.PlanResourceChangeResponse{
			PlannedState: req.ProposedNewState,
		}
	}

	s, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	if !p.ReadResourceCalled {
		t.Fatal("read resource should be called")
	}

	mod := s.RootModule()

	newState, err := mod.Resources["aws_instance.web"].Instances[addrs.NoKey].Current.Decode(schema.ImpliedType())
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(readStateVal, newState.Value, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(readStateVal, newState.Value, valueComparer, equateEmpty))
	}

	for _, r := range mod.Resources {
		if r.Addr.Resource.Type == "" {
			t.Fatalf("no type: %#v", r)
		}
	}
}

func TestContext2Refresh_orphanModule(t *testing.T) {
	p := testProvider("aws")
	m := testModule(t, "refresh-module-orphan")

	// Create a custom refresh function to track the order they were visited
	var order []string
	var orderLock sync.Mutex
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		orderLock.Lock()
		defer orderLock.Unlock()

		order = append(order, req.PriorState.GetAttr("id").AsString())
		return providers.ReadResourceResponse{
			NewState: req.PriorState,
		}
	}

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"i-abc123"}`),
			Dependencies: []addrs.ConfigResource{
				{Module: addrs.Module{"module.child"}},
				{Module: addrs.Module{"module.child"}},
			},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	child := state.EnsureModule(addrs.RootModuleInstance.Child("child", addrs.NoKey))
	child.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.bar").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"i-bcd23"}`),
			Dependencies: []addrs.ConfigResource{{Module: addrs.Module{"module.grandchild"}}},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	grandchild := state.EnsureModule(addrs.RootModuleInstance.Child("child", addrs.NoKey).Child("grandchild", addrs.NoKey))
	testSetResourceInstanceCurrent(grandchild, "aws_instance.baz", `{"id":"i-cde345"}`, `provider["registry.opentofu.org/hashicorp/aws"]`)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	testCheckDeadlock(t, func() {
		_, err := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode})
		if err != nil {
			t.Fatalf("err: %s", err.Err())
		}

		// TODO: handle order properly for orphaned modules / resources
		// expected := []string{"i-abc123", "i-bcd234", "i-cde345"}
		// if !reflect.DeepEqual(order, expected) {
		// 	t.Fatalf("expected: %#v, got: %#v", expected, order)
		// }
	})
}

func TestContext2Validate(t *testing.T) {
	p := testProvider("aws")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{},
		ResourceTypes: map[string]*configschema.Block{
			"aws_instance": {
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.String,
						Optional: true,
					},
					"num": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
		},
	})

	m := testModule(t, "validate-good")
	c := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	diags := c.Validate(context.Background(), m)
	if len(diags) != 0 {
		t.Fatalf("unexpected error: %#v", diags.ErrWithWarnings())
	}
}

func TestContext2Refresh_updateProviderInState(t *testing.T) {
	m := testModule(t, "update-resource-provider")
	p := testProvider("aws")

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceCurrent(root, "aws_instance.bar", `{"id":"foo"}`, `provider["registry.opentofu.org/hashicorp/aws"].baz`)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	expected := strings.TrimSpace(`
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"].foo`)

	s, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	actual := s.String()
	if actual != expected {
		t.Fatalf("expected:\n%s\n\ngot:\n%s", expected, actual)
	}
}

func TestContext2Refresh_schemaUpgradeFlatmap(t *testing.T) {
	m := testModule(t, "refresh-schema-upgrade")
	p := testProvider("test")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"test_thing": {
				Attributes: map[string]*configschema.Attribute{
					"name": { // imagining we renamed this from "id"
						Type:     cty.String,
						Optional: true,
					},
				},
			},
		},
		ResourceTypeSchemaVersions: map[string]uint64{
			"test_thing": 5,
		},
	})
	p.UpgradeResourceStateResponse = &providers.UpgradeResourceStateResponse{
		UpgradedState: cty.ObjectVal(map[string]cty.Value{
			"name": cty.StringVal("foo"),
		}),
	}

	s := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_thing",
				Name: "bar",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				Status:        states.ObjectReady,
				SchemaVersion: 3,
				AttrsFlat: map[string]string{
					"id": "foo",
				},
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	state, diags := ctx.Refresh(context.Background(), m, s, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	{
		got := p.UpgradeResourceStateRequest
		want := providers.UpgradeResourceStateRequest{
			TypeName: "test_thing",
			Version:  3,
			RawStateFlatmap: map[string]string{
				"id": "foo",
			},
		}
		if !cmp.Equal(got, want) {
			t.Errorf("wrong upgrade request\n%s", cmp.Diff(want, got))
		}
	}

	{
		got := state.String()
		want := strings.TrimSpace(`
test_thing.bar:
  ID = 
  provider = provider["registry.opentofu.org/hashicorp/test"]
  name = foo
`)
		if got != want {
			t.Fatalf("wrong result state\ngot:\n%s\n\nwant:\n%s", got, want)
		}
	}
}

func TestContext2Refresh_schemaUpgradeJSON(t *testing.T) {
	m := testModule(t, "refresh-schema-upgrade")
	p := testProvider("test")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"test_thing": {
				Attributes: map[string]*configschema.Attribute{
					"name": { // imagining we renamed this from "id"
						Type:     cty.String,
						Optional: true,
					},
				},
			},
		},
		ResourceTypeSchemaVersions: map[string]uint64{
			"test_thing": 5,
		},
	})
	p.UpgradeResourceStateResponse = &providers.UpgradeResourceStateResponse{
		UpgradedState: cty.ObjectVal(map[string]cty.Value{
			"name": cty.StringVal("foo"),
		}),
	}

	s := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_thing",
				Name: "bar",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				Status:        states.ObjectReady,
				SchemaVersion: 3,
				AttrsJSON:     []byte(`{"id":"foo"}`),
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	state, diags := ctx.Refresh(context.Background(), m, s, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	{
		got := p.UpgradeResourceStateRequest
		want := providers.UpgradeResourceStateRequest{
			TypeName:     "test_thing",
			Version:      3,
			RawStateJSON: []byte(`{"id":"foo"}`),
		}
		if !cmp.Equal(got, want) {
			t.Errorf("wrong upgrade request\n%s", cmp.Diff(want, got))
		}
	}

	{
		got := state.String()
		want := strings.TrimSpace(`
test_thing.bar:
  ID = 
  provider = provider["registry.opentofu.org/hashicorp/test"]
  name = foo
`)
		if got != want {
			t.Fatalf("wrong result state\ngot:\n%s\n\nwant:\n%s", got, want)
		}
	}
}

func TestContext2Refresh_dataValidation(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
data "aws_data_source" "foo" {
  foo = "bar"
}
`,
	})

	p := testProvider("aws")
	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
		resp.PlannedState = req.ProposedNewState
		return
	}
	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) (resp providers.ReadDataSourceResponse) {
		resp.State = req.Config
		return
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Refresh(context.Background(), m, states.NewState(), &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		// Should get this error:
		// Unsupported attribute: This object does not have an attribute named "missing"
		t.Fatal(diags.Err())
	}

	if !p.ValidateDataResourceConfigCalled {
		t.Fatal("ValidateDataSourceConfig not called during plan")
	}
}

func TestContext2Refresh_dataResourceDependsOn(t *testing.T) {
	m := testModule(t, "plan-data-depends-on")
	p := testProvider("test")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"test_resource": {
				Attributes: map[string]*configschema.Attribute{
					"id":  {Type: cty.String, Computed: true},
					"foo": {Type: cty.String, Optional: true},
				},
			},
		},
		DataSources: map[string]*configschema.Block{
			"test_data": {
				Attributes: map[string]*configschema.Attribute{
					"compute": {Type: cty.String, Computed: true},
				},
			},
		},
	})
	p.ReadDataSourceResponse = &providers.ReadDataSourceResponse{
		State: cty.ObjectVal(map[string]cty.Value{
			"compute": cty.StringVal("value"),
		}),
	}

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	testSetResourceInstanceCurrent(root, "test_resource.a", `{"id":"a"}`, `provider["registry.opentofu.org/hashicorp/test"]`)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}
}

// verify that create_before_destroy is updated in the state during refresh
func TestRefresh_updateLifecycle(t *testing.T) {
	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "aws_instance",
			Name: "bar",
		}.Instance(addrs.NoKey),
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"bar"}`),
		},
		addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("aws"),
			Module:   addrs.RootModule,
		},
		addrs.NoKey,
	)

	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "aws_instance" "bar" {
  lifecycle {
    create_before_destroy = true
  }
}
`,
	})

	p := testProvider("aws")

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	state, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatalf("plan errors: %s", diags.Err())
	}

	r := state.ResourceInstance(mustResourceInstanceAddr("aws_instance.bar"))
	if !r.Current.CreateBeforeDestroy {
		t.Fatal("create_before_destroy not updated in instance state")
	}
}

func TestContext2Refresh_dataSourceOrphan(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": ``,
	})

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		addrs.Resource{
			Mode: addrs.DataResourceMode,
			Type: "test_data_source",
			Name: "foo",
		}.Instance(addrs.NoKey),
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"foo"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("test"),
			Module:   addrs.RootModule,
		},
		addrs.NoKey,
	)
	p := testProvider("test")
	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) (resp providers.ReadDataSourceResponse) {
		resp.State = cty.NullVal(req.Config.Type())
		return
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Refresh(context.Background(), m, state, &PlanOpts{Mode: plans.NormalMode})
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	if p.ReadResourceCalled {
		t.Fatal("there are no managed resources to read")
	}

	if p.ReadDataSourceCalled {
		t.Fatal("orphaned data source instance should not be read")
	}
}

// Legacy providers may return invalid null values for blocks, causing noise in
// the diff output and unexpected behavior with ignore_changes. Make sure
// refresh fixes these up before storing the state.
func TestContext2Refresh_reifyNullBlock(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_resource" "foo" {
}
`,
	})

	p := new(MockProvider)
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		// incorrectly return a null _set_block value
		v := req.PriorState.AsValueMap()
		v["set_block"] = cty.NullVal(v["set_block"].Type())
		return providers.ReadResourceResponse{NewState: cty.ObjectVal(v)}
	}

	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{},
		ResourceTypes: map[string]*configschema.Block{
			"test_resource": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
				BlockTypes: map[string]*configschema.NestedBlock{
					"set_block": {
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"a": {Type: cty.String, Optional: true},
							},
						},
						Nesting: configschema.NestingSet,
					},
				},
			},
		},
	})
	p.PlanResourceChangeFn = testDiffFn

	fooAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test_resource",
		Name: "foo",
	}.Instance(addrs.NoKey)

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		fooAddr,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"foo", "network_interface":[]}`),
			Dependencies: []addrs.ConfigResource{},
		},
		addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("test"),
			Module:   addrs.RootModule,
		},
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{Mode: plans.RefreshOnlyMode})
	if diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}

	jsonState := plan.PriorState.ResourceInstance(fooAddr.Absolute(addrs.RootModuleInstance)).Current.AttrsJSON

	// the set_block should still be an empty container, and not null
	expected := `{"id":"foo","set_block":[]}`
	if string(jsonState) != expected {
		t.Fatalf("invalid state\nexpected: %s\ngot: %s\n", expected, jsonState)
	}
}
