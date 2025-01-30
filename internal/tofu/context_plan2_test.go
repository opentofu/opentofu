// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"

	// "github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestContext2Plan_removedDuringRefresh(t *testing.T) {
	// This tests the situation where an object tracked in the previous run
	// state has been deleted outside OpenTofu, which we should detect
	// during the refresh step and thus ultimately produce a plan to recreate
	// the object, since it's still present in the configuration.
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
}
`,
	})

	p := simpleMockProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{Block: simpleTestSchema()},
		ResourceTypes: map[string]providers.Schema{
			"test_object": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"arg": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}
	p.ReadResourceFn = func(req providers.ReadResourceRequest) (resp providers.ReadResourceResponse) {
		resp.NewState = cty.NullVal(req.PriorState.Type())
		return resp
	}
	p.UpgradeResourceStateFn = func(req providers.UpgradeResourceStateRequest) (resp providers.UpgradeResourceStateResponse) {
		// We should've been given the prior state JSON as our input to upgrade.
		if !bytes.Contains(req.RawStateJSON, []byte("previous_run")) {
			t.Fatalf("UpgradeResourceState request doesn't contain the previous run object\n%s", req.RawStateJSON)
		}

		// We'll put something different in "arg" as part of upgrading, just
		// so that we can verify below that PrevRunState contains the upgraded
		// (but NOT refreshed) version of the object.
		resp.UpgradedState = cty.ObjectVal(map[string]cty.Value{
			"arg": cty.StringVal("upgraded"),
		})
		return resp
	}

	addr := mustResourceInstanceAddr("test_object.a")
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addr, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{"arg":"previous_run"}`),
			Status:    states.ObjectTainted,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, DefaultPlanOpts)
	assertNoErrors(t, diags)

	if !p.UpgradeResourceStateCalled {
		t.Errorf("Provider's UpgradeResourceState wasn't called; should've been")
	}
	if !p.ReadResourceCalled {
		t.Errorf("Provider's ReadResource wasn't called; should've been")
	}

	// The object should be absent from the plan's prior state, because that
	// records the result of refreshing.
	if got := plan.PriorState.ResourceInstance(addr); got != nil {
		t.Errorf(
			"instance %s is in the prior state after planning; should've been removed\n%s",
			addr, spew.Sdump(got),
		)
	}

	// However, the object should still be in the PrevRunState, because
	// that reflects what we believed to exist before refreshing.
	if got := plan.PrevRunState.ResourceInstance(addr); got == nil {
		t.Errorf(
			"instance %s is missing from the previous run state after planning; should've been preserved",
			addr,
		)
	} else {
		if !bytes.Contains(got.Current.AttrsJSON, []byte("upgraded")) {
			t.Fatalf("previous run state has non-upgraded object\n%s", got.Current.AttrsJSON)
		}
	}

	// This situation should result in a drifted resource change.
	var drifted *plans.ResourceInstanceChangeSrc
	for _, dr := range plan.DriftedResources {
		if dr.Addr.Equal(addr) {
			drifted = dr
			break
		}
	}

	if drifted == nil {
		t.Errorf("instance %s is missing from the drifted resource changes", addr)
	} else {
		if got, want := drifted.Action, plans.Delete; got != want {
			t.Errorf("unexpected instance %s drifted resource change action. got: %s, want: %s", addr, got, want)
		}
	}

	// Because the configuration still mentions test_object.a, we should've
	// planned to recreate it in order to fix the drift.
	for _, c := range plan.Changes.Resources {
		if c.Action != plans.Create {
			t.Fatalf("expected Create action for missing %s, got %s", c.Addr, c.Action)
		}
	}
}

func TestContext2Plan_noChangeDataSourceSensitiveNestedSet(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
variable "bar" {
  sensitive = true
  default   = "baz"
}

data "test_data_source" "foo" {
  foo {
    bar = var.bar
  }
}
`,
	})

	p := new(MockProvider)
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		DataSources: map[string]*configschema.Block{
			"test_data_source": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"bar": {Type: cty.String, Optional: true},
							},
						},
						Nesting: configschema.NestingSet,
					},
				},
			},
		},
	})

	p.ReadDataSourceResponse = &providers.ReadDataSourceResponse{
		State: cty.ObjectVal(map[string]cty.Value{
			"id":  cty.StringVal("data_id"),
			"foo": cty.SetVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{"bar": cty.StringVal("baz")})}),
		}),
	}

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("data.test_data_source.foo").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"data_id", "foo":[{"bar":"baz"}]}`),
			AttrSensitivePaths: []cty.PathValueMarks{
				{
					Path:  cty.GetAttrPath("foo"),
					Marks: cty.NewValueMarks(marks.Sensitive),
				},
			},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, SimplePlanOpts(plans.NormalMode, testInputValuesUnset(m.Module.Variables)))
	assertNoErrors(t, diags)

	for _, res := range plan.Changes.Resources {
		if res.Action != plans.NoOp {
			t.Fatalf("expected NoOp, got: %q %s", res.Addr, res.Action)
		}
	}
}

func TestContext2Plan_orphanDataInstance(t *testing.T) {
	// ensure the planned replacement of the data source is evaluated properly
	m := testModuleInline(t, map[string]string{
		"main.tf": `
data "test_object" "a" {
  for_each = { new = "ok" }
}

output "out" {
  value = [ for k, _ in data.test_object.a: k ]
}
`,
	})

	p := simpleMockProvider()
	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) (resp providers.ReadDataSourceResponse) {
		resp.State = req.Config
		return resp
	}

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(mustResourceInstanceAddr(`data.test_object.a["old"]`), &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{"test_string":"foo"}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, DefaultPlanOpts)
	assertNoErrors(t, diags)

	change, err := plan.Changes.Outputs[0].Decode()
	if err != nil {
		t.Fatal(err)
	}

	expected := cty.TupleVal([]cty.Value{cty.StringVal("new")})

	if change.After.Equals(expected).False() {
		t.Fatalf("expected %#v, got %#v\n", expected, change.After)
	}
}

func TestContext2Plan_basicConfigurationAliases(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
provider "test" {
  alias = "z"
  test_string = "config"
}

module "mod" {
  source = "./mod"
  providers = {
    test.x = test.z
  }
}
`,

		"mod/main.tf": `
terraform {
  required_providers {
    test = {
      source = "registry.opentofu.org/hashicorp/test"
      configuration_aliases = [ test.x ]
	}
  }
}

resource "test_object" "a" {
  provider = test.x
}

`,
	})

	p := simpleMockProvider()

	// The resource within the module should be using the provider configured
	// from the root module. We should never see an empty configuration.
	p.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) (resp providers.ConfigureProviderResponse) {
		if req.Config.GetAttr("test_string").IsNull() {
			resp.Diagnostics = resp.Diagnostics.Append(errors.New("missing test_string value"))
		}
		return resp
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
	assertNoErrors(t, diags)
}

func TestContext2Plan_dataReferencesResourceInModules(t *testing.T) {
	p := testProvider("test")
	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) (resp providers.ReadDataSourceResponse) {
		cfg := req.Config.AsValueMap()
		cfg["id"] = cty.StringVal("d")
		resp.State = cty.ObjectVal(cfg)
		return resp
	}

	m := testModuleInline(t, map[string]string{
		"main.tf": `
locals {
  things = {
    old = "first"
    new = "second"
  }
}

module "mod" {
  source = "./mod"
  for_each = local.things
}
`,

		"./mod/main.tf": `
resource "test_resource" "a" {
}

data "test_data_source" "d" {
  depends_on = [test_resource.a]
}

resource "test_resource" "b" {
  value = data.test_data_source.d.id
}
`})

	oldDataAddr := mustResourceInstanceAddr(`module.mod["old"].data.test_data_source.d`)

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			mustResourceInstanceAddr(`module.mod["old"].test_resource.a`),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"a"}`),
				Status:    states.ObjectReady,
			}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
			addrs.NoKey,
		)
		s.SetResourceInstanceCurrent(
			mustResourceInstanceAddr(`module.mod["old"].test_resource.b`),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"b","value":"d"}`),
				Status:    states.ObjectReady,
			}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
			addrs.NoKey,
		)
		s.SetResourceInstanceCurrent(
			oldDataAddr,
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"d"}`),
				Status:    states.ObjectReady,
			}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
			addrs.NoKey,
		)
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, DefaultPlanOpts)
	assertNoErrors(t, diags)

	oldMod := oldDataAddr.Module

	for _, c := range plan.Changes.Resources {
		// there should be no changes from the old module instance
		if c.Addr.Module.Equal(oldMod) && c.Action != plans.NoOp {
			t.Errorf("unexpected change %s for %s\n", c.Action, c.Addr)
		}
	}
}

func TestContext2Plan_resourceChecksInExpandedModule(t *testing.T) {
	// When a resource is in a nested module we have two levels of expansion
	// to do: first expand the module the resource is declared in, and then
	// expand the resource itself.
	//
	// In earlier versions of Terraform we did that expansion as two levels
	// of DynamicExpand, which led to a bug where we didn't have any central
	// location from which to register all of the instances of a checkable
	// resource.
	//
	// We now handle the full expansion all in one graph node and one dynamic
	// subgraph, which avoids the problem. This is a regression test for the
	// earlier bug. If this test is panicking with "duplicate checkable objects
	// report" then that suggests the bug is reintroduced and we're now back
	// to reporting each module instance separately again, which is incorrect.

	p := testProvider("test")
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{
			Block: &configschema.Block{},
		},
		ResourceTypes: map[string]providers.Schema{
			"test": {
				Block: &configschema.Block{},
			},
		},
	}
	p.ReadResourceFn = func(req providers.ReadResourceRequest) (resp providers.ReadResourceResponse) {
		resp.NewState = req.PriorState
		return resp
	}
	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
		resp.PlannedState = cty.EmptyObjectVal
		return resp
	}
	p.ApplyResourceChangeFn = func(req providers.ApplyResourceChangeRequest) (resp providers.ApplyResourceChangeResponse) {
		resp.NewState = req.PlannedState
		return resp
	}

	m := testModuleInline(t, map[string]string{
		"main.tf": `
			module "child" {
				source = "./child"
				count = 2 # must be at least 2 for this test to be valid
			}
		`,
		"child/child.tf": `
			locals {
				a = "a"
			}

			resource "test" "test1" {
				lifecycle {
					postcondition {
						# It doesn't matter what this checks as long as it
						# passes, because if we don't handle expansion properly
						# then we'll crash before we even get to evaluating this.
						condition = local.a == local.a
						error_message = "Postcondition failed."
					}
				}
			}

			resource "test" "test2" {
				count = 2

				lifecycle {
					postcondition {
						# It doesn't matter what this checks as long as it
						# passes, because if we don't handle expansion properly
						# then we'll crash before we even get to evaluating this.
						condition = local.a == local.a
						error_message = "Postcondition failed."
					}
				}
			}
		`,
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	priorState := states.NewState()
	plan, diags := ctx.Plan(context.Background(), m, priorState, DefaultPlanOpts)
	assertNoErrors(t, diags)

	resourceInsts := []addrs.AbsResourceInstance{
		mustResourceInstanceAddr("module.child[0].test.test1"),
		mustResourceInstanceAddr("module.child[0].test.test2[0]"),
		mustResourceInstanceAddr("module.child[0].test.test2[1]"),
		mustResourceInstanceAddr("module.child[1].test.test1"),
		mustResourceInstanceAddr("module.child[1].test.test2[0]"),
		mustResourceInstanceAddr("module.child[1].test.test2[1]"),
	}

	for _, instAddr := range resourceInsts {
		t.Run(fmt.Sprintf("results for %s", instAddr), func(t *testing.T) {
			if rc := plan.Changes.ResourceInstance(instAddr); rc != nil {
				if got, want := rc.Action, plans.Create; got != want {
					t.Errorf("wrong action for %s\ngot:  %s\nwant: %s", instAddr, got, want)
				}
				if got, want := rc.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
					t.Errorf("wrong action reason for %s\ngot:  %s\nwant: %s", instAddr, got, want)
				}
			} else {
				t.Errorf("no planned change for %s", instAddr)
			}

			if checkResult := plan.Checks.GetObjectResult(instAddr); checkResult != nil {
				if got, want := checkResult.Status, checks.StatusPass; got != want {
					t.Errorf("wrong check status for %s\ngot:  %s\nwant: %s", instAddr, got, want)
				}
			} else {
				t.Errorf("no check result for %s", instAddr)
			}
		})
	}
}

func TestContext2Plan_dataResourceChecksManagedResourceChange(t *testing.T) {
	// This tests the situation where the remote system contains data that
	// isn't valid per a data resource postcondition, but that the
	// configuration is destined to make the remote system valid during apply
	// and so we must defer reading the data resource and checking its
	// conditions until the apply step.
	//
	// This is an exception to the rule tested in
	// TestContext2Plan_dataReferencesResourceIndirectly which is relevant
	// whenever there's at least one precondition or postcondition attached
	// to a data resource.
	//
	// See TestContext2Plan_managedResourceChecksOtherManagedResourceChange for
	// an incorrect situation where a data resource is used only indirectly
	// to drive a precondition elsewhere, which therefore doesn't achieve this
	// special exception.

	p := testProvider("test")
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{
			Block: &configschema.Block{},
		},
		ResourceTypes: map[string]providers.Schema{
			"test_resource": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id": {
							Type:     cty.String,
							Computed: true,
						},
						"valid": {
							Type:     cty.Bool,
							Required: true,
						},
					},
				},
			},
		},
		DataSources: map[string]providers.Schema{
			"test_data_source": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id": {
							Type:     cty.String,
							Required: true,
						},
						"valid": {
							Type:     cty.Bool,
							Computed: true,
						},
					},
				},
			},
		},
	}
	var mu sync.Mutex
	validVal := cty.False
	p.ReadResourceFn = func(req providers.ReadResourceRequest) (resp providers.ReadResourceResponse) {
		// NOTE: This assumes that the prior state declared below will have
		// "valid" set to false already, and thus will match validVal above.
		resp.NewState = req.PriorState
		return resp
	}
	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) (resp providers.ReadDataSourceResponse) {
		cfg := req.Config.AsValueMap()
		mu.Lock()
		cfg["valid"] = validVal
		mu.Unlock()
		resp.State = cty.ObjectVal(cfg)
		return resp
	}
	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
		cfg := req.Config.AsValueMap()
		prior := req.PriorState.AsValueMap()
		resp.PlannedState = cty.ObjectVal(map[string]cty.Value{
			"id":    prior["id"],
			"valid": cfg["valid"],
		})
		return resp
	}
	p.ApplyResourceChangeFn = func(req providers.ApplyResourceChangeRequest) (resp providers.ApplyResourceChangeResponse) {
		planned := req.PlannedState.AsValueMap()

		mu.Lock()
		validVal = planned["valid"]
		mu.Unlock()

		resp.NewState = req.PlannedState
		return resp
	}

	m := testModuleInline(t, map[string]string{
		"main.tf": `

resource "test_resource" "a" {
	valid = true
}

locals {
	# NOTE: We intentionally read through a local value here to make sure
	# that this behavior still works even if there isn't a direct dependency
	# between the data resource and the managed resource.
	object_id = test_resource.a.id
}

data "test_data_source" "a" {
	id = local.object_id

	lifecycle {
		postcondition {
			condition     = self.valid
			error_message = "Not valid!"
		}
	}
}
`})

	managedAddr := mustResourceInstanceAddr(`test_resource.a`)
	dataAddr := mustResourceInstanceAddr(`data.test_data_source.a`)

	// This state is intended to represent the outcome of a previous apply that
	// failed due to postcondition failure but had already updated the
	// relevant object to be invalid.
	//
	// It could also potentially represent a similar situation where the
	// previous apply succeeded but there has been a change outside of
	// OpenTofu that made it invalid, although technically in that scenario
	// the state data would become invalid only during the planning step. For
	// our purposes here that's close enough because we don't have a real
	// remote system in place anyway.
	priorState := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			managedAddr,
			&states.ResourceInstanceObjectSrc{
				// NOTE: "valid" is false here but is true in the configuration
				// above, which is intended to represent that applying the
				// configuration change would make this object become valid.
				AttrsJSON: []byte(`{"id":"boop","valid":false}`),
				Status:    states.ObjectReady,
			}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
			addrs.NoKey,
		)
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, priorState, DefaultPlanOpts)
	assertNoErrors(t, diags)

	if rc := plan.Changes.ResourceInstance(dataAddr); rc != nil {
		if got, want := rc.Action, plans.Read; got != want {
			t.Errorf("wrong action for %s\ngot:  %s\nwant: %s", dataAddr, got, want)
		}
		if got, want := rc.ActionReason, plans.ResourceInstanceReadBecauseDependencyPending; got != want {
			t.Errorf("wrong action reason for %s\ngot:  %s\nwant: %s", dataAddr, got, want)
		}
	} else {
		t.Fatalf("no planned change for %s", dataAddr)
	}

	if rc := plan.Changes.ResourceInstance(managedAddr); rc != nil {
		if got, want := rc.Action, plans.Update; got != want {
			t.Errorf("wrong action for %s\ngot:  %s\nwant: %s", managedAddr, got, want)
		}
		if got, want := rc.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
			t.Errorf("wrong action reason for %s\ngot:  %s\nwant: %s", managedAddr, got, want)
		}
	} else {
		t.Fatalf("no planned change for %s", managedAddr)
	}

	// This is primarily a plan-time test, since the special handling of
	// data resources is a plan-time concern, but we'll still try applying the
	// plan here just to make sure it's valid.
	newState, diags := ctx.Apply(context.Background(), plan, m)
	assertNoErrors(t, diags)

	if rs := newState.ResourceInstance(dataAddr); rs != nil {
		if !rs.HasCurrent() {
			t.Errorf("no final state for %s", dataAddr)
		}
	} else {
		t.Errorf("no final state for %s", dataAddr)
	}

	if rs := newState.ResourceInstance(managedAddr); rs != nil {
		if !rs.HasCurrent() {
			t.Errorf("no final state for %s", managedAddr)
		}
	} else {
		t.Errorf("no final state for %s", managedAddr)
	}

	if got, want := validVal, cty.True; got != want {
		t.Errorf("wrong final valid value\ngot:  %#v\nwant: %#v", got, want)
	}

}

func TestContext2Plan_managedResourceChecksOtherManagedResourceChange(t *testing.T) {
	// This tests the incorrect situation where a managed resource checks
	// another managed resource indirectly via a data resource.
	// This doesn't work because OpenTofu can't tell that the data resource
	// outcome will be updated by a separate managed resource change and so
	// we expect it to fail.
	// This would ideally have worked except that we previously included a
	// special case in the rules for data resources where they only consider
	// direct dependencies when deciding whether to defer (except when the
	// data resource itself has conditions) and so they can potentially
	// read "too early" if the user creates the explicitly-not-recommended
	// situation of a data resource and a managed resource in the same
	// configuration both representing the same remote object.

	p := testProvider("test")
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{
			Block: &configschema.Block{},
		},
		ResourceTypes: map[string]providers.Schema{
			"test_resource": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id": {
							Type:     cty.String,
							Computed: true,
						},
						"valid": {
							Type:     cty.Bool,
							Required: true,
						},
					},
				},
			},
		},
		DataSources: map[string]providers.Schema{
			"test_data_source": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id": {
							Type:     cty.String,
							Required: true,
						},
						"valid": {
							Type:     cty.Bool,
							Computed: true,
						},
					},
				},
			},
		},
	}
	var mu sync.Mutex
	validVal := cty.False
	p.ReadResourceFn = func(req providers.ReadResourceRequest) (resp providers.ReadResourceResponse) {
		// NOTE: This assumes that the prior state declared below will have
		// "valid" set to false already, and thus will match validVal above.
		resp.NewState = req.PriorState
		return resp
	}
	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) (resp providers.ReadDataSourceResponse) {
		cfg := req.Config.AsValueMap()
		if cfg["id"].AsString() == "main" {
			mu.Lock()
			cfg["valid"] = validVal
			mu.Unlock()
		}
		resp.State = cty.ObjectVal(cfg)
		return resp
	}
	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
		cfg := req.Config.AsValueMap()
		prior := req.PriorState.AsValueMap()
		resp.PlannedState = cty.ObjectVal(map[string]cty.Value{
			"id":    prior["id"],
			"valid": cfg["valid"],
		})
		return resp
	}
	p.ApplyResourceChangeFn = func(req providers.ApplyResourceChangeRequest) (resp providers.ApplyResourceChangeResponse) {
		planned := req.PlannedState.AsValueMap()

		if planned["id"].AsString() == "main" {
			mu.Lock()
			validVal = planned["valid"]
			mu.Unlock()
		}

		resp.NewState = req.PlannedState
		return resp
	}

	m := testModuleInline(t, map[string]string{
		"main.tf": `

resource "test_resource" "a" {
  valid = true
}

locals {
	# NOTE: We intentionally read through a local value here because a
	# direct reference from data.test_data_source.a to test_resource.a would
	# cause OpenTofu to defer the data resource to the apply phase due to
	# there being a pending change for the managed resource. We're explicitly
	# testing the failure case where the data resource read happens too
	# eagerly, which is what results from the reference being only indirect
	# so OpenTofu can't "see" that the data resource result might be affected
	# by changes to the managed resource.
	object_id = test_resource.a.id
}

data "test_data_source" "a" {
	id = local.object_id
}

resource "test_resource" "b" {
	valid = true

	lifecycle {
		precondition {
			condition     = data.test_data_source.a.valid
			error_message = "Not valid!"
		}
	}
}
`})

	managedAddrA := mustResourceInstanceAddr(`test_resource.a`)
	managedAddrB := mustResourceInstanceAddr(`test_resource.b`)

	// This state is intended to represent the outcome of a previous apply that
	// failed due to postcondition failure but had already updated the
	// relevant object to be invalid.
	//
	// It could also potentially represent a similar situation where the
	// previous apply succeeded but there has been a change outside of
	// OpenTofu that made it invalid, although technically in that scenario
	// the state data would become invalid only during the planning step. For
	// our purposes here that's close enough because we don't have a real
	// remote system in place anyway.
	priorState := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			managedAddrA,
			&states.ResourceInstanceObjectSrc{
				// NOTE: "valid" is false here but is true in the configuration
				// above, which is intended to represent that applying the
				// configuration change would make this object become valid.
				AttrsJSON: []byte(`{"id":"main","valid":false}`),
				Status:    states.ObjectReady,
			}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
			addrs.NoKey,
		)
		s.SetResourceInstanceCurrent(
			managedAddrB,
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"checker","valid":true}`),
				Status:    states.ObjectReady,
			}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
			addrs.NoKey,
		)
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, priorState, DefaultPlanOpts)
	if !diags.HasErrors() {
		t.Fatalf("unexpected successful plan; should've failed with non-passing precondition")
	}

	if got, want := diags.Err().Error(), "Resource precondition failed: Not valid!"; !strings.Contains(got, want) {
		t.Errorf("Missing expected error message\ngot: %s\nwant substring: %s", got, want)
	}
}

func TestContext2Plan_destroyWithRefresh(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
}
`,
	})

	p := simpleMockProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{Block: simpleTestSchema()},
		ResourceTypes: map[string]providers.Schema{
			"test_object": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"arg": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}

	// This is called from the first instance of this provider, so we can't
	// check p.ReadResourceCalled after plan.
	readResourceCalled := false
	p.ReadResourceFn = func(req providers.ReadResourceRequest) (resp providers.ReadResourceResponse) {
		readResourceCalled = true
		newVal, err := cty.Transform(req.PriorState, func(path cty.Path, v cty.Value) (cty.Value, error) {
			if len(path) == 1 && path[0] == (cty.GetAttrStep{Name: "arg"}) {
				return cty.StringVal("current"), nil
			}
			return v, nil
		})
		if err != nil {
			// shouldn't get here
			t.Fatalf("ReadResourceFn transform failed")
			return providers.ReadResourceResponse{}
		}
		return providers.ReadResourceResponse{
			NewState: newVal,
		}
	}

	upgradeResourceStateCalled := false
	p.UpgradeResourceStateFn = func(req providers.UpgradeResourceStateRequest) (resp providers.UpgradeResourceStateResponse) {
		upgradeResourceStateCalled = true
		t.Logf("UpgradeResourceState %s", req.RawStateJSON)

		// In the destroy-with-refresh codepath we end up calling
		// UpgradeResourceState twice, because we do so once during refreshing
		// (as part making a normal plan) and then again during the plan-destroy
		// walk. The second call receives the result of the earlier refresh,
		// so we need to tolerate both "before" and "current" as possible
		// inputs here.
		if !bytes.Contains(req.RawStateJSON, []byte("before")) {
			if !bytes.Contains(req.RawStateJSON, []byte("current")) {
				t.Fatalf("UpgradeResourceState request doesn't contain the 'before' object or the 'current' object\n%s", req.RawStateJSON)
			}
		}

		// We'll put something different in "arg" as part of upgrading, just
		// so that we can verify below that PrevRunState contains the upgraded
		// (but NOT refreshed) version of the object.
		resp.UpgradedState = cty.ObjectVal(map[string]cty.Value{
			"arg": cty.StringVal("upgraded"),
		})
		return resp
	}

	addr := mustResourceInstanceAddr("test_object.a")
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addr, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{"arg":"before"}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode:        plans.DestroyMode,
		SkipRefresh: false, // the default
	})
	assertNoErrors(t, diags)

	if !upgradeResourceStateCalled {
		t.Errorf("Provider's UpgradeResourceState wasn't called; should've been")
	}
	if !readResourceCalled {
		t.Errorf("Provider's ReadResource wasn't called; should've been")
	}

	if plan.PriorState == nil {
		t.Fatal("missing plan state")
	}

	for _, c := range plan.Changes.Resources {
		if c.Action != plans.Delete {
			t.Errorf("unexpected %s change for %s", c.Action, c.Addr)
		}
	}

	if instState := plan.PrevRunState.ResourceInstance(addr); instState == nil {
		t.Errorf("%s has no previous run state at all after plan", addr)
	} else {
		if instState.Current == nil {
			t.Errorf("%s has no current object in the previous run state", addr)
		} else if got, want := instState.Current.AttrsJSON, `"upgraded"`; !bytes.Contains(got, []byte(want)) {
			t.Errorf("%s has wrong previous run state after plan\ngot:\n%s\n\nwant substring: %s", addr, got, want)
		}
	}
	if instState := plan.PriorState.ResourceInstance(addr); instState == nil {
		t.Errorf("%s has no prior state at all after plan", addr)
	} else {
		if instState.Current == nil {
			t.Errorf("%s has no current object in the prior state", addr)
		} else if got, want := instState.Current.AttrsJSON, `"current"`; !bytes.Contains(got, []byte(want)) {
			t.Errorf("%s has wrong prior state after plan\ngot:\n%s\n\nwant substring: %s", addr, got, want)
		}
	}
}

func TestContext2Plan_destroyWithRefresh_skipImport(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
}

resource "test_object" "imported" {
}

import {
  to   = test_object.imported
  id   = "123"
}
`,
	})

	p := simpleMockProvider()

	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.ObjectVal(map[string]cty.Value{
			"test_string": cty.StringVal("foo"),
		}),
	}

	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_object",
				State: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			},
		},
	}

	expectedDestroyedAddr := mustResourceInstanceAddr("test_object.a")
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(expectedDestroyedAddr, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{"arg":"before"}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode:        plans.DestroyMode,
		SkipRefresh: false, // the default
	})
	assertNoErrors(t, diags)

	if plan.PriorState == nil {
		t.Fatal("missing plan state")
	}

	if len(plan.Changes.Resources) > 1 {
		t.Fatal("only a single resource should be changed in the plan")
	}

	changedResource := plan.Changes.Resources[0]

	if changedResource.Action != plans.Delete {
		t.Errorf("unexpected %s change for %s", changedResource.Action, changedResource.Addr)
	}

	if !changedResource.Addr.Equal(expectedDestroyedAddr) {
		t.Errorf("unexpected change for resource %s instead of %s", changedResource.Addr, expectedDestroyedAddr)
	}
}

func TestContext2Plan_destroySkipRefresh(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
}
`,
	})

	p := simpleMockProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{Block: simpleTestSchema()},
		ResourceTypes: map[string]providers.Schema{
			"test_object": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"arg": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}
	p.ReadResourceFn = func(req providers.ReadResourceRequest) (resp providers.ReadResourceResponse) {
		t.Helper()
		t.Errorf("unexpected call to ReadResource")
		resp.NewState = req.PriorState
		return resp
	}
	p.UpgradeResourceStateFn = func(req providers.UpgradeResourceStateRequest) (resp providers.UpgradeResourceStateResponse) {
		t.Logf("UpgradeResourceState %s", req.RawStateJSON)
		// We should've been given the prior state JSON as our input to upgrade.
		if !bytes.Contains(req.RawStateJSON, []byte("before")) {
			t.Fatalf("UpgradeResourceState request doesn't contain the 'before' object\n%s", req.RawStateJSON)
		}

		// We'll put something different in "arg" as part of upgrading, just
		// so that we can verify below that PrevRunState contains the upgraded
		// (but NOT refreshed) version of the object.
		resp.UpgradedState = cty.ObjectVal(map[string]cty.Value{
			"arg": cty.StringVal("upgraded"),
		})
		return resp
	}

	addr := mustResourceInstanceAddr("test_object.a")
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addr, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{"arg":"before"}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode:        plans.DestroyMode,
		SkipRefresh: true,
	})
	assertNoErrors(t, diags)

	if !p.UpgradeResourceStateCalled {
		t.Errorf("Provider's UpgradeResourceState wasn't called; should've been")
	}
	if p.ReadResourceCalled {
		t.Errorf("Provider's ReadResource was called; shouldn't have been")
	}

	if plan.PriorState == nil {
		t.Fatal("missing plan state")
	}

	for _, c := range plan.Changes.Resources {
		if c.Action != plans.Delete {
			t.Errorf("unexpected %s change for %s", c.Action, c.Addr)
		}
	}

	if instState := plan.PrevRunState.ResourceInstance(addr); instState == nil {
		t.Errorf("%s has no previous run state at all after plan", addr)
	} else {
		if instState.Current == nil {
			t.Errorf("%s has no current object in the previous run state", addr)
		} else if got, want := instState.Current.AttrsJSON, `"upgraded"`; !bytes.Contains(got, []byte(want)) {
			t.Errorf("%s has wrong previous run state after plan\ngot:\n%s\n\nwant substring: %s", addr, got, want)
		}
	}
	if instState := plan.PriorState.ResourceInstance(addr); instState == nil {
		t.Errorf("%s has no prior state at all after plan", addr)
	} else {
		if instState.Current == nil {
			t.Errorf("%s has no current object in the prior state", addr)
		} else if got, want := instState.Current.AttrsJSON, `"upgraded"`; !bytes.Contains(got, []byte(want)) {
			// NOTE: The prior state should still have been _upgraded_, even
			// though we skipped running refresh after upgrading it.
			t.Errorf("%s has wrong prior state after plan\ngot:\n%s\n\nwant substring: %s", addr, got, want)
		}
	}
}

func TestContext2Plan_unmarkingSensitiveAttributeForOutput(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_resource" "foo" {
}

output "result" {
  value = nonsensitive(test_resource.foo.sensitive_attr)
}
`,
	})

	p := new(MockProvider)
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"test_resource": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"sensitive_attr": {
						Type:      cty.String,
						Computed:  true,
						Sensitive: true,
					},
				},
			},
		},
	})

	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
		return providers.PlanResourceChangeResponse{
			PlannedState: cty.UnknownVal(cty.Object(map[string]cty.Type{
				"id":             cty.String,
				"sensitive_attr": cty.String,
			})),
		}
	}

	state := states.NewState()

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, DefaultPlanOpts)
	assertNoErrors(t, diags)

	for _, res := range plan.Changes.Resources {
		if res.Action != plans.Create {
			t.Fatalf("expected create, got: %q %s", res.Addr, res.Action)
		}
	}
}

func TestContext2Plan_destroyNoProviderConfig(t *testing.T) {
	// providers do not need to be configured during a destroy plan
	p := simpleMockProvider()
	p.ValidateProviderConfigFn = func(req providers.ValidateProviderConfigRequest) (resp providers.ValidateProviderConfigResponse) {
		v := req.Config.GetAttr("test_string")
		if v.IsNull() || !v.IsKnown() || v.AsString() != "ok" {
			resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("invalid provider configuration: %#v", req.Config))
		}
		return resp
	}

	m := testModuleInline(t, map[string]string{
		"main.tf": `
locals {
  value = "ok"
}

provider "test" {
  test_string = local.value
}
`,
	})

	addr := mustResourceInstanceAddr("test_object.a")
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addr, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{"test_string":"foo"}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.DestroyMode,
	})
	assertNoErrors(t, diags)
}

func TestContext2Plan_movedResourceBasic(t *testing.T) {
	addrA := mustResourceInstanceAddr("test_object.a")
	addrB := mustResourceInstanceAddr("test_object.b")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			resource "test_object" "b" {
			}

			moved {
				from = test_object.a
				to   = test_object.b
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		// The prior state tracks test_object.a, which we should treat as
		// test_object.b because of the "moved" block in the config.
		s.SetResourceInstanceCurrent(addrA, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addrA,
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	t.Run(addrA.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addrA)
		if instPlan != nil {
			t.Fatalf("unexpected plan for %s; should've moved to %s", addrA, addrB)
		}
	})
	t.Run(addrB.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addrB)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addrB)
		}

		if got, want := instPlan.Addr, addrB; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addrA; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.NoOp; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})
}

func constructProviderSchemaForTesting(attrs map[string]*configschema.Attribute) providers.Schema {
	return providers.Schema{
		Block: &configschema.Block{Attributes: attrs},
	}
}

func TestContext2Plan_movedResourceToDifferentType(t *testing.T) {
	type test struct {
		name          string
		mockProvider  *MockProvider
		providerAddr1 *addrs.AbsProviderConfig
		providerAddr2 *addrs.AbsProviderConfig
		oldSchema     providers.Schema
		newSchema     providers.Schema
		oldType       string
		newType       string
		config        map[string]string
		attrsJSON     []byte
		wantOp        plans.Action
		wantErrStr    string
	}

	tests := []test{
		{
			name: "basic case",
			oldSchema: constructProviderSchemaForTesting(map[string]*configschema.Attribute{
				"test_number": {
					Type:     cty.Number,
					Optional: true,
				},
			}),
			newSchema: constructProviderSchemaForTesting(map[string]*configschema.Attribute{
				"test_number": {
					Type:     cty.Number,
					Optional: true,
				},
			}),
			oldType: "test_object0",
			newType: "test_object",
			config: map[string]string{
				"main.tf": `
					resource "test_object" "new" {

					}
					moved {
						from = test_object0.old
						to   = test_object.new
					}
				`,
			},
			attrsJSON: []byte(`{}`),
			wantOp:    plans.NoOp,
		},
		{
			name: "attributes unchanged",
			oldSchema: constructProviderSchemaForTesting(map[string]*configschema.Attribute{
				"test_number": {
					Type:     cty.Number,
					Required: true,
				},
			}),
			newSchema: constructProviderSchemaForTesting(map[string]*configschema.Attribute{
				"test_number": {
					Type:     cty.Number,
					Required: true,
				},
			}),
			oldType: "test_object0",
			newType: "test_object",
			config: map[string]string{
				"main.tf": `
					resource "test_object" "new" {
						test_number = 1
					}
					moved {
						from = test_object0.old
						to   = test_object.new
					}
				`,
			},
			attrsJSON: []byte(`{"test_number": 1}`),
			wantOp:    plans.NoOp,
		},
		{
			name: "attribute changed",
			oldSchema: constructProviderSchemaForTesting(map[string]*configschema.Attribute{
				"test_number": {
					Type:     cty.Number,
					Required: true,
				},
			}),
			newSchema: constructProviderSchemaForTesting(map[string]*configschema.Attribute{
				"test_number": {
					Type:     cty.Number,
					Required: true,
				},
			}),
			oldType: "test_object0",
			newType: "test_object",
			config: map[string]string{
				"main.tf": `
					resource "test_object" "new" {
						test_number = 1
					}
					moved {
						from = test_object0.old
						to   = test_object.new
					}
				`,
			},
			attrsJSON: []byte(`{"test_number": 2}`),
			wantOp:    plans.Update,
		},
		{
			name: "attribute removed provider doesn't remove the attribute",
			oldSchema: constructProviderSchemaForTesting(map[string]*configschema.Attribute{
				"test_number": {
					Type:     cty.Number,
					Required: true,
				},
				"test_string": {
					Type:     cty.String,
					Required: true,
				},
			}),
			newSchema: constructProviderSchemaForTesting(map[string]*configschema.Attribute{
				"test_number": {
					Type:     cty.Number,
					Required: true,
				},
			}),
			oldType: "test_object0",
			newType: "test_object",
			config: map[string]string{
				"main.tf": `
					resource "test_object" "new" {
						test_number = 1
					}
					moved {
						from = test_object0.old
						to   = test_object.new
					}
				`,
			},
			attrsJSON:  []byte(`{"test_number": 1, "test_string": "foo"}`),
			wantErrStr: `unsupported attribute "test_string"`,
		},
		{
			name:    "provider failure",
			oldType: "test_object0",
			newType: "test_object",
			config: map[string]string{
				"main.tf": `
					resource "test_object" "new" {}
					moved {
						from = test_object0.old
						to   = test_object.new
					}
				`,
			},
			attrsJSON: []byte(`{}`),
			mockProvider: &MockProvider{
				GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
					ResourceTypes: map[string]providers.Schema{
						"test_object": constructProviderSchemaForTesting(map[string]*configschema.Attribute{
							"test_number": {
								Type:     cty.Number,
								Optional: true,
							},
						}),
						"test_object0": constructProviderSchemaForTesting(map[string]*configschema.Attribute{
							"test_number": {
								Type:     cty.Number,
								Optional: true,
							},
						}),
					},
				},
				MoveResourceStateResponse: &providers.MoveResourceStateResponse{
					Diagnostics: tfdiags.Diagnostics{tfdiags.Sourceless(tfdiags.Error, "move failed", "")},
				},
			},
			wantErrStr: "move failed",
		},
		{
			name: "different provider with same schema",
			oldSchema: constructProviderSchemaForTesting(map[string]*configschema.Attribute{
				"test_number": {
					Type:     cty.Number,
					Required: true,
				},
			}),
			newSchema: constructProviderSchemaForTesting(map[string]*configschema.Attribute{
				"test_number": {
					Type:     cty.Number,
					Required: true,
				},
			}),
			oldType: "provider1_object",
			newType: "provider2_object",
			config: map[string]string{
				"main.tf": `
					resource "provider2_object" "new" {
						test_number = 1
					}
					moved {
						from = provider1_object.old
						to   = provider2_object.new
					}
				`,
			},
			attrsJSON: []byte(`{"test_number": 1}`),
			wantOp:    plans.NoOp,
			providerAddr1: &addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("provider1"),
				Module:   addrs.RootModule,
			},
			providerAddr2: &addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("provider2"),
			},
			mockProvider: &MockProvider{
				GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
					ResourceTypes: map[string]providers.Schema{
						"provider2_object": constructProviderSchemaForTesting(map[string]*configschema.Attribute{
							"test_number": {
								Type:     cty.Number,
								Required: true,
							},
						}),
						"provider1_object": constructProviderSchemaForTesting(map[string]*configschema.Attribute{
							"test_number": {
								Type:     cty.Number,
								Required: true,
							},
						}),
					},
				},
				MoveResourceStateResponse: &providers.MoveResourceStateResponse{
					TargetState: cty.ObjectVal(map[string]cty.Value{
						"test_number": cty.NumberIntVal(1),
					}),
				},
			},
		},
		{
			name: "different provider with schema change",
			oldSchema: constructProviderSchemaForTesting(map[string]*configschema.Attribute{
				"test_number": {
					Type:     cty.Number,
					Required: true,
				},
			}),
			newSchema: constructProviderSchemaForTesting(map[string]*configschema.Attribute{
				"test_string": {
					Type:     cty.String,
					Required: true,
				},
			}),
			oldType: "provider1_object",
			newType: "provider2_object",
			config: map[string]string{
				"main.tf": `
					resource "provider2_object" "new" {
						test_string = "2"
					}
					moved {
						from = provider1_object.old
						to   = provider2_object.new
					}
				`,
			},
			attrsJSON: []byte(`{"test_number": 1}`),
			wantOp:    plans.Update,
			providerAddr1: &addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("provider1"),
				Module:   addrs.RootModule,
			},
			providerAddr2: &addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("provider2"),
			},
			mockProvider: &MockProvider{
				GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
					ResourceTypes: map[string]providers.Schema{
						"provider2_object": constructProviderSchemaForTesting(map[string]*configschema.Attribute{
							"test_string": {
								Type:     cty.String,
								Required: true,
							},
						}),
						"provider1_object": constructProviderSchemaForTesting(map[string]*configschema.Attribute{
							"test_number": {
								Type:     cty.Number,
								Required: true,
							},
						}),
					},
				},
				MoveResourceStateResponse: &providers.MoveResourceStateResponse{
					TargetState: cty.ObjectVal(map[string]cty.Value{
						"test_string": cty.StringVal("1"),
					}),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldAddr := mustResourceInstanceAddr(tt.oldType + ".old")
			newAddr := mustResourceInstanceAddr(tt.newType + ".new")
			m := testModuleInline(t, tt.config)

			providerAddr := addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			}
			if tt.providerAddr1 != nil {
				providerAddr = *tt.providerAddr1
			}

			state := states.BuildState(
				func(s *states.SyncState) {
					s.SetResourceProvider(oldAddr.ContainingResource(), providerAddr)
					s.SetResourceInstanceCurrent(oldAddr, &states.ResourceInstanceObjectSrc{
						Status:    states.ObjectReady,
						AttrsJSON: tt.attrsJSON,
					}, providerAddr, addrs.NoKey)
				})

			provider := &MockProvider{
				// New provider should know about both resource types
				GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
					ResourceTypes: map[string]providers.Schema{
						tt.newType: tt.newSchema,
						tt.oldType: tt.oldSchema,
					},
				},
			}
			if tt.mockProvider != nil {
				provider = tt.mockProvider
			}

			providerFactory := map[addrs.Provider]providers.Factory{
				providerAddr.Provider: testProviderFuncFixed(provider),
			}
			if tt.providerAddr2 != nil {
				providerFactory[tt.providerAddr2.Provider] = testProviderFuncFixed(provider)
			}

			ctx := testContext2(t, &ContextOpts{
				Providers: providerFactory,
			})

			plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
				Mode: plans.NormalMode,
			})

			if tt.wantErrStr != "" {
				if !diags.HasErrors() {
					t.Fatalf("expected errors, got none")
				}
				if got, want := diags.Err().Error(), tt.wantErrStr; !strings.Contains(got, want) {
					t.Fatalf("wrong error\ngot: %s\nwant substring: %s", got, want)
				}
				return
			}

			assertNoErrors(t, diags)

			// Check that no plan was created for the old resource
			instPlan := plan.Changes.ResourceInstance(oldAddr)
			if instPlan != nil {
				t.Fatalf("unexpected plan for %s; should've moved to %s", oldAddr, newAddr)
			}

			// Check that a plan was created for the new resource
			instPlan = plan.Changes.ResourceInstance(newAddr)
			if instPlan == nil {
				t.Fatalf("no plan for %s at all", newAddr)
			}

			if got, want := instPlan.Addr, newAddr; !got.Equal(want) {
				t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.PrevRunAddr, oldAddr; !got.Equal(want) {
				t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.Action, tt.wantOp; got != want {
				t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
				t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
			}
		})
	}
}

func TestContext2Plan_movedResourceCollision(t *testing.T) {
	addrNoKey := mustResourceInstanceAddr("test_object.a")
	addrZeroKey := mustResourceInstanceAddr("test_object.a[0]")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			resource "test_object" "a" {
				# No "count" set, so test_object.a[0] will want
				# to implicitly move to test_object.a, but will get
				# blocked by the existing object at that address.
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addrNoKey, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
		s.SetResourceInstanceCurrent(addrZeroKey, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	// We should have a warning, though! We'll lightly abuse the "for RPC"
	// feature of diagnostics to get some more-readily-comparable diagnostic
	// values.
	gotDiags := diags.ForRPC()
	wantDiags := tfdiags.Diagnostics{
		tfdiags.Sourceless(
			tfdiags.Warning,
			"Unresolved resource instance address changes",
			`OpenTofu tried to adjust resource instance addresses in the prior state based on change information recorded in the configuration, but some adjustments did not succeed due to existing objects already at the intended addresses:
  - test_object.a[0] could not move to test_object.a

OpenTofu has planned to destroy these objects. If OpenTofu's proposed changes aren't appropriate, you must first resolve the conflicts using the "tofu state" subcommands and then create a new plan.`,
		),
	}.ForRPC()
	if diff := cmp.Diff(wantDiags, gotDiags); diff != "" {
		t.Errorf("wrong diagnostics\n%s", diff)
	}

	t.Run(addrNoKey.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addrNoKey)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addrNoKey)
		}

		if got, want := instPlan.Addr, addrNoKey; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addrNoKey; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.NoOp; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run(addrZeroKey.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addrZeroKey)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addrZeroKey)
		}

		if got, want := instPlan.Addr, addrZeroKey; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addrZeroKey; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.Delete; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceDeleteBecauseWrongRepetition; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})
}

func TestContext2Plan_movedResourceCollisionDestroy(t *testing.T) {
	// This is like TestContext2Plan_movedResourceCollision but intended to
	// ensure we still produce the expected warning (and produce it only once)
	// when we're creating a destroy plan, rather than a normal plan.
	// (This case is interesting at the time of writing because we happen to
	// use a normal plan as a trick to refresh before creating a destroy plan.
	// This test will probably become uninteresting if a future change to
	// the destroy-time planning behavior handles refreshing in a different
	// way, which avoids this pre-processing step of running a normal plan
	// first.)

	addrNoKey := mustResourceInstanceAddr("test_object.a")
	addrZeroKey := mustResourceInstanceAddr("test_object.a[0]")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			resource "test_object" "a" {
				# No "count" set, so test_object.a[0] will want
				# to implicitly move to test_object.a, but will get
				# blocked by the existing object at that address.
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addrNoKey, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
		s.SetResourceInstanceCurrent(addrZeroKey, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.DestroyMode,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	// We should have a warning, though! We'll lightly abuse the "for RPC"
	// feature of diagnostics to get some more-readily-comparable diagnostic
	// values.
	gotDiags := diags.ForRPC()
	wantDiags := tfdiags.Diagnostics{
		tfdiags.Sourceless(
			tfdiags.Warning,
			"Unresolved resource instance address changes",
			// NOTE: This message is _lightly_ confusing in the destroy case,
			// because it says "OpenTofu has planned to destroy these objects"
			// but this is a plan to destroy all objects, anyway. We expect the
			// conflict situation to be pretty rare though, and even rarer in
			// a "tofu destroy", so we'll just live with that for now
			// unless we see evidence that lots of folks are being confused by
			// it in practice.
			`OpenTofu tried to adjust resource instance addresses in the prior state based on change information recorded in the configuration, but some adjustments did not succeed due to existing objects already at the intended addresses:
  - test_object.a[0] could not move to test_object.a

OpenTofu has planned to destroy these objects. If OpenTofu's proposed changes aren't appropriate, you must first resolve the conflicts using the "tofu state" subcommands and then create a new plan.`,
		),
	}.ForRPC()
	if diff := cmp.Diff(wantDiags, gotDiags); diff != "" {
		// If we get here with a diff that makes it seem like the above warning
		// is being reported twice, the likely cause is not correctly handling
		// the warnings from the hidden normal plan we run as part of preparing
		// for a destroy plan, unless that strategy has changed in the meantime
		// since we originally wrote this test.
		t.Errorf("wrong diagnostics\n%s", diff)
	}

	t.Run(addrNoKey.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addrNoKey)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addrNoKey)
		}

		if got, want := instPlan.Addr, addrNoKey; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addrNoKey; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.Delete; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run(addrZeroKey.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addrZeroKey)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addrZeroKey)
		}

		if got, want := instPlan.Addr, addrZeroKey; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addrZeroKey; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.Delete; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})
}

func TestContext2Plan_movedResourceUntargeted(t *testing.T) {
	addrA := mustResourceInstanceAddr("test_object.a")
	addrB := mustResourceInstanceAddr("test_object.b")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			resource "test_object" "b" {
			}

			moved {
				from = test_object.a
				to   = test_object.b
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		// The prior state tracks test_object.a, which we should treat as
		// test_object.b because of the "moved" block in the config.
		s.SetResourceInstanceCurrent(addrA, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	t.Run("without targeting instance A", func(t *testing.T) {
		_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.NormalMode,
			Targets: []addrs.Targetable{
				// NOTE: addrA isn't included here, but it's pending move to addrB
				// and so this plan request is invalid.
				addrB,
			},
		})
		diags.Sort()

		// We're semi-abusing "ForRPC" here just to get diagnostics that are
		// more easily comparable than the various different diagnostics types
		// tfdiags uses internally. The RPC-friendly diagnostics are also
		// comparison-friendly, by discarding all of the dynamic type information.
		gotDiags := diags.ForRPC()
		wantDiags := tfdiags.Diagnostics{
			tfdiags.Sourceless(
				tfdiags.Warning,
				"Resource targeting is in effect",
				`You are creating a plan with either the -target option or the -exclude option, which means that the result of this plan may not represent all of the changes requested by the current configuration.

The -target and -exclude options are not for routine use, and are provided only for exceptional situations such as recovering from errors or mistakes, or when OpenTofu specifically suggests to use it as part of an error message.`,
			),
			tfdiags.Sourceless(
				tfdiags.Error,
				"Moved resource instances excluded by targeting",
				`Resource instances in your current state have moved to new addresses in the latest configuration. OpenTofu must include those resource instances while planning in order to ensure a correct result, but your -target=... options do not fully cover all of those resource instances.

To create a valid plan, either remove your -target=... options altogether or add the following additional target options:
  -target="test_object.a"

Note that adding these options may include further additional resource instances in your plan, in order to respect object dependencies.`,
			),
		}.ForRPC()

		if diff := cmp.Diff(wantDiags, gotDiags); diff != "" {
			t.Errorf("wrong diagnostics\n%s", diff)
		}
	})
	t.Run("without targeting instance B", func(t *testing.T) {
		_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.NormalMode,
			Targets: []addrs.Targetable{
				addrA,
				// NOTE: addrB isn't included here, but it's pending move from
				// addrA and so this plan request is invalid.
			},
		})
		diags.Sort()

		// We're semi-abusing "ForRPC" here just to get diagnostics that are
		// more easily comparable than the various different diagnostics types
		// tfdiags uses internally. The RPC-friendly diagnostics are also
		// comparison-friendly, by discarding all of the dynamic type information.
		gotDiags := diags.ForRPC()
		wantDiags := tfdiags.Diagnostics{
			tfdiags.Sourceless(
				tfdiags.Warning,
				"Resource targeting is in effect",
				`You are creating a plan with either the -target option or the -exclude option, which means that the result of this plan may not represent all of the changes requested by the current configuration.

The -target and -exclude options are not for routine use, and are provided only for exceptional situations such as recovering from errors or mistakes, or when OpenTofu specifically suggests to use it as part of an error message.`,
			),
			tfdiags.Sourceless(
				tfdiags.Error,
				"Moved resource instances excluded by targeting",
				`Resource instances in your current state have moved to new addresses in the latest configuration. OpenTofu must include those resource instances while planning in order to ensure a correct result, but your -target=... options do not fully cover all of those resource instances.

To create a valid plan, either remove your -target=... options altogether or add the following additional target options:
  -target="test_object.b"

Note that adding these options may include further additional resource instances in your plan, in order to respect object dependencies.`,
			),
		}.ForRPC()

		if diff := cmp.Diff(wantDiags, gotDiags); diff != "" {
			t.Errorf("wrong diagnostics\n%s", diff)
		}
	})
	t.Run("without targeting either instance", func(t *testing.T) {
		_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.NormalMode,
			Targets: []addrs.Targetable{
				mustResourceInstanceAddr("test_object.unrelated"),
				// NOTE: neither addrA nor addrB are included here, but there's
				// a pending move between them and so this is invalid.
			},
		})
		diags.Sort()

		// We're semi-abusing "ForRPC" here just to get diagnostics that are
		// more easily comparable than the various different diagnostics types
		// tfdiags uses internally. The RPC-friendly diagnostics are also
		// comparison-friendly, by discarding all of the dynamic type information.
		gotDiags := diags.ForRPC()
		wantDiags := tfdiags.Diagnostics{
			tfdiags.Sourceless(
				tfdiags.Warning,
				"Resource targeting is in effect",
				`You are creating a plan with either the -target option or the -exclude option, which means that the result of this plan may not represent all of the changes requested by the current configuration.

The -target and -exclude options are not for routine use, and are provided only for exceptional situations such as recovering from errors or mistakes, or when OpenTofu specifically suggests to use it as part of an error message.`,
			),
			tfdiags.Sourceless(
				tfdiags.Error,
				"Moved resource instances excluded by targeting",
				`Resource instances in your current state have moved to new addresses in the latest configuration. OpenTofu must include those resource instances while planning in order to ensure a correct result, but your -target=... options do not fully cover all of those resource instances.

To create a valid plan, either remove your -target=... options altogether or add the following additional target options:
  -target="test_object.a"
  -target="test_object.b"

Note that adding these options may include further additional resource instances in your plan, in order to respect object dependencies.`,
			),
		}.ForRPC()

		if diff := cmp.Diff(wantDiags, gotDiags); diff != "" {
			t.Errorf("wrong diagnostics\n%s", diff)
		}
	})
	t.Run("with both addresses in the target set", func(t *testing.T) {
		// The error messages in the other subtests above suggest adding
		// addresses to the set of targets. This additional test makes sure that
		// following that advice actually leads to a valid result.

		_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.NormalMode,
			Targets: []addrs.Targetable{
				// This time we're including both addresses in the target,
				// to get the same effect an end-user would get if following
				// the advice in our error message in the other subtests.
				addrA,
				addrB,
			},
		})
		diags.Sort()

		// We're semi-abusing "ForRPC" here just to get diagnostics that are
		// more easily comparable than the various different diagnostics types
		// tfdiags uses internally. The RPC-friendly diagnostics are also
		// comparison-friendly, by discarding all of the dynamic type information.
		gotDiags := diags.ForRPC()
		wantDiags := tfdiags.Diagnostics{
			// Still get the warning about the -target option...
			tfdiags.Sourceless(
				tfdiags.Warning,
				"Resource targeting is in effect",
				`You are creating a plan with either the -target option or the -exclude option, which means that the result of this plan may not represent all of the changes requested by the current configuration.

The -target and -exclude options are not for routine use, and are provided only for exceptional situations such as recovering from errors or mistakes, or when OpenTofu specifically suggests to use it as part of an error message.`,
			),
			// ...but now we have no error about test_object.a
		}.ForRPC()

		if diff := cmp.Diff(wantDiags, gotDiags); diff != "" {
			t.Errorf("wrong diagnostics\n%s", diff)
		}
	})
	t.Run("excluding instance A", func(t *testing.T) {
		_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.NormalMode,
			Excludes: []addrs.Targetable{
				// NOTE: addrA isn't excluded, and it's pending move to addrB
				// and so this plan request is invalid.
				addrA,
			},
		})
		diags.Sort()

		// We're semi-abusing "ForRPC" here just to get diagnostics that are
		// more easily comparable than the various different diagnostics types
		// tfdiags uses internally. The RPC-friendly diagnostics are also
		// comparison-friendly, by discarding all of the dynamic type information.
		gotDiags := diags.ForRPC()
		wantDiags := tfdiags.Diagnostics{
			tfdiags.Sourceless(
				tfdiags.Warning,
				"Resource targeting is in effect",
				`You are creating a plan with either the -target option or the -exclude option, which means that the result of this plan may not represent all of the changes requested by the current configuration.

The -target and -exclude options are not for routine use, and are provided only for exceptional situations such as recovering from errors or mistakes, or when OpenTofu specifically suggests to use it as part of an error message.`,
			),
			tfdiags.Sourceless(
				tfdiags.Error,
				"Moved resource instances excluded by targeting",
				`Resource instances in your current state have moved to new addresses in the latest configuration. OpenTofu must include those resource instances while planning in order to ensure a correct result, but your -exclude=... options exclude some of those resource instances.

To create a valid plan, either remove your -exclude=... options altogether or just specifically remove the following options:
  -exclude="test_object.a"

Note that removing these options may include further additional resource instances in your plan, in order to respect object dependencies.`,
			),
		}.ForRPC()

		if diff := cmp.Diff(wantDiags, gotDiags); diff != "" {
			t.Errorf("wrong diagnostics\n%s", diff)
		}
	})
	t.Run("excluding instance B", func(t *testing.T) {
		_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.NormalMode,
			Excludes: []addrs.Targetable{
				addrB,
				// NOTE: addrB is excluded here, and it's pending move from
				// addrA and so this plan request is invalid.
			},
		})
		diags.Sort()

		// We're semi-abusing "ForRPC" here just to get diagnostics that are
		// more easily comparable than the various different diagnostics types
		// tfdiags uses internally. The RPC-friendly diagnostics are also
		// comparison-friendly, by discarding all of the dynamic type information.
		gotDiags := diags.ForRPC()
		wantDiags := tfdiags.Diagnostics{
			tfdiags.Sourceless(
				tfdiags.Warning,
				"Resource targeting is in effect",
				`You are creating a plan with either the -target option or the -exclude option, which means that the result of this plan may not represent all of the changes requested by the current configuration.

The -target and -exclude options are not for routine use, and are provided only for exceptional situations such as recovering from errors or mistakes, or when OpenTofu specifically suggests to use it as part of an error message.`,
			),
			tfdiags.Sourceless(
				tfdiags.Error,
				"Moved resource instances excluded by targeting",
				`Resource instances in your current state have moved to new addresses in the latest configuration. OpenTofu must include those resource instances while planning in order to ensure a correct result, but your -exclude=... options exclude some of those resource instances.

To create a valid plan, either remove your -exclude=... options altogether or just specifically remove the following options:
  -exclude="test_object.b"

Note that removing these options may include further additional resource instances in your plan, in order to respect object dependencies.`,
			),
		}.ForRPC()

		if diff := cmp.Diff(wantDiags, gotDiags); diff != "" {
			t.Errorf("wrong diagnostics\n%s", diff)
		}
	})
	t.Run("excluding both addresses", func(t *testing.T) {
		_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.NormalMode,
			Excludes: []addrs.Targetable{
				// NOTE: both addrA nor addrB are excluded here, but there's
				// a pending move between them and so this is invalid.
				addrA,
				addrB,
			},
		})
		diags.Sort()

		// We're semi-abusing "ForRPC" here just to get diagnostics that are
		// more easily comparable than the various different diagnostics types
		// tfdiags uses internally. The RPC-friendly diagnostics are also
		// comparison-friendly, by discarding all of the dynamic type information.
		gotDiags := diags.ForRPC()
		wantDiags := tfdiags.Diagnostics{
			tfdiags.Sourceless(
				tfdiags.Warning,
				"Resource targeting is in effect",
				`You are creating a plan with either the -target option or the -exclude option, which means that the result of this plan may not represent all of the changes requested by the current configuration.

The -target and -exclude options are not for routine use, and are provided only for exceptional situations such as recovering from errors or mistakes, or when OpenTofu specifically suggests to use it as part of an error message.`,
			),
			tfdiags.Sourceless(
				tfdiags.Error,
				"Moved resource instances excluded by targeting",
				`Resource instances in your current state have moved to new addresses in the latest configuration. OpenTofu must include those resource instances while planning in order to ensure a correct result, but your -exclude=... options exclude some of those resource instances.

To create a valid plan, either remove your -exclude=... options altogether or just specifically remove the following options:
  -exclude="test_object.a"
  -exclude="test_object.b"

Note that removing these options may include further additional resource instances in your plan, in order to respect object dependencies.`,
			),
		}.ForRPC()

		if diff := cmp.Diff(wantDiags, gotDiags); diff != "" {
			t.Errorf("wrong diagnostics\n%s", diff)
		}
	})
	t.Run("without excluding either instance", func(t *testing.T) {
		// The error messages in the other subtests above suggest removing
		// addresses to the set of excludes. This additional test makes sure that
		// following that advice actually leads to a valid result.
		_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.NormalMode,
			Excludes: []addrs.Targetable{
				mustResourceInstanceAddr("test_object.unrelated"),
				// This time we're excluding neither address,
				// to get the same effect an end-user would get if following
				// the advice in our error message in the other subtests.
			},
		})
		diags.Sort()

		// We're semi-abusing "ForRPC" here just to get diagnostics that are
		// more easily comparable than the various different diagnostics types
		// tfdiags uses internally. The RPC-friendly diagnostics are also
		// comparison-friendly, by discarding all of the dynamic type information.
		gotDiags := diags.ForRPC()
		wantDiags := tfdiags.Diagnostics{
			// Still get the warning about the -target option...
			tfdiags.Sourceless(
				tfdiags.Warning,
				"Resource targeting is in effect",
				`You are creating a plan with either the -target option or the -exclude option, which means that the result of this plan may not represent all of the changes requested by the current configuration.

The -target and -exclude options are not for routine use, and are provided only for exceptional situations such as recovering from errors or mistakes, or when OpenTofu specifically suggests to use it as part of an error message.`,
			),
			// ...but now we have no error about test_object.a
		}.ForRPC()

		if diff := cmp.Diff(wantDiags, gotDiags); diff != "" {
			t.Errorf("wrong diagnostics\n%s", diff)
		}
	})
}

func TestContext2Plan_untargetedResourceSchemaChange(t *testing.T) {
	// an untargeted resource which requires a schema migration should not
	// block planning due external changes in the plan.
	addrA := mustResourceInstanceAddr("test_object.a")
	addrB := mustResourceInstanceAddr("test_object.b")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
}
resource "test_object" "b" {
}`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addrA, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
		s.SetResourceInstanceCurrent(addrB, &states.ResourceInstanceObjectSrc{
			// old_list is no longer in the schema
			AttrsJSON: []byte(`{"old_list":["used to be","a list here"]}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()

	// external changes trigger a "drift report", but because test_object.b was
	// not targeted, the state was not fixed to match the schema and cannot be
	// decoded for the report.
	p.ReadResourceFn = func(req providers.ReadResourceRequest) (resp providers.ReadResourceResponse) {
		obj := req.PriorState.AsValueMap()
		// test_number changed externally
		obj["test_number"] = cty.NumberIntVal(1)
		resp.NewState = cty.ObjectVal(obj)
		return resp
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		Targets: []addrs.Targetable{
			addrA,
		},
	})
	//
	assertNoErrors(t, diags)

	if len(plan.Changes.Resources) != 1 {
		t.Fatalf("expected 1 resource change, but got %d", len(plan.Changes.Resources))
	}

	instPlan := plan.Changes.ResourceInstance(addrA)
	if instPlan == nil {
		t.Fatalf("expected plan for %s; but got none", addrA)
	}
}

func TestContext2Plan_excludedResourceSchemaChange(t *testing.T) {
	// an excluded resource which requires a schema migration should not
	// block planning due external changes in the plan.
	addrA := mustResourceInstanceAddr("test_object.a")
	addrB := mustResourceInstanceAddr("test_object.b")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
}
resource "test_object" "b" {
}`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addrA, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
		s.SetResourceInstanceCurrent(addrB, &states.ResourceInstanceObjectSrc{
			// old_list is no longer in the schema
			AttrsJSON: []byte(`{"old_list":["used to be","a list here"]}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()

	// external changes trigger a "drift report", but because test_object.b was
	// excluded, the state was not fixed to match the schema and cannot be
	// deocded for the report.
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		var resp providers.ReadResourceResponse
		obj := req.PriorState.AsValueMap()
		// test_number changed externally
		obj["test_number"] = cty.NumberIntVal(1)
		resp.NewState = cty.ObjectVal(obj)
		return resp
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrB,
		},
	})
	//
	assertNoErrors(t, diags)

	if len(plan.Changes.Resources) != 1 {
		t.Fatalf("expected 1 resource change, but got %d", len(plan.Changes.Resources))
	}

	instPlan := plan.Changes.ResourceInstance(addrA)
	if instPlan == nil {
		t.Fatalf("expected plan for %s; but got none", addrA)
	}
}

func TestContext2Plan_movedResourceRefreshOnly(t *testing.T) {
	addrA := mustResourceInstanceAddr("test_object.a")
	addrB := mustResourceInstanceAddr("test_object.b")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			resource "test_object" "b" {
			}

			moved {
				from = test_object.a
				to   = test_object.b
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		// The prior state tracks test_object.a, which we should treat as
		// test_object.b because of the "moved" block in the config.
		s.SetResourceInstanceCurrent(addrA, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.RefreshOnlyMode,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	t.Run(addrA.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addrA)
		if instPlan != nil {
			t.Fatalf("unexpected plan for %s; should've moved to %s", addrA, addrB)
		}
	})
	t.Run(addrB.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addrB)
		if instPlan != nil {
			t.Fatalf("unexpected plan for %s", addrB)
		}
	})
	t.Run("drift", func(t *testing.T) {
		var drifted *plans.ResourceInstanceChangeSrc
		for _, dr := range plan.DriftedResources {
			if dr.Addr.Equal(addrB) {
				drifted = dr
				break
			}
		}

		if drifted == nil {
			t.Fatalf("instance %s is missing from the drifted resource changes", addrB)
		}

		if got, want := drifted.PrevRunAddr, addrA; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := drifted.Action, plans.NoOp; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
	})
}

func TestContext2Plan_refreshOnlyMode(t *testing.T) {
	addr := mustResourceInstanceAddr("test_object.a")

	// The configuration, the prior state, and the refresh result intentionally
	// have different values for "test_string" so we can observe that the
	// refresh took effect but the configuration change wasn't considered.
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			resource "test_object" "a" {
				arg = "after"
			}

			output "out" {
				value = test_object.a.arg
			}
		`,
	})
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addr, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{"arg":"before"}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{Block: simpleTestSchema()},
		ResourceTypes: map[string]providers.Schema{
			"test_object": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"arg": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		newVal, err := cty.Transform(req.PriorState, func(path cty.Path, v cty.Value) (cty.Value, error) {
			if len(path) == 1 && path[0] == (cty.GetAttrStep{Name: "arg"}) {
				return cty.StringVal("current"), nil
			}
			return v, nil
		})
		if err != nil {
			// shouldn't get here
			t.Fatalf("ReadResourceFn transform failed")
			return providers.ReadResourceResponse{}
		}
		return providers.ReadResourceResponse{
			NewState: newVal,
		}
	}
	p.UpgradeResourceStateFn = func(req providers.UpgradeResourceStateRequest) (resp providers.UpgradeResourceStateResponse) {
		// We should've been given the prior state JSON as our input to upgrade.
		if !bytes.Contains(req.RawStateJSON, []byte("before")) {
			t.Fatalf("UpgradeResourceState request doesn't contain the 'before' object\n%s", req.RawStateJSON)
		}

		// We'll put something different in "arg" as part of upgrading, just
		// so that we can verify below that PrevRunState contains the upgraded
		// (but NOT refreshed) version of the object.
		resp.UpgradedState = cty.ObjectVal(map[string]cty.Value{
			"arg": cty.StringVal("upgraded"),
		})
		return resp
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.RefreshOnlyMode,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	if !p.UpgradeResourceStateCalled {
		t.Errorf("Provider's UpgradeResourceState wasn't called; should've been")
	}
	if !p.ReadResourceCalled {
		t.Errorf("Provider's ReadResource wasn't called; should've been")
	}

	if got, want := len(plan.Changes.Resources), 0; got != want {
		t.Errorf("plan contains resource changes; want none\n%s", spew.Sdump(plan.Changes.Resources))
	}

	if instState := plan.PriorState.ResourceInstance(addr); instState == nil {
		t.Errorf("%s has no prior state at all after plan", addr)
	} else {
		if instState.Current == nil {
			t.Errorf("%s has no current object after plan", addr)
		} else if got, want := instState.Current.AttrsJSON, `"current"`; !bytes.Contains(got, []byte(want)) {
			// Should've saved the result of refreshing
			t.Errorf("%s has wrong prior state after plan\ngot:\n%s\n\nwant substring: %s", addr, got, want)
		}
	}
	if instState := plan.PrevRunState.ResourceInstance(addr); instState == nil {
		t.Errorf("%s has no previous run state at all after plan", addr)
	} else {
		if instState.Current == nil {
			t.Errorf("%s has no current object in the previous run state", addr)
		} else if got, want := instState.Current.AttrsJSON, `"upgraded"`; !bytes.Contains(got, []byte(want)) {
			// Should've saved the result of upgrading
			t.Errorf("%s has wrong previous run state after plan\ngot:\n%s\n\nwant substring: %s", addr, got, want)
		}
	}

	// The output value should also have updated. If not, it's likely that we
	// skipped updating the working state to match the refreshed state when we
	// were evaluating the resource.
	if outChangeSrc := plan.Changes.OutputValue(addrs.RootModuleInstance.OutputValue("out")); outChangeSrc == nil {
		t.Errorf("no change planned for output value 'out'")
	} else {
		outChange, err := outChangeSrc.Decode()
		if err != nil {
			t.Fatalf("failed to decode output value 'out': %s", err)
		}
		got := outChange.After
		want := cty.StringVal("current")
		if !want.RawEquals(got) {
			t.Errorf("wrong value for output value 'out'\ngot:  %#v\nwant: %#v", got, want)
		}
	}
}

func TestContext2Plan_refreshOnlyMode_deposed(t *testing.T) {
	addr := mustResourceInstanceAddr("test_object.a")
	deposedKey := states.DeposedKey("byebye")

	// The configuration, the prior state, and the refresh result intentionally
	// have different values for "test_string" so we can observe that the
	// refresh took effect but the configuration change wasn't considered.
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			resource "test_object" "a" {
				arg = "after"
			}

			output "out" {
				value = test_object.a.arg
			}
		`,
	})
	state := states.BuildState(func(s *states.SyncState) {
		// Note that we're intentionally recording a _deposed_ object here,
		// and not including a current object, so a normal (non-refresh)
		// plan would normally plan to create a new object _and_ destroy
		// the deposed one, but refresh-only mode should prevent that.
		s.SetResourceInstanceDeposed(addr, deposedKey, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{"arg":"before"}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{Block: simpleTestSchema()},
		ResourceTypes: map[string]providers.Schema{
			"test_object": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"arg": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		newVal, err := cty.Transform(req.PriorState, func(path cty.Path, v cty.Value) (cty.Value, error) {
			if len(path) == 1 && path[0] == (cty.GetAttrStep{Name: "arg"}) {
				return cty.StringVal("current"), nil
			}
			return v, nil
		})
		if err != nil {
			// shouldn't get here
			t.Fatalf("ReadResourceFn transform failed")
			return providers.ReadResourceResponse{}
		}
		return providers.ReadResourceResponse{
			NewState: newVal,
		}
	}
	p.UpgradeResourceStateFn = func(req providers.UpgradeResourceStateRequest) (resp providers.UpgradeResourceStateResponse) {
		// We should've been given the prior state JSON as our input to upgrade.
		if !bytes.Contains(req.RawStateJSON, []byte("before")) {
			t.Fatalf("UpgradeResourceState request doesn't contain the 'before' object\n%s", req.RawStateJSON)
		}

		// We'll put something different in "arg" as part of upgrading, just
		// so that we can verify below that PrevRunState contains the upgraded
		// (but NOT refreshed) version of the object.
		resp.UpgradedState = cty.ObjectVal(map[string]cty.Value{
			"arg": cty.StringVal("upgraded"),
		})
		return resp
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.RefreshOnlyMode,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	if !p.UpgradeResourceStateCalled {
		t.Errorf("Provider's UpgradeResourceState wasn't called; should've been")
	}
	if !p.ReadResourceCalled {
		t.Errorf("Provider's ReadResource wasn't called; should've been")
	}

	if got, want := len(plan.Changes.Resources), 0; got != want {
		t.Errorf("plan contains resource changes; want none\n%s", spew.Sdump(plan.Changes.Resources))
	}

	if instState := plan.PriorState.ResourceInstance(addr); instState == nil {
		t.Errorf("%s has no prior state at all after plan", addr)
	} else {
		if obj := instState.Deposed[deposedKey]; obj == nil {
			t.Errorf("%s has no deposed object after plan", addr)
		} else if got, want := obj.AttrsJSON, `"current"`; !bytes.Contains(got, []byte(want)) {
			// Should've saved the result of refreshing
			t.Errorf("%s has wrong prior state after plan\ngot:\n%s\n\nwant substring: %s", addr, got, want)
		}
	}
	if instState := plan.PrevRunState.ResourceInstance(addr); instState == nil {
		t.Errorf("%s has no previous run state at all after plan", addr)
	} else {
		if obj := instState.Deposed[deposedKey]; obj == nil {
			t.Errorf("%s has no deposed object in the previous run state", addr)
		} else if got, want := obj.AttrsJSON, `"upgraded"`; !bytes.Contains(got, []byte(want)) {
			// Should've saved the result of upgrading
			t.Errorf("%s has wrong previous run state after plan\ngot:\n%s\n\nwant substring: %s", addr, got, want)
		}
	}

	// The output value should also have updated. If not, it's likely that we
	// skipped updating the working state to match the refreshed state when we
	// were evaluating the resource.
	if outChangeSrc := plan.Changes.OutputValue(addrs.RootModuleInstance.OutputValue("out")); outChangeSrc == nil {
		t.Errorf("no change planned for output value 'out'")
	} else {
		outChange, err := outChangeSrc.Decode()
		if err != nil {
			t.Fatalf("failed to decode output value 'out': %s", err)
		}
		got := outChange.After
		want := cty.UnknownVal(cty.String)
		if !want.RawEquals(got) {
			t.Errorf("wrong value for output value 'out'\ngot:  %#v\nwant: %#v", got, want)
		}
	}

	// Deposed objects should not be represented in drift.
	if len(plan.DriftedResources) > 0 {
		t.Errorf("unexpected drifted resources (%d)", len(plan.DriftedResources))
	}
}

func TestContext2Plan_refreshOnlyMode_orphan(t *testing.T) {
	addr := mustAbsResourceAddr("test_object.a")

	// The configuration, the prior state, and the refresh result intentionally
	// have different values for "test_string" so we can observe that the
	// refresh took effect but the configuration change wasn't considered.
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			resource "test_object" "a" {
				arg = "after"
				count = 1
			}

			output "out" {
				value = test_object.a.*.arg
			}
		`,
	})
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addr.Instance(addrs.IntKey(0)), &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{"arg":"before"}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
		s.SetResourceInstanceCurrent(addr.Instance(addrs.IntKey(1)), &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{"arg":"before"}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{Block: simpleTestSchema()},
		ResourceTypes: map[string]providers.Schema{
			"test_object": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"arg": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		newVal, err := cty.Transform(req.PriorState, func(path cty.Path, v cty.Value) (cty.Value, error) {
			if len(path) == 1 && path[0] == (cty.GetAttrStep{Name: "arg"}) {
				return cty.StringVal("current"), nil
			}
			return v, nil
		})
		if err != nil {
			// shouldn't get here
			t.Fatalf("ReadResourceFn transform failed")
			return providers.ReadResourceResponse{}
		}
		return providers.ReadResourceResponse{
			NewState: newVal,
		}
	}
	p.UpgradeResourceStateFn = func(req providers.UpgradeResourceStateRequest) (resp providers.UpgradeResourceStateResponse) {
		// We should've been given the prior state JSON as our input to upgrade.
		if !bytes.Contains(req.RawStateJSON, []byte("before")) {
			t.Fatalf("UpgradeResourceState request doesn't contain the 'before' object\n%s", req.RawStateJSON)
		}

		// We'll put something different in "arg" as part of upgrading, just
		// so that we can verify below that PrevRunState contains the upgraded
		// (but NOT refreshed) version of the object.
		resp.UpgradedState = cty.ObjectVal(map[string]cty.Value{
			"arg": cty.StringVal("upgraded"),
		})
		return resp
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.RefreshOnlyMode,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	if !p.UpgradeResourceStateCalled {
		t.Errorf("Provider's UpgradeResourceState wasn't called; should've been")
	}
	if !p.ReadResourceCalled {
		t.Errorf("Provider's ReadResource wasn't called; should've been")
	}

	if got, want := len(plan.Changes.Resources), 0; got != want {
		t.Errorf("plan contains resource changes; want none\n%s", spew.Sdump(plan.Changes.Resources))
	}

	if rState := plan.PriorState.Resource(addr); rState == nil {
		t.Errorf("%s has no prior state at all after plan", addr)
	} else {
		for i := 0; i < 2; i++ {
			instKey := addrs.IntKey(i)
			if obj := rState.Instance(instKey).Current; obj == nil {
				t.Errorf("%s%s has no object after plan", addr, instKey)
			} else if got, want := obj.AttrsJSON, `"current"`; !bytes.Contains(got, []byte(want)) {
				// Should've saved the result of refreshing
				t.Errorf("%s%s has wrong prior state after plan\ngot:\n%s\n\nwant substring: %s", addr, instKey, got, want)
			}
		}
	}
	if rState := plan.PrevRunState.Resource(addr); rState == nil {
		t.Errorf("%s has no prior state at all after plan", addr)
	} else {
		for i := 0; i < 2; i++ {
			instKey := addrs.IntKey(i)
			if obj := rState.Instance(instKey).Current; obj == nil {
				t.Errorf("%s%s has no object after plan", addr, instKey)
			} else if got, want := obj.AttrsJSON, `"upgraded"`; !bytes.Contains(got, []byte(want)) {
				// Should've saved the result of upgrading
				t.Errorf("%s%s has wrong prior state after plan\ngot:\n%s\n\nwant substring: %s", addr, instKey, got, want)
			}
		}
	}

	// The output value should also have updated. If not, it's likely that we
	// skipped updating the working state to match the refreshed state when we
	// were evaluating the resource.
	if outChangeSrc := plan.Changes.OutputValue(addrs.RootModuleInstance.OutputValue("out")); outChangeSrc == nil {
		t.Errorf("no change planned for output value 'out'")
	} else {
		outChange, err := outChangeSrc.Decode()
		if err != nil {
			t.Fatalf("failed to decode output value 'out': %s", err)
		}
		got := outChange.After
		want := cty.TupleVal([]cty.Value{cty.StringVal("current"), cty.StringVal("current")})
		if !want.RawEquals(got) {
			t.Errorf("wrong value for output value 'out'\ngot:  %#v\nwant: %#v", got, want)
		}
	}
}

func TestContext2Plan_invalidSensitiveModuleOutput(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"child/main.tf": `
output "out" {
  value = sensitive("xyz")
}`,
		"main.tf": `
module "child" {
  source = "./child"
}

output "root" {
  value = module.child.out
}`,
	})

	ctx := testContext2(t, &ContextOpts{})

	_, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
	if !diags.HasErrors() {
		t.Fatal("succeeded; want errors")
	}
	if got, want := diags.Err().Error(), "Output refers to sensitive values"; !strings.Contains(got, want) {
		t.Fatalf("wrong error:\ngot:  %s\nwant: message containing %q", got, want)
	}
}

func TestContext2Plan_planDataSourceSensitiveNested(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_instance" "bar" {
}

data "test_data_source" "foo" {
  foo {
    bar = test_instance.bar.sensitive
  }
}
`,
	})

	p := new(MockProvider)
	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
		resp.PlannedState = cty.ObjectVal(map[string]cty.Value{
			"sensitive": cty.UnknownVal(cty.String),
		})
		return resp
	}
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"test_instance": {
				Attributes: map[string]*configschema.Attribute{
					"sensitive": {
						Type:      cty.String,
						Computed:  true,
						Sensitive: true,
					},
				},
			},
		},
		DataSources: map[string]*configschema.Block{
			"test_data_source": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"bar": {Type: cty.String, Optional: true},
							},
						},
						Nesting: configschema.NestingSet,
					},
				},
			},
		},
	})

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("data.test_data_source.foo").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"string":"data_id", "foo":[{"bar":"old"}]}`),
			AttrSensitivePaths: []cty.PathValueMarks{
				{
					Path:  cty.GetAttrPath("foo"),
					Marks: cty.NewValueMarks(marks.Sensitive),
				},
			},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_instance.bar").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"sensitive":"old"}`),
			AttrSensitivePaths: []cty.PathValueMarks{
				{
					Path:  cty.GetAttrPath("sensitive"),
					Marks: cty.NewValueMarks(marks.Sensitive),
				},
			},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, DefaultPlanOpts)
	assertNoErrors(t, diags)

	for _, res := range plan.Changes.Resources {
		switch res.Addr.String() {
		case "test_instance.bar":
			if res.Action != plans.Update {
				t.Fatalf("unexpected %s change for %s", res.Action, res.Addr)
			}
		case "data.test_data_source.foo":
			if res.Action != plans.Read {
				t.Fatalf("unexpected %s change for %s", res.Action, res.Addr)
			}
		default:
			t.Fatalf("unexpected %s change for %s", res.Action, res.Addr)
		}
	}
}

func TestContext2Plan_forceReplace(t *testing.T) {
	addrA := mustResourceInstanceAddr("test_object.a")
	addrB := mustResourceInstanceAddr("test_object.b")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			resource "test_object" "a" {
			}
			resource "test_object" "b" {
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addrA, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
		s.SetResourceInstanceCurrent(addrB, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addrA,
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	t.Run(addrA.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addrA)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addrA)
		}

		if got, want := instPlan.Action, plans.DeleteThenCreate; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceReplaceByRequest; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run(addrB.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addrB)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addrB)
		}

		if got, want := instPlan.Action, plans.NoOp; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})
}

func TestContext2Plan_forceReplaceIncompleteAddr(t *testing.T) {
	addr0 := mustResourceInstanceAddr("test_object.a[0]")
	addr1 := mustResourceInstanceAddr("test_object.a[1]")
	addrBare := mustResourceInstanceAddr("test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			resource "test_object" "a" {
				count = 2
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addr0, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
		s.SetResourceInstanceCurrent(addr1, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addrBare,
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}
	diagsErr := diags.ErrWithWarnings()
	if diagsErr == nil {
		t.Fatalf("no warnings were returned")
	}
	if got, want := diagsErr.Error(), "Incompletely-matched force-replace resource instance"; !strings.Contains(got, want) {
		t.Errorf("missing expected warning\ngot:\n%s\n\nwant substring: %s", got, want)
	}

	t.Run(addr0.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addr0)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addr0)
		}

		if got, want := instPlan.Action, plans.NoOp; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run(addr1.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addr1)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addr1)
		}

		if got, want := instPlan.Action, plans.NoOp; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})
}

// Verify that adding a module instance does force existing module data sources
// to be deferred
func TestContext2Plan_noChangeDataSourceAddingModuleInstance(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
locals {
  data = {
    a = "a"
    b = "b"
  }
}

module "one" {
  source   = "./mod"
  for_each = local.data
  input = each.value
}

module "two" {
  source   = "./mod"
  for_each = module.one
  input = each.value.output
}
`,
		"mod/main.tf": `
variable "input" {
}

resource "test_resource" "x" {
  value = var.input
}

data "test_data_source" "d" {
  foo = test_resource.x.id
}

output "output" {
  value = test_resource.x.id
}
`,
	})

	p := testProvider("test")
	p.ReadDataSourceResponse = &providers.ReadDataSourceResponse{
		State: cty.ObjectVal(map[string]cty.Value{
			"id":  cty.StringVal("data"),
			"foo": cty.StringVal("foo"),
		}),
	}
	state := states.NewState()
	modOne := addrs.RootModuleInstance.Child("one", addrs.StringKey("a"))
	modTwo := addrs.RootModuleInstance.Child("two", addrs.StringKey("a"))
	one := state.EnsureModule(modOne)
	two := state.EnsureModule(modTwo)
	one.SetResourceInstanceCurrent(
		mustResourceInstanceAddr(`test_resource.x`).Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"foo","value":"a"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	one.SetResourceInstanceCurrent(
		mustResourceInstanceAddr(`data.test_data_source.d`).Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"data"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	two.SetResourceInstanceCurrent(
		mustResourceInstanceAddr(`test_resource.x`).Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"foo","value":"foo"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	two.SetResourceInstanceCurrent(
		mustResourceInstanceAddr(`data.test_data_source.d`).Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"data"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, DefaultPlanOpts)
	assertNoErrors(t, diags)

	for _, res := range plan.Changes.Resources {
		// both existing data sources should be read during plan
		if res.Addr.Module[0].InstanceKey == addrs.StringKey("b") {
			continue
		}

		if res.Addr.Resource.Resource.Mode == addrs.DataResourceMode && res.Action != plans.NoOp {
			t.Errorf("unexpected %s plan for %s", res.Action, res.Addr)
		}
	}
}

func TestContext2Plan_moduleExpandOrphansResourceInstance(t *testing.T) {
	// This test deals with the situation where a user has changed the
	// repetition/expansion mode for a module call while there are already
	// resource instances from the previous declaration in the state.
	//
	// This is conceptually just the same as removing the resources
	// from the module configuration only for that instance, but the
	// implementation of it ends up a little different because it's
	// an entry in the resource address's _module path_ that we'll find
	// missing, rather than the resource's own instance key, and so
	// our analyses need to handle that situation by indicating that all
	// of the resources under the missing module instance have zero
	// instances, regardless of which resource in that module we might
	// be asking about, and do so without tripping over any missing
	// registrations in the instance expander that might lead to panics
	// if we aren't careful.
	//
	// (For some history here, see https://github.com/hashicorp/terraform/issues/30110 )

	addrNoKey := mustResourceInstanceAddr("module.child.test_object.a[0]")
	addrZeroKey := mustResourceInstanceAddr("module.child[0].test_object.a[0]")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			module "child" {
				source = "./child"
				count = 1
			}
		`,
		"child/main.tf": `
			resource "test_object" "a" {
				count = 1
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		// Notice that addrNoKey is the address which lacks any instance key
		// for module.child, and so that module instance doesn't match the
		// call declared above with count = 1, and therefore the resource
		// inside is "orphaned" even though the resource block actually
		// still exists there.
		s.SetResourceInstanceCurrent(addrNoKey, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	t.Run(addrNoKey.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addrNoKey)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addrNoKey)
		}

		if got, want := instPlan.Addr, addrNoKey; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addrNoKey; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.Delete; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceDeleteBecauseNoModule; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})

	t.Run(addrZeroKey.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addrZeroKey)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addrZeroKey)
		}

		if got, want := instPlan.Addr, addrZeroKey; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addrZeroKey; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.Create; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})
}

func TestContext2Plan_resourcePreconditionPostcondition(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
variable "boop" {
  type = string
}

resource "test_resource" "a" {
  value = var.boop
  lifecycle {
    precondition {
      condition     = var.boop == "boop"
      error_message = "Wrong boop."
    }
    postcondition {
      condition     = self.output != ""
      error_message = "Output must not be blank."
    }
  }
}

`,
	})

	p := testProvider("test")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"test_resource": {
				Attributes: map[string]*configschema.Attribute{
					"value": {
						Type:     cty.String,
						Required: true,
					},
					"output": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
		},
	})

	t.Run("conditions pass", func(t *testing.T) {
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
			},
		})

		p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
			m := req.ProposedNewState.AsValueMap()
			m["output"] = cty.StringVal("bar")

			resp.PlannedState = cty.ObjectVal(m)
			resp.LegacyTypeSystem = true
			return resp
		}
		plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"boop": &InputValue{
					Value:      cty.StringVal("boop"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		assertNoErrors(t, diags)
		for _, res := range plan.Changes.Resources {
			switch res.Addr.String() {
			case "test_resource.a":
				if res.Action != plans.Create {
					t.Fatalf("unexpected %s change for %s", res.Action, res.Addr)
				}
			default:
				t.Fatalf("unexpected %s change for %s", res.Action, res.Addr)
			}
		}
	})

	t.Run("precondition fail", func(t *testing.T) {
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
			},
		})

		_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"boop": &InputValue{
					Value:      cty.StringVal("nope"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		if !diags.HasErrors() {
			t.Fatal("succeeded; want errors")
		}
		if got, want := diags.Err().Error(), "Resource precondition failed: Wrong boop."; got != want {
			t.Fatalf("wrong error:\ngot:  %s\nwant: %q", got, want)
		}
		if p.PlanResourceChangeCalled {
			t.Errorf("Provider's PlanResourceChange was called; should'nt've been")
		}
	})

	t.Run("precondition fail refresh-only", func(t *testing.T) {
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
			},
		})

		state := states.BuildState(func(s *states.SyncState) {
			s.SetResourceInstanceCurrent(mustResourceInstanceAddr("test_resource.a"), &states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"value":"boop","output":"blorp"}`),
				Status:    states.ObjectReady,
			}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
		})
		_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.RefreshOnlyMode,
			SetVariables: InputValues{
				"boop": &InputValue{
					Value:      cty.StringVal("nope"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		assertNoErrors(t, diags)
		if len(diags) == 0 {
			t.Fatalf("no diags, but should have warnings")
		}
		if got, want := diags.ErrWithWarnings().Error(), "Resource precondition failed: Wrong boop."; got != want {
			t.Fatalf("wrong warning:\ngot:  %s\nwant: %q", got, want)
		}
		if !p.ReadResourceCalled {
			t.Errorf("Provider's ReadResource wasn't called; should've been")
		}
	})

	t.Run("postcondition fail", func(t *testing.T) {
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
			},
		})

		p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
			m := req.ProposedNewState.AsValueMap()
			m["output"] = cty.StringVal("")

			resp.PlannedState = cty.ObjectVal(m)
			resp.LegacyTypeSystem = true
			return resp
		}
		_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"boop": &InputValue{
					Value:      cty.StringVal("boop"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		if !diags.HasErrors() {
			t.Fatal("succeeded; want errors")
		}
		if got, want := diags.Err().Error(), "Resource postcondition failed: Output must not be blank."; got != want {
			t.Fatalf("wrong error:\ngot:  %s\nwant: %q", got, want)
		}
		if !p.PlanResourceChangeCalled {
			t.Errorf("Provider's PlanResourceChange wasn't called; should've been")
		}
	})

	t.Run("postcondition fail refresh-only", func(t *testing.T) {
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
			},
		})

		state := states.BuildState(func(s *states.SyncState) {
			s.SetResourceInstanceCurrent(mustResourceInstanceAddr("test_resource.a"), &states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"value":"boop","output":"blorp"}`),
				Status:    states.ObjectReady,
			}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
		})
		p.ReadResourceFn = func(req providers.ReadResourceRequest) (resp providers.ReadResourceResponse) {
			newVal, err := cty.Transform(req.PriorState, func(path cty.Path, v cty.Value) (cty.Value, error) {
				if len(path) == 1 && path[0] == (cty.GetAttrStep{Name: "output"}) {
					return cty.StringVal(""), nil
				}
				return v, nil
			})
			if err != nil {
				// shouldn't get here
				t.Fatalf("ReadResourceFn transform failed")
				return providers.ReadResourceResponse{}
			}
			return providers.ReadResourceResponse{
				NewState: newVal,
			}
		}
		_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.RefreshOnlyMode,
			SetVariables: InputValues{
				"boop": &InputValue{
					Value:      cty.StringVal("boop"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		assertNoErrors(t, diags)
		if len(diags) == 0 {
			t.Fatalf("no diags, but should have warnings")
		}
		if got, want := diags.ErrWithWarnings().Error(), "Resource postcondition failed: Output must not be blank."; got != want {
			t.Fatalf("wrong warning:\ngot:  %s\nwant: %q", got, want)
		}
		if !p.ReadResourceCalled {
			t.Errorf("Provider's ReadResource wasn't called; should've been")
		}
		if p.PlanResourceChangeCalled {
			t.Errorf("Provider's PlanResourceChange was called; should'nt've been")
		}
	})

	t.Run("precondition and postcondition fail refresh-only", func(t *testing.T) {
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
			},
		})

		state := states.BuildState(func(s *states.SyncState) {
			s.SetResourceInstanceCurrent(mustResourceInstanceAddr("test_resource.a"), &states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"value":"boop","output":"blorp"}`),
				Status:    states.ObjectReady,
			}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
		})
		p.ReadResourceFn = func(req providers.ReadResourceRequest) (resp providers.ReadResourceResponse) {
			newVal, err := cty.Transform(req.PriorState, func(path cty.Path, v cty.Value) (cty.Value, error) {
				if len(path) == 1 && path[0] == (cty.GetAttrStep{Name: "output"}) {
					return cty.StringVal(""), nil
				}
				return v, nil
			})
			if err != nil {
				// shouldn't get here
				t.Fatalf("ReadResourceFn transform failed")
				return providers.ReadResourceResponse{}
			}
			return providers.ReadResourceResponse{
				NewState: newVal,
			}
		}
		_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.RefreshOnlyMode,
			SetVariables: InputValues{
				"boop": &InputValue{
					Value:      cty.StringVal("nope"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		assertNoErrors(t, diags)
		if got, want := len(diags), 2; got != want {
			t.Errorf("wrong number of warnings, got %d, want %d", got, want)
		}
		warnings := diags.ErrWithWarnings().Error()
		wantWarnings := []string{
			"Resource precondition failed: Wrong boop.",
			"Resource postcondition failed: Output must not be blank.",
		}
		for _, want := range wantWarnings {
			if !strings.Contains(warnings, want) {
				t.Errorf("missing warning:\ngot:  %s\nwant to contain: %q", warnings, want)
			}
		}
		if !p.ReadResourceCalled {
			t.Errorf("Provider's ReadResource wasn't called; should've been")
		}
		if p.PlanResourceChangeCalled {
			t.Errorf("Provider's PlanResourceChange was called; should'nt've been")
		}
	})
}

func TestContext2Plan_dataSourcePreconditionPostcondition(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
variable "boop" {
  type = string
}

data "test_data_source" "a" {
  foo = var.boop
  lifecycle {
    precondition {
      condition     = var.boop == "boop"
      error_message = "Wrong boop."
    }
    postcondition {
      condition     = length(self.results) > 0
      error_message = "Results cannot be empty."
    }
  }
}

resource "test_resource" "a" {
  value    = data.test_data_source.a.results[0]
}
`,
	})

	p := testProvider("test")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"test_resource": {
				Attributes: map[string]*configschema.Attribute{
					"value": {
						Type:     cty.String,
						Required: true,
					},
				},
			},
		},
		DataSources: map[string]*configschema.Block{
			"test_data_source": {
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.String,
						Required: true,
					},
					"results": {
						Type:     cty.List(cty.String),
						Computed: true,
					},
				},
			},
		},
	})

	t.Run("conditions pass", func(t *testing.T) {
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
			},
		})
		p.ReadDataSourceResponse = &providers.ReadDataSourceResponse{
			State: cty.ObjectVal(map[string]cty.Value{
				"foo":     cty.StringVal("boop"),
				"results": cty.ListVal([]cty.Value{cty.StringVal("boop")}),
			}),
		}
		plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"boop": &InputValue{
					Value:      cty.StringVal("boop"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		assertNoErrors(t, diags)
		for _, res := range plan.Changes.Resources {
			switch res.Addr.String() {
			case "test_resource.a":
				if res.Action != plans.Create {
					t.Fatalf("unexpected %s change for %s", res.Action, res.Addr)
				}
			case "data.test_data_source.a":
				if res.Action != plans.Read {
					t.Fatalf("unexpected %s change for %s", res.Action, res.Addr)
				}
			default:
				t.Fatalf("unexpected %s change for %s", res.Action, res.Addr)
			}
		}

		addr := mustResourceInstanceAddr("data.test_data_source.a")
		if gotResult := plan.Checks.GetObjectResult(addr); gotResult == nil {
			t.Errorf("no check result for %s", addr)
		} else {
			wantResult := &states.CheckResultObject{
				Status: checks.StatusPass,
			}
			if diff := cmp.Diff(wantResult, gotResult, valueComparer); diff != "" {
				t.Errorf("wrong check result for %s\n%s", addr, diff)
			}
		}
	})

	t.Run("precondition fail", func(t *testing.T) {
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
			},
		})
		_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"boop": &InputValue{
					Value:      cty.StringVal("nope"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		if !diags.HasErrors() {
			t.Fatal("succeeded; want errors")
		}
		if got, want := diags.Err().Error(), "Resource precondition failed: Wrong boop."; got != want {
			t.Fatalf("wrong error:\ngot:  %s\nwant: %q", got, want)
		}
		if p.ReadDataSourceCalled {
			t.Errorf("Provider's ReadResource was called; should'nt've been")
		}
	})

	t.Run("precondition fail refresh-only", func(t *testing.T) {
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
			},
		})
		plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.RefreshOnlyMode,
			SetVariables: InputValues{
				"boop": &InputValue{
					Value:      cty.StringVal("nope"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		assertNoErrors(t, diags)
		if len(diags) == 0 {
			t.Fatalf("no diags, but should have warnings")
		}
		if got, want := diags.ErrWithWarnings().Error(), "Resource precondition failed: Wrong boop."; got != want {
			t.Fatalf("wrong warning:\ngot:  %s\nwant: %q", got, want)
		}
		for _, res := range plan.Changes.Resources {
			switch res.Addr.String() {
			case "test_resource.a":
				if res.Action != plans.Create {
					t.Fatalf("unexpected %s change for %s", res.Action, res.Addr)
				}
			case "data.test_data_source.a":
				if res.Action != plans.Read {
					t.Fatalf("unexpected %s change for %s", res.Action, res.Addr)
				}
			default:
				t.Fatalf("unexpected %s change for %s", res.Action, res.Addr)
			}
		}
	})

	t.Run("postcondition fail", func(t *testing.T) {
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
			},
		})
		p.ReadDataSourceResponse = &providers.ReadDataSourceResponse{
			State: cty.ObjectVal(map[string]cty.Value{
				"foo":     cty.StringVal("boop"),
				"results": cty.ListValEmpty(cty.String),
			}),
		}
		_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"boop": &InputValue{
					Value:      cty.StringVal("boop"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		if !diags.HasErrors() {
			t.Fatal("succeeded; want errors")
		}
		if got, want := diags.Err().Error(), "Resource postcondition failed: Results cannot be empty."; got != want {
			t.Fatalf("wrong error:\ngot:  %s\nwant: %q", got, want)
		}
		if !p.ReadDataSourceCalled {
			t.Errorf("Provider's ReadDataSource wasn't called; should've been")
		}
	})

	t.Run("postcondition fail refresh-only", func(t *testing.T) {
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
			},
		})
		p.ReadDataSourceResponse = &providers.ReadDataSourceResponse{
			State: cty.ObjectVal(map[string]cty.Value{
				"foo":     cty.StringVal("boop"),
				"results": cty.ListValEmpty(cty.String),
			}),
		}
		plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.RefreshOnlyMode,
			SetVariables: InputValues{
				"boop": &InputValue{
					Value:      cty.StringVal("boop"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		assertNoErrors(t, diags)
		if got, want := diags.ErrWithWarnings().Error(), "Resource postcondition failed: Results cannot be empty."; got != want {
			t.Fatalf("wrong error:\ngot:  %s\nwant: %q", got, want)
		}
		addr := mustResourceInstanceAddr("data.test_data_source.a")
		if gotResult := plan.Checks.GetObjectResult(addr); gotResult == nil {
			t.Errorf("no check result for %s", addr)
		} else {
			wantResult := &states.CheckResultObject{
				Status: checks.StatusFail,
				FailureMessages: []string{
					"Results cannot be empty.",
				},
			}
			if diff := cmp.Diff(wantResult, gotResult, valueComparer); diff != "" {
				t.Errorf("wrong check result\n%s", diff)
			}
		}
	})

	t.Run("precondition and postcondition fail refresh-only", func(t *testing.T) {
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
			},
		})
		p.ReadDataSourceResponse = &providers.ReadDataSourceResponse{
			State: cty.ObjectVal(map[string]cty.Value{
				"foo":     cty.StringVal("nope"),
				"results": cty.ListValEmpty(cty.String),
			}),
		}
		_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.RefreshOnlyMode,
			SetVariables: InputValues{
				"boop": &InputValue{
					Value:      cty.StringVal("nope"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		assertNoErrors(t, diags)
		if got, want := len(diags), 2; got != want {
			t.Errorf("wrong number of warnings, got %d, want %d", got, want)
		}
		warnings := diags.ErrWithWarnings().Error()
		wantWarnings := []string{
			"Resource precondition failed: Wrong boop.",
			"Resource postcondition failed: Results cannot be empty.",
		}
		for _, want := range wantWarnings {
			if !strings.Contains(warnings, want) {
				t.Errorf("missing warning:\ngot:  %s\nwant to contain: %q", warnings, want)
			}
		}
	})
}

func TestContext2Plan_outputPrecondition(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
variable "boop" {
  type = string
}

output "a" {
  value = var.boop
  precondition {
    condition     = var.boop == "boop"
    error_message = "Wrong boop."
  }
}
`,
	})

	p := testProvider("test")

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	t.Run("condition pass", func(t *testing.T) {
		plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"boop": &InputValue{
					Value:      cty.StringVal("boop"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		assertNoErrors(t, diags)
		addr := addrs.RootModuleInstance.OutputValue("a")
		outputPlan := plan.Changes.OutputValue(addr)
		if outputPlan == nil {
			t.Fatalf("no plan for %s at all", addr)
		}
		if got, want := outputPlan.Addr, addr; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := outputPlan.Action, plans.Create; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if gotResult := plan.Checks.GetObjectResult(addr); gotResult == nil {
			t.Errorf("no check result for %s", addr)
		} else {
			wantResult := &states.CheckResultObject{
				Status: checks.StatusPass,
			}
			if diff := cmp.Diff(wantResult, gotResult, valueComparer); diff != "" {
				t.Errorf("wrong check result\n%s", diff)
			}
		}
	})

	t.Run("condition fail", func(t *testing.T) {
		_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"boop": &InputValue{
					Value:      cty.StringVal("nope"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		if !diags.HasErrors() {
			t.Fatal("succeeded; want errors")
		}
		if got, want := diags.Err().Error(), "Module output value precondition failed: Wrong boop."; got != want {
			t.Fatalf("wrong error:\ngot:  %s\nwant: %q", got, want)
		}
	})

	t.Run("condition fail refresh-only", func(t *testing.T) {
		plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.RefreshOnlyMode,
			SetVariables: InputValues{
				"boop": &InputValue{
					Value:      cty.StringVal("nope"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		assertNoErrors(t, diags)
		if len(diags) == 0 {
			t.Fatalf("no diags, but should have warnings")
		}
		if got, want := diags.ErrWithWarnings().Error(), "Module output value precondition failed: Wrong boop."; got != want {
			t.Errorf("wrong warning:\ngot:  %s\nwant: %q", got, want)
		}
		addr := addrs.RootModuleInstance.OutputValue("a")
		outputPlan := plan.Changes.OutputValue(addr)
		if outputPlan == nil {
			t.Fatalf("no plan for %s at all", addr)
		}
		if got, want := outputPlan.Addr, addr; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := outputPlan.Action, plans.Create; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if gotResult := plan.Checks.GetObjectResult(addr); gotResult == nil {
			t.Errorf("no condition result for %s", addr)
		} else {
			wantResult := &states.CheckResultObject{
				Status:          checks.StatusFail,
				FailureMessages: []string{"Wrong boop."},
			}
			if diff := cmp.Diff(wantResult, gotResult, valueComparer); diff != "" {
				t.Errorf("wrong condition result\n%s", diff)
			}
		}
	})
}

func TestContext2Plan_preconditionErrors(t *testing.T) {
	testCases := []struct {
		condition   string
		wantSummary string
		wantDetail  string
	}{
		{
			"data.test_data_source",
			"Invalid reference",
			`The "data" object must be followed by two attribute names`,
		},
		{
			"self.value",
			`Invalid "self" reference`,
			"only in resource provisioner, connection, and postcondition blocks",
		},
		{
			"data.foo.bar",
			"Reference to undeclared resource",
			`A data resource "foo" "bar" has not been declared in the root module`,
		},
		{
			"test_resource.b.value",
			"Invalid condition result",
			"Condition expression must return either true or false",
		},
		{
			"test_resource.c.value",
			"Invalid condition result",
			"Invalid condition result value: a bool is required",
		},
	}

	p := testProvider("test")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	for _, tc := range testCases {
		t.Run(tc.condition, func(t *testing.T) {
			main := fmt.Sprintf(`
			resource "test_resource" "a" {
				value = var.boop
				lifecycle {
					precondition {
						condition     = %s
						error_message = "Not relevant."
					}
				}
			}

			resource "test_resource" "b" {
				value = null
			}

			resource "test_resource" "c" {
				value = "bar"
			}
			`, tc.condition)
			m := testModuleInline(t, map[string]string{"main.tf": main})

			plan, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
			if !diags.HasErrors() {
				t.Fatal("succeeded; want errors")
			}

			if !plan.Errored {
				t.Fatal("plan failed to record error")
			}

			diag := diags[0]
			if got, want := diag.Description().Summary, tc.wantSummary; got != want {
				t.Errorf("unexpected summary\n got: %s\nwant: %s", got, want)
			}
			if got, want := diag.Description().Detail, tc.wantDetail; !strings.Contains(got, want) {
				t.Errorf("unexpected summary\ngot: %s\nwant to contain %q", got, want)
			}

			for _, kv := range plan.Checks.ConfigResults.Elements() {
				// All these are configuration or evaluation errors
				if kv.Value.Status != checks.StatusError {
					t.Errorf("incorrect status, got %s", kv.Value.Status)
				}
			}
		})
	}
}

func TestContext2Plan_preconditionSensitiveValues(t *testing.T) {
	p := testProvider("test")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	m := testModuleInline(t, map[string]string{
		"main.tf": `
variable "boop" {
  sensitive = true
  type      = string
}

output "a" {
  sensitive = true
  value     = var.boop

  precondition {
    condition     = length(var.boop) <= 4
    error_message = "Boop is too long, ${length(var.boop)} > 4"
  }
}
`,
	})

	_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		SetVariables: InputValues{
			"boop": &InputValue{
				Value:      cty.StringVal("bleep"),
				SourceType: ValueFromCLIArg,
			},
		},
	})
	if !diags.HasErrors() {
		t.Fatal("succeeded; want errors")
	}
	if got, want := len(diags), 2; got != want {
		t.Errorf("wrong number of diags, got %d, want %d", got, want)
	}
	for _, diag := range diags {
		desc := diag.Description()
		if desc.Summary == "Module output value precondition failed" {
			if got, want := desc.Detail, "This check failed, but has an invalid error message as described in the other accompanying messages."; !strings.Contains(got, want) {
				t.Errorf("unexpected detail\ngot: %s\nwant to contain %q", got, want)
			}
		} else if desc.Summary == "Error message refers to sensitive values" {
			if got, want := desc.Detail, "The error expression used to explain this condition refers to sensitive values, so OpenTofu will not display the resulting message."; !strings.Contains(got, want) {
				t.Errorf("unexpected detail\ngot: %s\nwant to contain %q", got, want)
			}
		} else {
			t.Errorf("unexpected summary\ngot: %s", desc.Summary)
		}
	}
}

func TestContext2Plan_triggeredBy(t *testing.T) {
	type TestConfiguration struct {
		Description         string
		inlineConfiguration map[string]string
	}
	configurations := []TestConfiguration{
		{
			Description: "TF configuration",
			inlineConfiguration: map[string]string{
				"main.tf": `
resource "test_object" "a" {
  count = 1
  test_string = "new"
}
resource "test_object" "b" {
  count = 1
  test_string = test_object.a[count.index].test_string
  lifecycle {
    # the change to test_string in the other resource should trigger replacement
    replace_triggered_by = [ test_object.a[count.index].test_string ]
  }
}
`,
			},
		},
		{
			Description: "Json configuration",
			inlineConfiguration: map[string]string{
				"main.tf.json": `
{
    "resource": {
        "test_object": {
            "a": [
                {
                    "count": 1,
                    "test_string": "new"
                }
            ],
            "b": [
                {
                    "count": 1,
                    "lifecycle": [
                        {
                            "replace_triggered_by": [
                                "test_object.a[count.index].test_string"
                            ]
                        }
                    ],
                    "test_string": "test_object.a[count.index].test_string"
                }
            ]
        }
    }
}
`,
			},
		},
	}

	p := simpleMockProvider()

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			mustResourceInstanceAddr("test_object.a[0]"),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"test_string":"old"}`),
				Status:    states.ObjectReady,
			},
			mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
			addrs.NoKey,
		)
		s.SetResourceInstanceCurrent(
			mustResourceInstanceAddr("test_object.b[0]"),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{}`),
				Status:    states.ObjectReady,
			},
			mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
			addrs.NoKey,
		)
	})
	for _, configuration := range configurations {
		m := testModuleInline(t, configuration.inlineConfiguration)

		plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.NormalMode,
		})
		if diags.HasErrors() {
			t.Fatalf("unexpected errors\n%s", diags.Err().Error())
		}
		for _, c := range plan.Changes.Resources {
			switch c.Addr.String() {
			case "test_object.a[0]":
				if c.Action != plans.Update {
					t.Fatalf("unexpected %s change for %s\n", c.Action, c.Addr)
				}
			case "test_object.b[0]":
				if c.Action != plans.DeleteThenCreate {
					t.Fatalf("unexpected %s change for %s\n", c.Action, c.Addr)
				}
				if c.ActionReason != plans.ResourceInstanceReplaceByTriggers {
					t.Fatalf("incorrect reason for change: %s\n", c.ActionReason)
				}
			default:
				t.Fatal("unexpected change", c.Addr, c.Action)
			}
		}
	}
}

func TestContext2Plan_dataSchemaChange(t *testing.T) {
	// We can't decode the prior state when a data source upgrades the schema
	// in an incompatible way. Since prior state for data sources is purely
	// informational, decoding should be skipped altogether.
	m := testModuleInline(t, map[string]string{
		"main.tf": `
data "test_object" "a" {
  obj {
    # args changes from a list to a map
    args = {
      val = "string"
	}
  }
}
`,
	})

	p := new(MockProvider)
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		DataSources: map[string]*configschema.Block{
			"test_object": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
				BlockTypes: map[string]*configschema.NestedBlock{
					"obj": {
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"args": {Type: cty.Map(cty.String), Optional: true},
							},
						},
						Nesting: configschema.NestingSet,
					},
				},
			},
		},
	})

	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) (resp providers.ReadDataSourceResponse) {
		resp.State = req.Config
		return resp
	}

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(mustResourceInstanceAddr(`data.test_object.a`), &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{"id":"old","obj":[{"args":["string"]}]}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, DefaultPlanOpts)
	assertNoErrors(t, diags)
}

func TestContext2Plan_applyGraphError(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
}
resource "test_object" "b" {
	depends_on = [test_object.a]
}
`,
	})

	p := simpleMockProvider()

	// Here we introduce a cycle via state which only shows up in the apply
	// graph where the actual destroy instances are connected in the graph.
	// This could happen for example when a user has an existing state with
	// stored dependencies, and changes the config in such a way that
	// contradicts the stored dependencies.
	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.a").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectTainted,
			AttrsJSON:    []byte(`{"test_string":"a"}`),
			Dependencies: []addrs.ConfigResource{mustResourceInstanceAddr("test_object.b").ContainingResource().Config()},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.b").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectTainted,
			AttrsJSON: []byte(`{"test_string":"b"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
	})
	if !diags.HasErrors() {
		t.Fatal("cycle error not detected")
	}

	msg := diags.ErrWithWarnings().Error()
	if !strings.Contains(msg, "Cycle") {
		t.Fatalf("no cycle error found:\n got: %s\n", msg)
	}
}

// plan a destroy with no state where configuration could fail to evaluate
// expansion indexes.
func TestContext2Plan_emptyDestroy(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
locals {
  enable = true
  value  = local.enable ? module.example[0].out : null
}

module "example" {
  count  = local.enable ? 1 : 0
  source = "./example"
}
`,
		"example/main.tf": `
resource "test_resource" "x" {
}

output "out" {
  value = test_resource.x
}
`,
	})

	p := testProvider("test")
	state := states.NewState()

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.DestroyMode,
	})

	assertNoErrors(t, diags)

	// ensure that the given states are valid and can be serialized
	if plan.PrevRunState == nil {
		t.Fatal("nil plan.PrevRunState")
	}
	if plan.PriorState == nil {
		t.Fatal("nil plan.PriorState")
	}
}

// A deposed instances which no longer exists during ReadResource creates NoOp
// change, which should not effect the plan.
func TestContext2Plan_deposedNoLongerExists(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "b" {
  count = 1
  test_string = "updated"
  lifecycle {
    create_before_destroy = true
  }
}
`,
	})

	p := simpleMockProvider()
	p.ReadResourceFn = func(req providers.ReadResourceRequest) (resp providers.ReadResourceResponse) {
		s := req.PriorState.GetAttr("test_string").AsString()
		if s == "current" {
			resp.NewState = req.PriorState
			return resp
		}
		// pretend the non-current instance has been deleted already
		resp.NewState = cty.NullVal(req.PriorState.Type())
		return resp
	}

	// Here we introduce a cycle via state which only shows up in the apply
	// graph where the actual destroy instances are connected in the graph.
	// This could happen for example when a user has an existing state with
	// stored dependencies, and changes the config in such a way that
	// contradicts the stored dependencies.
	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceDeposed(
		mustResourceInstanceAddr("test_object.a[0]").Resource,
		states.DeposedKey("deposed"),
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectTainted,
			AttrsJSON:    []byte(`{"test_string":"old"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.a[0]").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectTainted,
			AttrsJSON:    []byte(`{"test_string":"current"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
	})
	assertNoErrors(t, diags)
}

// make sure there are no cycles with changes around a provider configured via
// managed resources.
func TestContext2Plan_destroyWithResourceConfiguredProvider(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
  in = "a"
}

provider "test" {
  alias = "other"
  in = test_object.a.out
}

resource "test_object" "b" {
  provider = test.other
  in = "a"
}
`})

	testProvider := &MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			Provider: providers.Schema{
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"in": {
							Type:     cty.String,
							Optional: true,
						},
					},
				},
			},
			ResourceTypes: map[string]providers.Schema{
				"test_object": providers.Schema{
					Block: &configschema.Block{
						Attributes: map[string]*configschema.Attribute{
							"in": {
								Type:     cty.String,
								Optional: true,
							},
							"out": {
								Type:     cty.Number,
								Computed: true,
							},
						},
					},
				},
			},
		},
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(testProvider),
		},
	})

	// plan+apply to create the initial state
	opts := SimplePlanOpts(plans.NormalMode, testInputValuesUnset(m.Module.Variables))
	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), opts)
	assertNoErrors(t, diags)
	state, diags := ctx.Apply(context.Background(), plan, m)
	assertNoErrors(t, diags)

	// Resource changes which have dependencies across providers which
	// themselves depend on resources can result in cycles.
	// Because other_object transitively depends on the module resources
	// through its provider, we trigger changes on both sides of this boundary
	// to ensure we can create a valid plan.
	//
	// Try to replace both instances
	addrA := mustResourceInstanceAddr("test_object.a")
	addrB := mustResourceInstanceAddr(`test_object.b`)
	opts.ForceReplace = []addrs.AbsResourceInstance{addrA, addrB}

	_, diags = ctx.Plan(context.Background(), m, state, opts)
	assertNoErrors(t, diags)
}

func TestContext2Plan_destroyPartialState(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
}

output "out" {
  value = module.mod.out
}

module "mod" {
  source = "./mod"
}
`,

		"./mod/main.tf": `
resource "test_object" "a" {
  count = 2

  lifecycle {
    precondition {
	  # test_object_b has already been destroyed, so referencing the first
      # instance must not fail during a destroy plan.
      condition = test_object.b[0].test_string == "invalid"
      error_message = "should not block destroy"
    }
    precondition {
      # this failing condition should bot block a destroy plan
      condition = !local.continue
      error_message = "should not block destroy"
    }
  }
}

resource "test_object" "b" {
  count = 2
}

locals {
  continue = true
}

output "out" {
  # the reference to test_object.b[0] may not be valid during a destroy plan,
  # but should not fail.
  value = local.continue ? test_object.a[1].test_string != "invalid"  && test_object.b[0].test_string != "invalid" : false

  precondition {
    # test_object_b has already been destroyed, so referencing the first
    # instance must not fail during a destroy plan.
    condition = test_object.b[0].test_string == "invalid"
    error_message = "should not block destroy"
  }
  precondition {
    # this failing condition should bot block a destroy plan
    condition = test_object.a[0].test_string == "invalid"
    error_message = "should not block destroy"
  }
}
`})

	p := simpleMockProvider()

	// This state could be the result of a failed destroy, leaving only 2
	// remaining instances. We want to be able to continue the destroy to
	// remove everything without blocking on invalid references or failing
	// conditions.
	state := states.NewState()
	mod := state.EnsureModule(addrs.RootModuleInstance.Child("mod", addrs.NoKey))
	mod.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.a[0]").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectTainted,
			AttrsJSON:    []byte(`{"test_string":"current"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	mod.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.a[1]").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectTainted,
			AttrsJSON:    []byte(`{"test_string":"current"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.DestroyMode,
	})
	assertNoErrors(t, diags)
}

func TestContext2Plan_destroyPartialStateLocalRef(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
module "already_destroyed" {
  count = 1
  source = "./mod"
}

locals {
  eval_error = module.already_destroyed[0].out
}

output "already_destroyed" {
  value = local.eval_error
}

`,

		"./mod/main.tf": `
resource "test_object" "a" {
}

output "out" {
  value = test_object.a.test_string
}
`})

	p := simpleMockProvider()

	state := states.NewState()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.DestroyMode,
	})
	assertNoErrors(t, diags)
}

// Make sure the data sources in the prior state are serializable even if
// there were an error in the plan.
func TestContext2Plan_dataSourceReadPlanError(t *testing.T) {
	m, snap := testModuleWithSnapshot(t, "data-source-read-with-plan-error")
	awsProvider := testProvider("aws")
	testProvider := testProvider("test")

	testProvider.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
		resp.PlannedState = req.ProposedNewState
		resp.Diagnostics = resp.Diagnostics.Append(errors.New("oops"))
		return resp
	}

	state := states.NewState()

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"):  testProviderFuncFixed(awsProvider),
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(testProvider),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, DefaultPlanOpts)
	if !diags.HasErrors() {
		t.Fatalf("expected plan error")
	}

	// make sure we can serialize the plan even if there were an error
	_, _, _, err := contextOptsForPlanViaFile(t, snap, plan)
	if err != nil {
		t.Fatalf("failed to round-trip through planfile: %s", err)
	}
}

func TestContext2Plan_ignoredMarkedValue(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
  map = {
    prior = "value"
    new   = sensitive("ignored")
  }
}
`})

	testProvider := &MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			ResourceTypes: map[string]providers.Schema{
				"test_object": providers.Schema{
					Block: &configschema.Block{
						Attributes: map[string]*configschema.Attribute{
							"map": {
								Type:     cty.Map(cty.String),
								Optional: true,
							},
						},
					},
				},
			},
		},
	}

	testProvider.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
		// We're going to ignore any changes here and return the prior state.
		resp.PlannedState = req.PriorState
		return resp
	}

	state := states.NewState()
	root := state.RootModule()
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.a").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"map":{"prior":"value"}}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(testProvider),
		},
	})

	// plan+apply to create the initial state
	opts := SimplePlanOpts(plans.NormalMode, testInputValuesUnset(m.Module.Variables))
	plan, diags := ctx.Plan(context.Background(), m, state, opts)
	assertNoErrors(t, diags)

	for _, c := range plan.Changes.Resources {
		if c.Action != plans.NoOp {
			t.Errorf("unexpected %s change for %s", c.Action, c.Addr)
		}
	}
}

func TestContext2Plan_importResourceBasic(t *testing.T) {
	addr := mustResourceInstanceAddr("test_object.a")

	type TestConfiguration struct {
		Description         string
		inlineConfiguration map[string]string
	}
	configurations := []TestConfiguration{
		{
			Description: "TF configuration",
			inlineConfiguration: map[string]string{
				"main.tf": `
resource "test_object" "a" {
  test_string = "foo"
}

import {
  to   = test_object.a
  id   = "123"
}
`,
			},
		},
		{
			Description: "Json configuration",
			inlineConfiguration: map[string]string{
				"main.tf.json": `
{
    "import": [
        {
            "id": "123",
            "to": "test_object.a"
        }
    ],
    "resource": {
        "test_object": {
            "a": [
		        {
		            "test_string": "foo"
		        }
            ]
        }
    }
}
`,
			},
		},
	}

	p := simpleMockProvider()
	hook := new(MockHook)
	ctx := testContext2(t, &ContextOpts{
		Hooks: []Hook{hook},
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})
	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.ObjectVal(map[string]cty.Value{
			"test_string": cty.StringVal("foo"),
		}),
	}
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_object",
				State: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			},
		},
	}

	for _, configuration := range configurations {
		m := testModuleInline(t, configuration.inlineConfiguration)

		plan, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
		if diags.HasErrors() {
			t.Fatalf("unexpected errors\n%s", diags.Err().Error())
		}

		t.Run(configuration.Description, func(t *testing.T) {
			instPlan := plan.Changes.ResourceInstance(addr)
			if instPlan == nil {
				t.Fatalf("no plan for %s at all", addr)
			}

			if got, want := instPlan.Addr, addr; !got.Equal(want) {
				t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.PrevRunAddr, addr; !got.Equal(want) {
				t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.Action, plans.NoOp; got != want {
				t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
				t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
			}
			if instPlan.Importing.ID != "123" {
				t.Errorf("expected import change from \"123\", got non-import change")
			}

			if !hook.PrePlanImportCalled {
				t.Fatalf("PostPlanImport hook not called")
			}
			if addr, wantAddr := hook.PrePlanImportAddr, instPlan.Addr; !addr.Equal(wantAddr) {
				t.Errorf("expected addr to be %s, but was %s", wantAddr, addr)
			}

			if !hook.PostPlanImportCalled {
				t.Fatalf("PostPlanImport hook not called")
			}
			if addr, wantAddr := hook.PostPlanImportAddr, instPlan.Addr; !addr.Equal(wantAddr) {
				t.Errorf("expected addr to be %s, but was %s", wantAddr, addr)
			}
		})
	}
}

func TestContext2Plan_importToDynamicAddress(t *testing.T) {
	type TestConfiguration struct {
		Description         string
		ResolvedAddress     string
		inlineConfiguration map[string]string
	}
	configurations := []TestConfiguration{
		{
			Description:     "To address includes a variable as index in TF configuration",
			ResolvedAddress: "test_object.a[0]",
			inlineConfiguration: map[string]string{
				"main.tf": `
variable "index" {
  default = 0
}

resource "test_object" "a" {
  count = 1
  test_string = "foo"
}

import {
  to   = test_object.a[var.index]
  id   = "%d"
}
`,
			},
		},
		{
			Description:     "To address includes a variable as index in JSON configuration",
			ResolvedAddress: "test_object.a[0]",
			inlineConfiguration: map[string]string{
				"main.tf.json": `
{
     "locals": [
        {
            "index": 0
        }
    ],
    "import": [
        {
            "id": "%d",
            "to": "test_object.a[local.index]"
        }
    ],
    "resource": {
        "test_object": {
            "a": [
		        {
                    "count": 1,
		            "test_string": "foo"
		        }
            ]
        }
    }
}
`,
			},
		},
		{
			Description:     "To address includes a local as index",
			ResolvedAddress: "test_object.a[0]",
			inlineConfiguration: map[string]string{
				"main.tf": `
locals {
  index = 0
}

resource "test_object" "a" {
  count = 1
  test_string = "foo"
}

import {
  to   = test_object.a[local.index]
  id   = "%d"
}
`,
			},
		},
		{
			Description:     "To address includes a conditional expression as index",
			ResolvedAddress: "test_object.a[\"zero\"]",
			inlineConfiguration: map[string]string{
				"main.tf": `
resource "test_object" "a" {
  for_each = toset(["zero"])
  test_string = "foo"
}

import {
  to   = test_object.a[ true ? "zero" : "one"]
  id   = "%d"
}
`,
			},
		},
		{
			Description:     "To address includes a conditional expression with vars and locals as index",
			ResolvedAddress: "test_object.a[\"one\"]",
			inlineConfiguration: map[string]string{
				"main.tf": `
variable "one" {
  default = 1
}

locals {
  zero = "zero"
  one = "one"
}

resource "test_object" "a" {
  for_each = toset(["one"])
  test_string = "foo"
}

import {
  to   = test_object.a[var.one == 1 ? local.one : local.zero]
  id   = "%d"
}
`,
			},
		},
		{
			Description:     "To address includes a resource reference as index",
			ResolvedAddress: "test_object.a[\"boop\"]",
			inlineConfiguration: map[string]string{
				"main.tf": `
resource "test_object" "reference" {
  test_string = "boop"
}

resource "test_object" "a" {
  for_each = toset(["boop"])
  test_string = "foo"
}

import {
  to   = test_object.a[test_object.reference.test_string]
  id   = "%d"
}
`,
			},
		},
		{
			Description:     "To address includes a data reference as index",
			ResolvedAddress: "test_object.a[\"bip\"]",
			inlineConfiguration: map[string]string{
				"main.tf": `
data "test_object" "reference" {
}

resource "test_object" "a" {
  for_each = toset(["bip"])
  test_string = "foo"
}

import {
  to   = test_object.a[data.test_object.reference.test_string]
  id   = "%d"
}
`,
			},
		},
	}

	const importId = 123

	for _, configuration := range configurations {
		t.Run(configuration.Description, func(t *testing.T) {

			// Format the configuration with the import ID
			formattedConfiguration := make(map[string]string)
			for configFileName, configFileContent := range configuration.inlineConfiguration {
				formattedConfigFileContent := fmt.Sprintf(configFileContent, importId)
				formattedConfiguration[configFileName] = formattedConfigFileContent
			}

			addr := mustResourceInstanceAddr(configuration.ResolvedAddress)
			m := testModuleInline(t, formattedConfiguration)

			p := &MockProvider{
				GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
					Provider: providers.Schema{Block: simpleTestSchema()},
					ResourceTypes: map[string]providers.Schema{
						"test_object": providers.Schema{Block: simpleTestSchema()},
					},
					DataSources: map[string]providers.Schema{
						"test_object": providers.Schema{
							Block: &configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"test_string": {
										Type:     cty.String,
										Optional: true,
									},
								},
							},
						},
					},
				},
			}

			hook := new(MockHook)
			ctx := testContext2(t, &ContextOpts{
				Hooks: []Hook{hook},
				Providers: map[addrs.Provider]providers.Factory{
					addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
				},
			})
			p.ReadResourceResponse = &providers.ReadResourceResponse{
				NewState: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			}

			p.ReadDataSourceResponse = &providers.ReadDataSourceResponse{
				State: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("bip"),
				}),
			}

			p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
				ImportedResources: []providers.ImportedResource{
					{
						TypeName: "test_object",
						State: cty.ObjectVal(map[string]cty.Value{
							"test_string": cty.StringVal("foo"),
						}),
					},
				},
			}

			plan, diags := ctx.Plan(context.Background(), m, states.NewState(), SimplePlanOpts(plans.NormalMode, testInputValuesUnset(m.Module.Variables)))
			if diags.HasErrors() {
				t.Fatalf("unexpected errors\n%s", diags.Err().Error())
			}

			t.Run(addr.String(), func(t *testing.T) {
				instPlan := plan.Changes.ResourceInstance(addr)
				if instPlan == nil {
					t.Fatalf("no plan for %s at all", addr)
				}

				if got, want := instPlan.Addr, addr; !got.Equal(want) {
					t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
				}
				if got, want := instPlan.PrevRunAddr, addr; !got.Equal(want) {
					t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
				}
				if got, want := instPlan.Action, plans.NoOp; got != want {
					t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
				}
				if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
					t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
				}
				if instPlan.Importing.ID != strconv.Itoa(importId) {
					t.Errorf("expected import change from \"%d\", got non-import change", importId)
				}

				if !hook.PrePlanImportCalled {
					t.Fatalf("PostPlanImport hook not called")
				}
				if addr, wantAddr := hook.PrePlanImportAddr, instPlan.Addr; !addr.Equal(wantAddr) {
					t.Errorf("expected addr to be %s, but was %s", wantAddr, addr)
				}

				if !hook.PostPlanImportCalled {
					t.Fatalf("PostPlanImport hook not called")
				}
				if addr, wantAddr := hook.PostPlanImportAddr, instPlan.Addr; !addr.Equal(wantAddr) {
					t.Errorf("expected addr to be %s, but was %s", wantAddr, addr)
				}
			})
		})
	}
}

func TestContext2Plan_importToInvalidDynamicAddress(t *testing.T) {
	type TestConfiguration struct {
		Description         string
		expectedError       string
		inlineConfiguration map[string]string
	}
	configurations := []TestConfiguration{
		{
			Description:   "To address index value is null",
			expectedError: "Import block 'to' address contains an invalid key: Import block contained a resource address using an index which is null. Please ensure the expression for the index is not null",
			inlineConfiguration: map[string]string{
				"main.tf": `
variable "index" {
  default = null
}

resource "test_object" "a" {
  count = 1
  test_string = "foo"
}

import {
  to   = test_object.a[var.index]
  id   = "123"
}
`,
			},
		},
		{
			Description:   "To address index is not a number or a string",
			expectedError: "Import block 'to' address contains an invalid key: Import block contained a resource address using an index which is not valid for a resource instance (not a string or a number). Please ensure the expression for the index is correct, and returns either a string or a number",
			inlineConfiguration: map[string]string{
				"main.tf": `
locals {
  index = toset(["foo"])
}

resource "test_object" "a" {
  for_each = toset(["foo"])
  test_string = "foo"
}

import {
  to   = test_object.a[local.index]
  id   = "123"
}
`,
			},
		},
		{
			Description:   "To address index value is sensitive",
			expectedError: "Import block 'to' address contains an invalid key: Import block contained a resource address using an index which is sensitive. Please ensure indexes used in the resource address of an import target are not sensitive",
			inlineConfiguration: map[string]string{
				"main.tf": `
locals {
  index = sensitive("foo")
}

resource "test_object" "a" {
  for_each = toset(["foo"])
  test_string = "foo"
}

import {
  to   = test_object.a[local.index]
  id   = "123"
}
`,
			},
		},
		{
			Description:   "To address index value will only be known after apply",
			expectedError: "Import block contained a resource address using an index that will only be known after apply. Please ensure to use expressions that are known at plan time for the index of an import target address",
			inlineConfiguration: map[string]string{
				"main.tf": `
resource "test_object" "reference" {
}

resource "test_object" "a" {
  count = 1
  test_string = "foo"
}

import {
  to   = test_object.a[test_object.reference.id]
  id   = "123"
}
`,
			},
		},
	}

	for _, configuration := range configurations {
		t.Run(configuration.Description, func(t *testing.T) {
			m := testModuleInline(t, configuration.inlineConfiguration)

			providerSchema := &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"test_string": {
						Type:     cty.String,
						Optional: true,
					},
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
			}

			p := &MockProvider{
				GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
					Provider: providers.Schema{Block: providerSchema},
					ResourceTypes: map[string]providers.Schema{
						"test_object": providers.Schema{Block: providerSchema},
					},
				},
			}

			hook := new(MockHook)
			ctx := testContext2(t, &ContextOpts{
				Hooks: []Hook{hook},
				Providers: map[addrs.Provider]providers.Factory{
					addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
				},
			})

			p.ReadResourceResponse = &providers.ReadResourceResponse{
				NewState: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			}

			p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
				testStringVal := req.ProposedNewState.GetAttr("test_string")
				return providers.PlanResourceChangeResponse{
					PlannedState: cty.ObjectVal(map[string]cty.Value{
						"test_string": testStringVal,
						"id":          cty.UnknownVal(cty.String),
					}),
				}
			}

			p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
				ImportedResources: []providers.ImportedResource{
					{
						TypeName: "test_object",
						State: cty.ObjectVal(map[string]cty.Value{
							"test_string": cty.StringVal("foo"),
						}),
					},
				},
			}

			_, diags := ctx.Plan(context.Background(), m, states.NewState(), SimplePlanOpts(plans.NormalMode, testInputValuesUnset(m.Module.Variables)))

			if !diags.HasErrors() {
				t.Fatal("succeeded; want errors")
			}
			if got, want := diags.Err().Error(), configuration.expectedError; !strings.Contains(got, want) {
				t.Fatalf("wrong error:\ngot:  %s\nwant: message containing %q", got, want)
			}
		})
	}
}

func TestContext2Plan_importForEach(t *testing.T) {
	type ImportResult struct {
		ResolvedAddress string
		ResolvedId      string
	}
	type TestConfiguration struct {
		Description         string
		ImportResults       []ImportResult
		inlineConfiguration map[string]string
	}
	configurations := []TestConfiguration{
		{
			Description:   "valid map",
			ImportResults: []ImportResult{{ResolvedAddress: `test_object.a["key1"]`, ResolvedId: "val1"}, {ResolvedAddress: `test_object.a["key2"]`, ResolvedId: "val2"}, {ResolvedAddress: `test_object.a["key3"]`, ResolvedId: "val3"}},
			inlineConfiguration: map[string]string{
				"main.tf": `
locals {
  map = {
    "key1" = "val1"
    "key2" = "val2"
    "key3" = "val3"
  }
}

resource "test_object" "a" {
  for_each = local.map
}

import {
  for_each = local.map
  to = test_object.a[each.key]
  id = each.value
}
`,
			},
		},
		{
			Description:   "valid set",
			ImportResults: []ImportResult{{ResolvedAddress: `test_object.a["val0"]`, ResolvedId: "val0"}, {ResolvedAddress: `test_object.a["val1"]`, ResolvedId: "val1"}, {ResolvedAddress: `test_object.a["val2"]`, ResolvedId: "val2"}},
			inlineConfiguration: map[string]string{
				"main.tf": `
variable "set" {
	type = set(string)
	default = ["val0", "val1", "val2"]
}

resource "test_object" "a" {
  for_each = var.set
}

import {
  for_each = var.set
  to = test_object.a[each.key]
  id = each.value
}
`,
			},
		},
		{
			Description:   "valid tuple",
			ImportResults: []ImportResult{{ResolvedAddress: `module.mod[0].test_object.a["resKey1"]`, ResolvedId: "val1"}, {ResolvedAddress: `module.mod[0].test_object.a["resKey2"]`, ResolvedId: "val2"}, {ResolvedAddress: `module.mod[1].test_object.a["resKey1"]`, ResolvedId: "val3"}, {ResolvedAddress: `module.mod[1].test_object.a["resKey2"]`, ResolvedId: "val4"}},
			inlineConfiguration: map[string]string{
				"mod/main.tf": `
variable "set" {
	type = set(string)
	default = ["resKey1", "resKey2"]
}

resource "test_object" "a" {
  for_each = var.set
}
`,
				"main.tf": `
locals {
  tuple = [
    {
      moduleKey = 0
      resourceKey = "resKey1"
      id = "val1"
    },
    {
      moduleKey = 0
      resourceKey = "resKey2"
      id = "val2"
    },
    {
      moduleKey = 1
      resourceKey = "resKey1"
      id = "val3"
    },
    {
      moduleKey = 1
      resourceKey = "resKey2"
      id = "val4"
    },
  ]
}

module "mod" {
  count = 2
  source = "./mod"
}

import {
  for_each = local.tuple
  id = each.value.id
  to = module.mod[each.value.moduleKey].test_object.a[each.value.resourceKey]
}
`,
			},
		},
	}

	for _, configuration := range configurations {
		t.Run(configuration.Description, func(t *testing.T) {
			m := testModuleInline(t, configuration.inlineConfiguration)
			p := simpleMockProvider()

			hook := new(MockHook)
			ctx := testContext2(t, &ContextOpts{
				Hooks: []Hook{hook},
				Providers: map[addrs.Provider]providers.Factory{
					addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
				},
			})
			p.ReadResourceResponse = &providers.ReadResourceResponse{
				NewState: cty.ObjectVal(map[string]cty.Value{}),
			}

			p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
				ImportedResources: []providers.ImportedResource{
					{
						TypeName: "test_object",
						State:    cty.ObjectVal(map[string]cty.Value{}),
					},
				},
			}

			plan, diags := ctx.Plan(context.Background(), m, states.NewState(), SimplePlanOpts(plans.NormalMode, testInputValuesUnset(m.Module.Variables)))
			if diags.HasErrors() {
				t.Fatalf("unexpected errors\n%s", diags.Err().Error())
			}

			if len(plan.Changes.Resources) != len(configuration.ImportResults) {
				t.Fatalf("expected %d resource changes in the plan, got %d instead", len(configuration.ImportResults), len(plan.Changes.Resources))
			}

			for _, importResult := range configuration.ImportResults {
				addr := mustResourceInstanceAddr(importResult.ResolvedAddress)

				t.Run(addr.String(), func(t *testing.T) {
					instPlan := plan.Changes.ResourceInstance(addr)
					if instPlan == nil {
						t.Fatalf("no plan for %s at all", addr)
					}

					if got, want := instPlan.Addr, addr; !got.Equal(want) {
						t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
					}
					if got, want := instPlan.PrevRunAddr, addr; !got.Equal(want) {
						t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
					}
					if got, want := instPlan.Action, plans.NoOp; got != want {
						t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
					}
					if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
						t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
					}
					if instPlan.Importing.ID != importResult.ResolvedId {
						t.Errorf("expected import change from \"%s\", got non-import change", importResult.ResolvedId)
					}

					if !hook.PrePlanImportCalled {
						t.Fatalf("PostPlanImport hook not called")
					}

					if !hook.PostPlanImportCalled {
						t.Fatalf("PostPlanImport hook not called")
					}
				})
			}
		})
	}
}

func TestContext2Plan_importWithInvalidForEach(t *testing.T) {
	type TestConfiguration struct {
		Description         string
		expectedError       string
		inlineConfiguration map[string]string
	}
	configurations := []TestConfiguration{
		{
			Description:   "for_each value is null",
			expectedError: "Invalid import id argument: The import ID cannot be null",
			inlineConfiguration: map[string]string{
				"main.tf": `
locals {
  map = {
    "key1" = null
  }
}

resource "test_object" "a" {
  for_each = local.map
}

import {
  for_each = local.map
  to = test_object.a[each.key]
  id = each.value
}
`,
			},
		},
		{
			Description:   "for_each key is null",
			expectedError: "Null value as key: Can't use a null value as a key.",
			inlineConfiguration: map[string]string{
				"main.tf": `
variable "nil" {
	default = null
}

locals {
  map = {
    (var.nil) = "val1"
  }
}

resource "test_object" "a" {
  for_each = local.map
}

import {
  for_each = local.map
  to = test_object.a[each.key]
  id = each.value
}
`,
			},
		},
		{
			Description:   "for_each expression is null",
			expectedError: `Invalid for_each argument: The given "for_each" argument value is unsuitable: the "for_each" argument must be a map, set of strings, or a tuple, and you have provided a value of type dynamic.`,
			inlineConfiguration: map[string]string{
				"main.tf": `
variable "map" {
  default = null
}

resource "test_object" "a" {
}

import {
  for_each = var.map
  to = test_object.a[each.key]
  id = each.value
}
`,
			},
		},
		{
			Description:   "for_each key is unknown",
			expectedError: `Invalid for_each argument: The "for_each" map includes keys derived from resource attributes that cannot be determined until apply, and so OpenTofu cannot determine the full set of keys that will identify the instances of this resource.`,
			inlineConfiguration: map[string]string{
				"main.tf": `
resource "test_object" "reference" {
}

locals {
  map = {
    (test_object.reference.id) = "val1"
  }
}

resource "test_object" "a" {
  count = 1
}

import {
  for_each = local.map
  to = test_object.a[each.key]
  id = each.value
}
`,
			},
		},
		{
			Description:   "for_each value is unknown",
			expectedError: `Invalid import id argument: The import block "id" argument depends on resource attributes that cannot be determined until apply, so OpenTofu cannot plan to import this resource.`,
			inlineConfiguration: map[string]string{
				"main.tf": `
resource "test_object" "reference" {
}

locals {
  map = {
    "key1" = (test_object.reference.id)
  }
}

resource "test_object" "a" {
  count = 1
}

import {
  for_each = local.map
  to = test_object.a[each.key]
  id = each.value
}
`,
			},
		},
		{
			Description:   "for_each expression is unknown",
			expectedError: `Invalid for_each argument: The "for_each" map includes keys derived from resource attributes that cannot be determined until apply, and so OpenTofu cannot determine the full set of keys that will identify the instances of this resource.`,
			inlineConfiguration: map[string]string{
				"main.tf": `
resource "test_object" "reference" {
}

locals {
  map = (test_object.reference.id)
}

resource "test_object" "a" {
  count = 1
}

import {
  for_each = local.map
  to = test_object.a[each.key]
  id = each.value
}
`,
			},
		},
		{
			Description:   "for_each value is sensitive",
			expectedError: "Invalid import id argument: The import ID cannot be sensitive.",
			inlineConfiguration: map[string]string{
				"main.tf": `
locals {
  index = sensitive("foo")
  map = {
    "key1" = local.index
  }
}

resource "test_object" "a" {
  for_each = local.map
}

import {
  for_each = local.map
  to = test_object.a[each.key]
  id = each.value
}
`,
			},
		},
		{
			Description:   "for_each key is sensitive",
			expectedError: "Invalid for_each argument: Sensitive values, or values derived from sensitive values, cannot be used as for_each arguments. If used, the sensitive value could be exposed as a resource instance key.",
			inlineConfiguration: map[string]string{
				"main.tf": `
locals {
  index = sensitive("foo")
  map = {
    (local.index) = "val1"
  }
}

resource "test_object" "a" {
  count = 1
}

import {
  for_each = local.map
  to = test_object.a[each.key]
  id = each.value
}
`,
			},
		},
		{
			Description:   "for_each expression is sensitive",
			expectedError: "Invalid for_each argument: Sensitive values, or values derived from sensitive values, cannot be used as for_each arguments. If used, the sensitive value could be exposed as a resource instance key.",
			inlineConfiguration: map[string]string{
				"main.tf": `
resource "test_object" "reference" {
}

locals {
  map = sensitive({
    "key1" = "val1"
  })
}

resource "test_object" "a" {
  count = 0
}

import {
  for_each = local.map
  to = test_object.a[each.key]
  id = each.value
}
`,
			},
		},
	}

	for _, configuration := range configurations {
		t.Run(configuration.Description, func(t *testing.T) {
			m := testModuleInline(t, configuration.inlineConfiguration)

			providerSchema := &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"test_string": {
						Type:     cty.String,
						Optional: true,
					},
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
			}

			p := &MockProvider{
				GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
					Provider: providers.Schema{Block: providerSchema},
					ResourceTypes: map[string]providers.Schema{
						"test_object": providers.Schema{Block: providerSchema},
					},
				},
			}

			hook := new(MockHook)
			ctx := testContext2(t, &ContextOpts{
				Hooks: []Hook{hook},
				Providers: map[addrs.Provider]providers.Factory{
					addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
				},
			})

			p.ReadResourceResponse = &providers.ReadResourceResponse{
				NewState: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			}

			p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
				testStringVal := req.ProposedNewState.GetAttr("test_string")
				return providers.PlanResourceChangeResponse{
					PlannedState: cty.ObjectVal(map[string]cty.Value{
						"test_string": testStringVal,
						"id":          cty.UnknownVal(cty.String),
					}),
				}
			}

			p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
				ImportedResources: []providers.ImportedResource{
					{
						TypeName: "test_object",
						State: cty.ObjectVal(map[string]cty.Value{
							"test_string": cty.StringVal("foo"),
						}),
					},
				},
			}

			_, diags := ctx.Plan(context.Background(), m, states.NewState(), SimplePlanOpts(plans.NormalMode, testInputValuesUnset(m.Module.Variables)))

			if !diags.HasErrors() {
				t.Fatal("succeeded; want errors")
			}
			if got, want := diags.Err().Error(), configuration.expectedError; !strings.Contains(got, want) {
				t.Fatalf("wrong error:\ngot:  %s\nwant: message containing %q", got, want)
			}
		})
	}
}

func TestContext2Plan_importResourceAlreadyInState(t *testing.T) {
	addr := mustResourceInstanceAddr("test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
  test_string = "foo"
}

import {
  to   = test_object.a
  id   = "123"
}
`,
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})
	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.ObjectVal(map[string]cty.Value{
			"test_string": cty.StringVal("foo"),
		}),
	}
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_object",
				State: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			},
		},
	}

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.a").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"test_string":"foo"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	plan, diags := ctx.Plan(context.Background(), m, state, DefaultPlanOpts)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	t.Run(addr.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addr)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addr)
		}

		if got, want := instPlan.Addr, addr; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addr; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.NoOp; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
		if instPlan.Importing != nil {
			t.Errorf("expected non-import change, got import change %+v", instPlan.Importing)
		}
	})
}

func TestContext2Plan_importResourceUpdate(t *testing.T) {
	addr := mustResourceInstanceAddr("test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
  test_string = "bar"
}

import {
  to   = test_object.a
  id   = "123"
}
`,
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})
	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.ObjectVal(map[string]cty.Value{
			"test_string": cty.StringVal("foo"),
		}),
	}
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_object",
				State: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			},
		},
	}

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	t.Run(addr.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addr)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addr)
		}

		if got, want := instPlan.Addr, addr; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addr; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.Update; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
		if instPlan.Importing.ID != "123" {
			t.Errorf("expected import change from \"123\", got non-import change")
		}
	})
}

func TestContext2Plan_importResourceReplace(t *testing.T) {
	addr := mustResourceInstanceAddr("test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
  test_string = "bar"
}

import {
  to   = test_object.a
  id   = "123"
}
`,
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})
	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.ObjectVal(map[string]cty.Value{
			"test_string": cty.StringVal("foo"),
		}),
	}
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_object",
				State: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			},
		},
	}

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addr,
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	t.Run(addr.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addr)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addr)
		}

		if got, want := instPlan.Addr, addr; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addr; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.DeleteThenCreate; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if instPlan.Importing.ID != "123" {
			t.Errorf("expected import change from \"123\", got non-import change")
		}
	})
}

func TestContext2Plan_importRefreshOnce(t *testing.T) {
	addr := mustResourceInstanceAddr("test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
  test_string = "bar"
}

import {
  to   = test_object.a
  id   = "123"
}
`,
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	readCalled := 0
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		readCalled++
		state, _ := simpleTestSchema().CoerceValue(cty.ObjectVal(map[string]cty.Value{
			"test_string": cty.StringVal("foo"),
		}))

		return providers.ReadResourceResponse{
			NewState: state,
		}
	}

	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_object",
				State: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			},
		},
	}

	_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addr,
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	if readCalled > 1 {
		t.Error("ReadResource called multiple times for import")
	}
}

func TestContext2Plan_importIdVariable(t *testing.T) {
	p := testProvider("aws")
	m := testModule(t, "import-id-variable")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "aws_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("foo"),
				}),
			},
		},
	}

	_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		SetVariables: InputValues{
			"the_id": &InputValue{
				// let var take its default value
				Value: cty.NilVal,
			},
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}
}

func TestContext2Plan_importIdReference(t *testing.T) {
	p := testProvider("aws")
	m := testModule(t, "import-id-reference")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "aws_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("foo"),
				}),
			},
		},
	}

	_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		SetVariables: InputValues{
			"the_id": &InputValue{
				// let var take its default value
				Value: cty.NilVal,
			},
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}
}

func TestContext2Plan_importIdFunc(t *testing.T) {
	p := testProvider("aws")
	m := testModule(t, "import-id-func")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "aws_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("foo"),
				}),
			},
		},
	}

	_, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}
}

func TestContext2Plan_importIdDataSource(t *testing.T) {
	p := testProvider("aws")
	m := testModule(t, "import-id-data-source")

	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"aws_subnet": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
		},
		DataSources: map[string]*configschema.Block{
			"aws_subnet": {
				Attributes: map[string]*configschema.Attribute{
					"vpc_id": {
						Type:     cty.String,
						Required: true,
					},
					"cidr_block": {
						Type:     cty.String,
						Computed: true,
					},
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
		},
	})
	p.ReadDataSourceResponse = &providers.ReadDataSourceResponse{
		State: cty.ObjectVal(map[string]cty.Value{
			"vpc_id":     cty.StringVal("abc"),
			"cidr_block": cty.StringVal("10.0.1.0/24"),
			"id":         cty.StringVal("123"),
		}),
	}
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "aws_subnet",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("foo"),
				}),
			},
		},
	}
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}
}

func TestContext2Plan_importIdModule(t *testing.T) {
	p := testProvider("aws")
	m := testModule(t, "import-id-module")

	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"aws_lb": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
		},
	})
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "aws_lb",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("foo"),
				}),
			},
		},
	}
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}
}

func TestContext2Plan_importIdInvalidNull(t *testing.T) {
	p := testProvider("test")
	m := testModule(t, "import-id-invalid-null")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		SetVariables: InputValues{
			"the_id": &InputValue{
				Value: cty.NullVal(cty.String),
			},
		},
	})
	if !diags.HasErrors() {
		t.Fatal("succeeded; want errors")
	}
	if got, want := diags.Err().Error(), "The import ID cannot be null"; !strings.Contains(got, want) {
		t.Fatalf("wrong error:\ngot:  %s\nwant: message containing %q", got, want)
	}
}

func TestContext2Plan_importIdInvalidUnknown(t *testing.T) {
	p := testProvider("test")
	m := testModule(t, "import-id-invalid-unknown")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"test_resource": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
		},
	})
	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
		return providers.PlanResourceChangeResponse{
			PlannedState: cty.UnknownVal(cty.Object(map[string]cty.Type{
				"id": cty.String,
			})),
		}
	}
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_resource",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("foo"),
				}),
			},
		},
	}

	_, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
	if !diags.HasErrors() {
		t.Fatal("succeeded; want errors")
	}
	if got, want := diags.Err().Error(), `The import block "id" argument depends on resource attributes that cannot be determined until apply`; !strings.Contains(got, want) {
		t.Fatalf("wrong error:\ngot:  %s\nwant: message containing %q", got, want)
	}
}

func TestContext2Plan_importIntoModuleWithGeneratedConfig(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
import {
  to = test_object.a
  id = "123"
}

import {
  to = module.mod.test_object.a
  id = "456"
}

module "mod" {
  source = "./mod"
}
`,
		"./mod/main.tf": `
resource "test_object" "a" {
  test_string = "bar"
}
`,
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})
	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.ObjectVal(map[string]cty.Value{
			"test_string": cty.StringVal("foo"),
		}),
	}
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_object",
				State: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			},
		},
	}

	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_object",
				State: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			},
		},
	}

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode:               plans.NormalMode,
		GenerateConfigPath: "generated.tf", // Actual value here doesn't matter, as long as it is not empty.
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	one := mustResourceInstanceAddr("test_object.a")
	two := mustResourceInstanceAddr("module.mod.test_object.a")

	onePlan := plan.Changes.ResourceInstance(one)
	twoPlan := plan.Changes.ResourceInstance(two)

	// This test is just to make sure things work e2e with modules and generated
	// config, so we're not too careful about the actual responses - we're just
	// happy nothing panicked. See the other import tests for actual validation
	// of responses and the like.
	if twoPlan.Action != plans.Update {
		t.Errorf("expected nested item to be updated but was %s", twoPlan.Action)
	}

	if len(onePlan.GeneratedConfig) == 0 {
		t.Errorf("expected root item to generate config but it didn't")
	}
}

func TestContext2Plan_importIntoNonExistentConfiguration(t *testing.T) {
	type TestConfiguration struct {
		Description         string
		inlineConfiguration map[string]string
	}
	configurations := []TestConfiguration{
		{
			Description: "Basic missing configuration",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = test_object.a
  id   = "123"
}
`,
			},
		},
		{
			Description: "Non-existent module",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = module.mod.test_object.a
  id   = "456"
}
`,
			},
		},
		{
			Description: "Wrong module key",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = module.mod["non-existent"].test_object.a
  id   = "123"
}

module "mod" {
  for_each = {
    existent = "1"
  }
  source = "./mod"
}
`,
				"./mod/main.tf": `
resource "test_object" "a" {
  test_string = "bar"
}
`,
			},
		},
		{
			Description: "Module key without for_each",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = module.mod["non-existent"].test_object.a
  id   = "123"
}

module "mod" {
  source = "./mod"
}
`,
				"./mod/main.tf": `
resource "test_object" "a" {
  test_string = "bar"
}
`,
			},
		},
		{
			Description: "Non-existent resource key - in module",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = module.mod.test_object.a["non-existent"]
  id   = "123"
}

module "mod" {
  source = "./mod"
}
`,
				"./mod/main.tf": `
resource "test_object" "a" {
  for_each = {
    existent = "1"
  }
  test_string = "bar"
}
`,
			},
		},
		{
			Description: "Non-existent resource key - in root",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = test_object.a[42]
  id   = "123"
}

resource "test_object" "a" {
  test_string = "bar"
}
`,
			},
		},
		{
			Description: "Existent module key, non-existent resource key",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = module.mod["existent"].test_object.b
  id   = "123"
}

module "mod" {
  for_each = {
    existent = "1"
    existent_two = "2"
  }
  source = "./mod"
}
`,
				"./mod/main.tf": `
resource "test_object" "a" {
  test_string = "bar"
}
`,
			},
		},
	}

	for _, configuration := range configurations {
		t.Run(configuration.Description, func(t *testing.T) {
			m := testModuleInline(t, configuration.inlineConfiguration)

			p := simpleMockProvider()
			ctx := testContext2(t, &ContextOpts{
				Providers: map[addrs.Provider]providers.Factory{
					addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
				},
			})

			_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
				Mode: plans.NormalMode,
			})

			if !diags.HasErrors() {
				t.Fatalf("expected error")
			}

			var errNum int
			for _, diag := range diags {
				if diag.Severity() == tfdiags.Error {
					errNum++
				}
			}
			if errNum > 1 {
				t.Fatalf("expected a single error, but got %d", errNum)
			}

			if !strings.Contains(diags.Err().Error(), "Configuration for import target does not exist") {
				t.Fatalf("expected error to be \"Configuration for import target does not exist\", but it was %s", diags.Err().Error())
			}
		})
	}
}

func TestContext2Plan_importDuplication(t *testing.T) {
	type TestConfiguration struct {
		Description         string
		inlineConfiguration map[string]string
		expectedError       string
	}
	configurations := []TestConfiguration{
		{
			Description: "Duplication with dynamic address with a variable",
			inlineConfiguration: map[string]string{
				"main.tf": `
resource "test_object" "a" {
 count = 2
}

variable "address1" {
 default = 1
}

variable "address2" {
 default = 1
}

import {
  to   = test_object.a[var.address1]
  id   = "123"
}

import {
  to   = test_object.a[var.address2]
  id   = "123"
}
`,
			},
			expectedError: "Duplicate import configuration for \"test_object.a[1]\"",
		},
		{
			Description: "Duplication with dynamic address with a resource reference",
			inlineConfiguration: map[string]string{
				"main.tf": `
resource "test_object" "example" {
 test_string = "boop"
}

resource "test_object" "a" {
 for_each = toset(["boop"])
}

import {
  to   = test_object.a[test_object.example.test_string]
  id   = "123"
}

import {
  to   = test_object.a[test_object.example.test_string]
  id   = "123"
}
`,
			},
			expectedError: "Duplicate import configuration for \"test_object.a[\\\"boop\\\"]\"",
		},
	}

	for _, configuration := range configurations {
		t.Run(configuration.Description, func(t *testing.T) {
			m := testModuleInline(t, configuration.inlineConfiguration)

			p := simpleMockProvider()
			ctx := testContext2(t, &ContextOpts{
				Providers: map[addrs.Provider]providers.Factory{
					addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
				},
			})

			p.ReadResourceResponse = &providers.ReadResourceResponse{
				NewState: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			}

			p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
				ImportedResources: []providers.ImportedResource{
					{
						TypeName: "test_object",
						State: cty.ObjectVal(map[string]cty.Value{
							"test_string": cty.StringVal("foo"),
						}),
					},
				},
			}

			_, diags := ctx.Plan(context.Background(), m, states.NewState(), SimplePlanOpts(plans.NormalMode, testInputValuesUnset(m.Module.Variables)))

			if !diags.HasErrors() {
				t.Fatalf("expected error")
			}

			var errNum int
			for _, diag := range diags {
				if diag.Severity() == tfdiags.Error {
					errNum++
				}
			}
			if errNum > 1 {
				t.Fatalf("expected a single error, but got %d", errNum)
			}

			if !strings.Contains(diags.Err().Error(), configuration.expectedError) {
				t.Fatalf("expected error to be %s, but it was %s", configuration.expectedError, diags.Err().Error())
			}
		})
	}
}

func TestContext2Plan_importResourceConfigGen(t *testing.T) {
	addr := mustResourceInstanceAddr("test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
import {
  to   = test_object.a
  id   = "123"
}
`,
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})
	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.ObjectVal(map[string]cty.Value{
			"test_string": cty.StringVal("foo"),
		}),
	}
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_object",
				State: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			},
		},
	}

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode:               plans.NormalMode,
		GenerateConfigPath: "generated.tf", // Actual value here doesn't matter, as long as it is not empty.
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	t.Run(addr.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addr)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addr)
		}

		if got, want := instPlan.Addr, addr; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addr; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.NoOp; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
		if instPlan.Importing.ID != "123" {
			t.Errorf("expected import change from \"123\", got non-import change")
		}

		want := `resource "test_object" "a" {
  test_bool   = null
  test_list   = null
  test_map    = null
  test_number = null
  test_string = "foo"
}`
		got := instPlan.GeneratedConfig
		if diff := cmp.Diff(want, got); len(diff) > 0 {
			t.Errorf("got:\n%s\nwant:\n%s\ndiff:\n%s", got, want, diff)
		}
	})
}

func TestContext2Plan_importResourceConfigGenWithAlias(t *testing.T) {
	addr := mustResourceInstanceAddr("test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
provider "test" {
  alias = "backup"
}

import {
  provider = test.backup
  to       = test_object.a
  id       = "123"
}
`,
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})
	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.ObjectVal(map[string]cty.Value{
			"test_string": cty.StringVal("foo"),
		}),
	}
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_object",
				State: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			},
		},
	}

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode:               plans.NormalMode,
		GenerateConfigPath: "generated.tf", // Actual value here doesn't matter, as long as it is not empty.
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	t.Run(addr.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addr)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addr)
		}

		if got, want := instPlan.Addr, addr; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addr; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.NoOp; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
		if instPlan.Importing.ID != "123" {
			t.Errorf("expected import change from \"123\", got non-import change")
		}

		want := `resource "test_object" "a" {
  provider    = test.backup
  test_bool   = null
  test_list   = null
  test_map    = null
  test_number = null
  test_string = "foo"
}`
		got := instPlan.GeneratedConfig
		if diff := cmp.Diff(want, got); len(diff) > 0 {
			t.Errorf("got:\n%s\nwant:\n%s\ndiff:\n%s", got, want, diff)
		}
	})
}

func TestContext2Plan_importResourceConfigGenValidation(t *testing.T) {
	type TestConfiguration struct {
		Description         string
		inlineConfiguration map[string]string
		expectedError       string
	}
	configurations := []TestConfiguration{
		{
			Description: "Resource with index",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = test_object.a[0]
  id   = "123"
}
`,
			},
			expectedError: "Configuration generation for count and for_each resources not supported",
		},
		{
			Description: "Resource with dynamic index",
			inlineConfiguration: map[string]string{
				"main.tf": `
locals {
  loc = "something"
}

import {
  to   = test_object.a[local.loc]
  id   = "123"
}
`,
			},
			expectedError: "Configuration generation for count and for_each resources not supported",
		},
		{
			Description: "Resource in module",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = module.mod.test_object.b
  id   = "456"
}

module "mod" {
  source = "./mod"
}


`,
				"./mod/main.tf": `
resource "test_object" "a" {
  test_string = "bar"
}
`,
			},
			expectedError: "Cannot generate configuration for resource inside sub-module",
		},
		{
			Description: "Resource in non-existent module",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = module.mod.test_object.a
  id   = "456"
}
`,
			},
			expectedError: "Cannot generate configuration for resource inside sub-module",
		},
		{
			Description: "Wrong module key",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = module.mod["non-existent"].test_object.a
  id   = "123"
}

module "mod" {
  for_each = {
    existent = "1"
  }
  source = "./mod"
}
`,
				"./mod/main.tf": `
resource "test_object" "a" {
  test_string = "bar"
}
`,
			},
			expectedError: "Configuration for import target does not exist",
		},
		{
			Description: "In module with module key",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = module.mod["existent"].test_object.b
  id   = "123"
}

module "mod" {
  for_each = {
    existent = "1"
  }
  source = "./mod"
}
`,
				"./mod/main.tf": `
resource "test_object" "a" {
  test_string = "bar"
}
`,
			},
			expectedError: "Cannot generate configuration for resource inside sub-module",
		},
		{
			Description: "Module key without for_each",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = module.mod["non-existent"].test_object.a
  id   = "123"
}

module "mod" {
  source = "./mod"
}
`,
				"./mod/main.tf": `
resource "test_object" "a" {
  test_string = "bar"
}
`,
			},
			expectedError: "Configuration for import target does not exist",
		},
		{
			Description: "Non-existent resource key - in module",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = module.mod.test_object.a["non-existent"]
  id   = "123"
}

module "mod" {
  source = "./mod"
}
`,
				"./mod/main.tf": `
resource "test_object" "a" {
  for_each = {
    existent = "1"
  }
  test_string = "bar"
}
`,
			},
			expectedError: "Configuration for import target does not exist",
		},
		{
			Description: "Non-existent resource key - in root",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = test_object.a[42]
  id   = "123"
}

resource "test_object" "a" {
  test_string = "bar"
}
`,
			},
			expectedError: "Configuration for import target does not exist",
		},
		{
			Description: "Existent module key, non-existent resource key",
			inlineConfiguration: map[string]string{
				"main.tf": `
import {
  to   = module.mod["existent"].test_object.b
  id   = "123"
}

module "mod" {
  for_each = {
    existent = "1"
    existent_two = "2"
  }
  source = "./mod"
}
`,
				"./mod/main.tf": `
resource "test_object" "a" {
  test_string = "bar"
}
`,
			},
			expectedError: "Cannot generate configuration for resource inside sub-module",
		},
	}

	for _, configuration := range configurations {
		t.Run(configuration.Description, func(t *testing.T) {
			m := testModuleInline(t, configuration.inlineConfiguration)

			p := simpleMockProvider()
			ctx := testContext2(t, &ContextOpts{
				Providers: map[addrs.Provider]providers.Factory{
					addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
				},
			})

			_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
				Mode:               plans.NormalMode,
				GenerateConfigPath: "generated.tf",
			})

			if !diags.HasErrors() {
				t.Fatalf("expected error")
			}

			var errNum int
			for _, diag := range diags {
				if diag.Severity() == tfdiags.Error {
					errNum++
				}
			}
			if errNum > 1 {
				t.Fatalf("expected a single error, but got %d", errNum)
			}

			if !strings.Contains(diags.Err().Error(), configuration.expectedError) {
				t.Fatalf("expected error to be %s, but it was %s", configuration.expectedError, diags.Err().Error())
			}
		})
	}
}

func TestContext2Plan_importResourceConfigGenExpandedResource(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
import {
  to       = test_object.a[0]
  id       = "123"
}
`,
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})
	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.ObjectVal(map[string]cty.Value{
			"test_string": cty.StringVal("foo"),
		}),
	}
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_object",
				State: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			},
		},
	}

	_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode:               plans.NormalMode,
		GenerateConfigPath: "generated.tf",
	})
	if !diags.HasErrors() {
		t.Fatalf("expected plan to error, but it did not")
	}
	if !strings.Contains(diags.Err().Error(), "Configuration generation for count and for_each resources not supported") {
		t.Fatalf("expected error to be \"Config generation for count and for_each resources not supported\", but it is %s", diags.Err().Error())
	}
}

// config generation still succeeds even when planning fails
func TestContext2Plan_importResourceConfigGenWithError(t *testing.T) {
	addr := mustResourceInstanceAddr("test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
import {
  to   = test_object.a
  id   = "123"
}
`,
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})
	p.PlanResourceChangeResponse = &providers.PlanResourceChangeResponse{
		PlannedState: cty.NullVal(cty.DynamicPseudoType),
		Diagnostics:  tfdiags.Diagnostics(nil).Append(errors.New("plan failed")),
	}
	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.ObjectVal(map[string]cty.Value{
			"test_string": cty.StringVal("foo"),
		}),
	}
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_object",
				State: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			},
		},
	}

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode:               plans.NormalMode,
		GenerateConfigPath: "generated.tf", // Actual value here doesn't matter, as long as it is not empty.
	})
	if !diags.HasErrors() {
		t.Fatal("expected error")
	}

	instPlan := plan.Changes.ResourceInstance(addr)
	if instPlan == nil {
		t.Fatalf("no plan for %s at all", addr)
	}

	want := `resource "test_object" "a" {
  test_bool   = null
  test_list   = null
  test_map    = null
  test_number = null
  test_string = "foo"
}`
	got := instPlan.GeneratedConfig
	if diff := cmp.Diff(want, got); len(diff) > 0 {
		t.Errorf("got:\n%s\nwant:\n%s\ndiff:\n%s", got, want, diff)
	}
}

func TestContext2Plan_providerForEachWithOrphanResourceInstanceNotUsingForEach(t *testing.T) {
	// This test is to cover the bug reported in this issue:
	//    https://github.com/opentofu/opentofu/issues/2334
	//
	// The bug there was that OpenTofu was failing to evaluate the provider
	// instance key expression for a graph node representing a "orphaned"
	// resource instance, and was thus always treating them as belonging to
	// the no-key instance of the given provider. Since a for_each provider
	// has no instance without an instance key, that caused an error trying
	// to use a non-existent provider instance.

	instAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test_thing",
		Name: "a",
	}.Instance(addrs.StringKey("orphaned")).Absolute(addrs.RootModuleInstance)
	providerConfigAddr := addrs.AbsProviderConfig{
		Module:   addrs.RootModule,
		Provider: addrs.NewBuiltInProvider("test"),
		Alias:    "multi",
	}
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			terraform {
				required_providers {
					test = {
						source = "terraform.io/builtin/test"
					}
				}
			}

			provider "test" {
				alias    = "multi"
				for_each = toset(["a"])
			}

			resource "test_thing" "a" {
				for_each = toset([])
				provider = test.multi["a"]
			}
		`,
	})
	s := states.BuildState(func(ss *states.SyncState) {
		ss.SetResourceInstanceCurrent(
			instAddr,
			&states.ResourceInstanceObjectSrc{
				Status:    states.ObjectReady,
				AttrsJSON: []byte(`{}`),
			},
			providerConfigAddr,
			addrs.NoKey, // NOTE: The prior state object is associated with a no-key instance of provider.test
		)
	})
	p := &MockProvider{}
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{
			Block: &configschema.Block{},
		},
		ResourceTypes: map[string]providers.Schema{
			"test_thing": {
				Block: &configschema.Block{},
			},
		},
	}

	tofuCtx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			providerConfigAddr.Provider: testProviderFuncFixed(p),
		},
	})
	_, diags := tofuCtx.Plan(context.Background(), m, s, DefaultPlanOpts)
	err := diags.Err()
	if err == nil {
		t.Fatalf("unexpected success; want an error about %s not being declared", providerConfigAddr.InstanceString(addrs.NoKey))
	}
	got := err.Error()
	wantSubstring := `To proceed, return to your previous single-instance configuration for provider["terraform.io/builtin/test"].multi and ensure that test_thing.a["orphaned"] has been destroyed or forgotten before using for_each with this provider, or change the resource configuration to still declare an instance with the key ["orphaned"].`
	if !strings.Contains(got, wantSubstring) {
		t.Errorf("missing expected error message\ngot:\n%s\nwant substring: %s", got, wantSubstring)
	}
}

func TestContext2Plan_plannedState(t *testing.T) {
	addr := mustResourceInstanceAddr("test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
	test_string = "foo"
}

locals {
  local_value = test_object.a.test_string
}
`,
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	state := states.NewState()
	plan, diags := ctx.Plan(context.Background(), m, state, nil)
	if diags.HasErrors() {
		t.Errorf("expected no errors, but got %s", diags)
	}

	module := state.RootModule()

	// So, the original state shouldn't have been updated at all.
	if len(module.LocalValues) > 0 {
		t.Errorf("expected no local values in the state but found %d", len(module.LocalValues))
	}

	if len(module.Resources) > 0 {
		t.Errorf("expected no resources in the state but found %d", len(module.LocalValues))
	}

	// But, this makes it hard for the testing framework to valid things about
	// the returned plan. So, the plan contains the planned state:
	module = plan.PlannedState.RootModule()

	if module.LocalValues["local_value"].AsString() != "foo" {
		t.Errorf("expected local value to be \"foo\" but was \"%s\"", module.LocalValues["local_value"].AsString())
	}

	if module.ResourceInstance(addr.Resource).Current.Status != states.ObjectPlanned {
		t.Errorf("expected resource to be in planned state")
	}
}

func TestContext2Plan_removedResourceBasic(t *testing.T) {
	desposedKey := states.DeposedKey("deposed")
	addr := mustResourceInstanceAddr("test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			removed {
				from = test_object.a
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		// The prior state tracks test_object.a, which we should be
		// removed from the state by the "removed" block in the config.
		s.SetResourceInstanceCurrent(addr, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
		s.SetResourceInstanceDeposed(
			mustResourceInstanceAddr(addr.String()),
			desposedKey,
			&states.ResourceInstanceObjectSrc{
				Status:       states.ObjectTainted,
				AttrsJSON:    []byte(`{"test_string":"old"}`),
				Dependencies: []addrs.ConfigResource{},
			},
			mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
			addrs.NoKey,
		)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addr,
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	for _, test := range []struct {
		deposedKey states.DeposedKey
		wantReason plans.ResourceInstanceChangeActionReason
	}{{desposedKey, plans.ResourceInstanceChangeNoReason}, {states.NotDeposed, plans.ResourceInstanceDeleteBecauseNoResourceConfig}} {
		t.Run(addr.String(), func(t *testing.T) {
			var instPlan *plans.ResourceInstanceChangeSrc

			if test.deposedKey == states.NotDeposed {
				instPlan = plan.Changes.ResourceInstance(addr)
			} else {
				instPlan = plan.Changes.ResourceInstanceDeposed(addr, test.deposedKey)
			}

			if instPlan == nil {
				t.Fatalf("no plan for %s at all", addr)
			}

			if got, want := instPlan.Addr, addr; !got.Equal(want) {
				t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.PrevRunAddr, addr; !got.Equal(want) {
				t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.Action, plans.Forget; got != want {
				t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.ActionReason, test.wantReason; got != want {
				t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
			}
		})
	}
}

func TestContext2Plan_removedModuleBasic(t *testing.T) {
	desposedKey := states.DeposedKey("deposed")
	addr := mustResourceInstanceAddr("module.mod.test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			removed {
				from = module.mod
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		// The prior state tracks module.mod.test_object.a, which should be
		// removed from the state by the module's "removed" block in the root module config.
		s.SetResourceInstanceCurrent(addr, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
		s.SetResourceInstanceDeposed(
			mustResourceInstanceAddr(addr.String()),
			desposedKey,
			&states.ResourceInstanceObjectSrc{
				Status:       states.ObjectTainted,
				AttrsJSON:    []byte(`{"test_string":"old"}`),
				Dependencies: []addrs.ConfigResource{},
			},
			mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
			addrs.NoKey,
		)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addr,
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	for _, test := range []struct {
		deposedKey states.DeposedKey
		wantReason plans.ResourceInstanceChangeActionReason
	}{{desposedKey, plans.ResourceInstanceChangeNoReason}, {states.NotDeposed, plans.ResourceInstanceDeleteBecauseNoResourceConfig}} {
		t.Run(addr.String(), func(t *testing.T) {
			var instPlan *plans.ResourceInstanceChangeSrc

			if test.deposedKey == states.NotDeposed {
				instPlan = plan.Changes.ResourceInstance(addr)
			} else {
				instPlan = plan.Changes.ResourceInstanceDeposed(addr, test.deposedKey)
			}

			if instPlan == nil {
				t.Fatalf("no plan for %s at all", addr)
			}

			if got, want := instPlan.Addr, addr; !got.Equal(want) {
				t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.PrevRunAddr, addr; !got.Equal(want) {
				t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.Action, plans.Forget; got != want {
				t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.ActionReason, test.wantReason; got != want {
				t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
			}
		})
	}
}

func TestContext2Plan_removedModuleForgetsAllInstances(t *testing.T) {
	addrFirst := mustResourceInstanceAddr("module.mod[0].test_object.a")
	addrSecond := mustResourceInstanceAddr("module.mod[1].test_object.a")

	m := testModuleInline(t, map[string]string{
		"main.tf": `
			removed {
				from = module.mod
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		// The prior state tracks module.mod[0].test_object.a and
		// module.mod[1].test_object.a, which we should be removed
		// from the state by the "removed" block in the config.
		s.SetResourceInstanceCurrent(addrFirst, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
		s.SetResourceInstanceCurrent(addrSecond, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addrFirst, addrSecond,
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	for _, resourceInstance := range []addrs.AbsResourceInstance{addrFirst, addrSecond} {
		t.Run(resourceInstance.String(), func(t *testing.T) {
			instPlan := plan.Changes.ResourceInstance(resourceInstance)
			if instPlan == nil {
				t.Fatalf("no plan for %s at all", resourceInstance)
			}

			if got, want := instPlan.Addr, resourceInstance; !got.Equal(want) {
				t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.PrevRunAddr, resourceInstance; !got.Equal(want) {
				t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.Action, plans.Forget; got != want {
				t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.ActionReason, plans.ResourceInstanceDeleteBecauseNoResourceConfig; got != want {
				t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
			}
		})
	}
}

func TestContext2Plan_removedResourceForgetsAllInstances(t *testing.T) {
	addrFirst := mustResourceInstanceAddr("test_object.a[0]")
	addrSecond := mustResourceInstanceAddr("test_object.a[1]")

	m := testModuleInline(t, map[string]string{
		"main.tf": `
			removed {
				from = test_object.a
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		// The prior state tracks test_object.a[0] and
		// test_object.a[1], which we should be removed from
		// the state by the "removed" block in the config.
		s.SetResourceInstanceCurrent(addrFirst, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
		s.SetResourceInstanceCurrent(addrSecond, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addrFirst, addrSecond,
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	for _, resourceInstance := range []addrs.AbsResourceInstance{addrFirst, addrSecond} {
		t.Run(resourceInstance.String(), func(t *testing.T) {
			instPlan := plan.Changes.ResourceInstance(resourceInstance)
			if instPlan == nil {
				t.Fatalf("no plan for %s at all", resourceInstance)
			}

			if got, want := instPlan.Addr, resourceInstance; !got.Equal(want) {
				t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.PrevRunAddr, resourceInstance; !got.Equal(want) {
				t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.Action, plans.Forget; got != want {
				t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := instPlan.ActionReason, plans.ResourceInstanceDeleteBecauseNoResourceConfig; got != want {
				t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
			}
		})
	}
}

func TestContext2Plan_removedResourceInChildModuleFromParentModule(t *testing.T) {
	addr := mustResourceInstanceAddr("module.mod.test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			module "mod" {
				  source = "./mod"
			}

			removed {
				from = module.mod.test_object.a
			}
		`,
		"mod/main.tf": ``,
	})

	state := states.BuildState(func(s *states.SyncState) {
		// The prior state tracks module.mod.test_object.a.a, which we should be
		// removed from the state by the "removed" block in the root config.
		s.SetResourceInstanceCurrent(addr, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addr,
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	t.Run(addr.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addr)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addr)
		}

		if got, want := instPlan.Addr, addr; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addr; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.Forget; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceDeleteBecauseNoResourceConfig; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})
}

func TestContext2Plan_removedResourceInChildModuleFromChildModule(t *testing.T) {
	addr := mustResourceInstanceAddr("module.mod.test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			module "mod" {
				  source = "./mod"
			}
		`,
		"mod/main.tf": `
			removed {
				from = test_object.a
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		// The prior state tracks module.mod.test_object.a.a, which we should be
		// removed from the state by the "removed" block in the child module config.
		s.SetResourceInstanceCurrent(addr, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addr,
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	t.Run(addr.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addr)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addr)
		}

		if got, want := instPlan.Addr, addr; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addr; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.Forget; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceDeleteBecauseNoResourceConfig; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})
}

func TestContext2Plan_removedResourceInGrandchildModuleFromRootModule(t *testing.T) {
	addr := mustResourceInstanceAddr("module.child.module.grandchild.test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			module "child" {
				  source = "./child"
			}

			removed {
				from = module.child.module.grandchild.test_object.a
			}
		`,
		"child/main.tf": ``,
	})

	state := states.BuildState(func(s *states.SyncState) {
		// The prior state tracks module.child.module.grandchild.test_object.a,
		// which we should be removed from the state by the "removed" block in
		// the root config.
		s.SetResourceInstanceCurrent(addr, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addr,
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	t.Run(addr.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addr)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addr)
		}

		if got, want := instPlan.Addr, addr; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addr; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.Forget; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceDeleteBecauseNoResourceConfig; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})
}

func TestContext2Plan_removedChildModuleForgetsResourceInGrandchildModule(t *testing.T) {
	addr := mustResourceInstanceAddr("module.child.module.grandchild.test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			module "child" {
				  source = "./child"
			}

			removed {
				from = module.child.module.grandchild
			}
		`,
		"child/main.tf": ``,
	})

	state := states.BuildState(func(s *states.SyncState) {
		// The prior state tracks module.child.module.grandchild.test_object.a,
		// which we should be removed from the state by the "removed" block
		// in the root config.
		s.SetResourceInstanceCurrent(addr, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addr,
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	t.Run(addr.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addr)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addr)
		}

		if got, want := instPlan.Addr, addr; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addr; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.Forget; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceDeleteBecauseNoResourceConfig; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})
}

func TestContext2Plan_movedAndRemovedResourceAtTheSameTime(t *testing.T) {
	// This is the only scenario where the "moved" and "removed" blocks can
	// coexist while referencing the same resource. In this case, the "moved" logic
	// will run first, trying to move the resource to a non-existing target.
	// Usually ,it will cause the resource to be destroyed, but because the
	// "removed" block is also present, it will be removed from the state instead.
	addrA := mustResourceInstanceAddr("test_object.a")
	addrB := mustResourceInstanceAddr("test_object.b")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			removed {
				from = test_object.b
			}

			moved {
				from = test_object.a
				to   = test_object.b
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		// The prior state tracks test_object.a, which we should treat as
		// test_object.b because of the "moved" block in the config.
		s.SetResourceInstanceCurrent(addrA, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addrA,
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	t.Run(addrA.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addrA)
		if instPlan != nil {
			t.Fatalf("unexpected plan for %s; should've moved to %s", addrA, addrB)
		}
	})
	t.Run(addrB.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addrB)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addrB)
		}

		if got, want := instPlan.Addr, addrB; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addrA; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.Forget; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceDeleteBecauseNoMoveTarget; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
	})
}

func TestContext2Plan_removedResourceButResourceBlockStillExists(t *testing.T) {
	addr := mustResourceInstanceAddr("test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			resource "test_object" "a" {
				test_string = "foo"
			}

			removed {
				from = test_object.a
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addr, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addr,
		},
	})

	if !diags.HasErrors() {
		t.Fatal("succeeded; want errors")
	}

	if got, want := diags.Err().Error(), "Removed resource block still exists"; !strings.Contains(got, want) {
		t.Fatalf("wrong error:\ngot:  %s\nwant: message containing %q", got, want)
	}
}

func TestContext2Plan_removedResourceButResourceBlockStillExistsInChildModule(t *testing.T) {
	addr := mustResourceInstanceAddr("module.mod.test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			module "mod" {
				source = "./mod"
			}

			removed {
				from = module.mod.test_object.a
			}
		`,
		"mod/main.tf": `
			resource "test_object" "a" {
				test_string = "foo"
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addr, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addr,
		},
	})

	if !diags.HasErrors() {
		t.Fatal("succeeded; want errors")
	}

	if got, want := diags.Err().Error(), "Removed resource block still exists"; !strings.Contains(got, want) {
		t.Fatalf("wrong error:\ngot:  %s\nwant: message containing %q", got, want)
	}
}

func TestContext2Plan_removedModuleButModuleBlockStillExists(t *testing.T) {
	addr := mustResourceInstanceAddr("module.mod.test_object.a")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			module "mod" {
				source = "./mod"
			}

			removed {
				from = module.mod
			}
		`,
		"mod/main.tf": `
			resource "test_object" "a" {
				test_string = "foo"
			}
		`,
	})

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addr, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	p := simpleMockProvider()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		ForceReplace: []addrs.AbsResourceInstance{
			addr,
		},
	})

	if !diags.HasErrors() {
		t.Fatal("succeeded; want errors")
	}

	if got, want := diags.Err().Error(), "Removed module block still exists"; !strings.Contains(got, want) {
		t.Fatalf("wrong error:\ngot:  %s\nwant: message containing %q", got, want)
	}
}

func TestContext2Plan_importResourceWithSensitiveDataSource(t *testing.T) {
	addr := mustResourceInstanceAddr("test_object.b")
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			data "test_data_source" "a" {
			}
			resource "test_object" "b" {
				test_string = data.test_data_source.a.test_string
			}
			import {
			  to   = test_object.b
			  id   = "123"
			}
		`,
	})

	p := &MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			Provider: providers.Schema{Block: simpleTestSchema()},
			ResourceTypes: map[string]providers.Schema{
				"test_object": {Block: simpleTestSchema()},
			},
			DataSources: map[string]providers.Schema{
				"test_data_source": {Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"test_string": {
							Type:      cty.String,
							Computed:  true,
							Sensitive: true,
						},
					},
				}},
			},
		},
	}
	hook := new(MockHook)
	ctx := testContext2(t, &ContextOpts{
		Hooks: []Hook{hook},
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	p.ReadDataSourceResponse = &providers.ReadDataSourceResponse{
		State: cty.ObjectVal(map[string]cty.Value{
			"test_string": cty.StringVal("foo"),
		}),
	}

	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.ObjectVal(map[string]cty.Value{
			"test_string": cty.StringVal("foo"),
		}),
	}
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_object",
				State: cty.ObjectVal(map[string]cty.Value{
					"test_string": cty.StringVal("foo"),
				}),
			},
		},
	}

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}

	t.Run(addr.String(), func(t *testing.T) {
		instPlan := plan.Changes.ResourceInstance(addr)
		if instPlan == nil {
			t.Fatalf("no plan for %s at all", addr)
		}

		if got, want := instPlan.Addr, addr; !got.Equal(want) {
			t.Errorf("wrong current address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.PrevRunAddr, addr; !got.Equal(want) {
			t.Errorf("wrong previous run address\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.Action, plans.NoOp; got != want {
			t.Errorf("wrong planned action\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := instPlan.ActionReason, plans.ResourceInstanceChangeNoReason; got != want {
			t.Errorf("wrong action reason\ngot:  %s\nwant: %s", got, want)
		}
		if instPlan.Importing.ID != "123" {
			t.Errorf("expected import change from \"123\", got non-import change")
		}

		if !hook.PrePlanImportCalled {
			t.Fatalf("PostPlanImport hook not called")
		}
		if addr, wantAddr := hook.PrePlanImportAddr, instPlan.Addr; !addr.Equal(wantAddr) {
			t.Errorf("expected addr to be %s, but was %s", wantAddr, addr)
		}

		if !hook.PostPlanImportCalled {
			t.Fatalf("PostPlanImport hook not called")
		}
		if addr, wantAddr := hook.PostPlanImportAddr, instPlan.Addr; !addr.Equal(wantAddr) {
			t.Errorf("expected addr to be %s, but was %s", wantAddr, addr)
		}
	})
}

func TestContext2Plan_insufficient_block(t *testing.T) {
	type testcase struct {
		filename string
		start    hcl.Pos
		end      hcl.Pos
	}

	tests := map[string]testcase{
		"insufficient-features-blocks-aliased-provider": {
			filename: "provider[\"registry.opentofu.org/hashicorp/test\"] with no configuration",
			start:    hcl.InitialPos,
			end:      hcl.InitialPos,
		},
		"insufficient-features-blocks-nested_module": {
			filename: "provider[\"registry.opentofu.org/hashicorp/test\"] with no configuration",
			start:    hcl.InitialPos,
			end:      hcl.InitialPos,
		},
		"insufficient-features-blocks-no-feats": {
			filename: "testdata/insufficient-features-blocks-no-feats/main.tf",
			start:    hcl.Pos{Line: 9, Column: 17, Byte: 146},
			end:      hcl.Pos{Line: 9, Column: 17, Byte: 146},
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			m := testModule(t, testName)
			p := mockProviderWithFeaturesBlock()

			ctx := testContext2(t, &ContextOpts{
				Providers: map[addrs.Provider]providers.Factory{
					addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
				},
			})

			_, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
			var expectedDiags tfdiags.Diagnostics

			expectedDiags = expectedDiags.Append(
				&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Insufficient features blocks",
					Detail:   "At least 1 \"features\" blocks are required.",
					Subject: &hcl.Range{
						Filename: tc.filename,
						Start:    tc.start,
						End:      tc.end,
					},
				},
			)

			assertDiagnosticsMatch(t, diags, expectedDiags)
		})
	}
}

func mockProviderWithFeaturesBlock() *MockProvider {
	return &MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			Provider: providers.Schema{Block: featuresBlockTestSchema()},
			ResourceTypes: map[string]providers.Schema{
				"test_object": {Block: simpleTestSchema()},
			},
		},
	}
}

func featuresBlockTestSchema() *configschema.Block {
	return &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"test_string": {
				Type:     cty.String,
				Optional: true,
			},
		},
		BlockTypes: map[string]*configschema.NestedBlock{
			"features": {
				MinItems: 1,
				MaxItems: 1,
				Nesting:  configschema.NestingList,
			},
		},
	}
}
