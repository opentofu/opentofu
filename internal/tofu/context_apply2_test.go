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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/version"
)

// Test that the PreApply hook is called with the correct deposed key
func TestContext2Apply_createBeforeDestroy_deposedKeyPreApply(t *testing.T) {
	m := testModule(t, "apply-cbd-deposed-only")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn

	deposedKey := states.NewDeposedKey()

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.bar").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"bar"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceDeposed(
		mustResourceInstanceAddr("aws_instance.bar").Resource,
		deposedKey,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectTainted,
			AttrsJSON: []byte(`{"id":"foo"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)

	hook := new(MockHook)
	ctx := testContext2(t, &ContextOpts{
		Hooks: []Hook{hook},
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, DefaultPlanOpts)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	} else {
		t.Logf("%s", legacyDiffComparisonString(plan.Changes))
	}

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	// Verify PreApply was called correctly
	if !hook.PreApplyCalled {
		t.Fatalf("PreApply hook not called")
	}
	if addr, wantAddr := hook.PreApplyAddr, mustResourceInstanceAddr("aws_instance.bar"); !addr.Equal(wantAddr) {
		t.Errorf("expected addr to be %s, but was %s", wantAddr, addr)
	}
	if gen := hook.PreApplyGen; gen != deposedKey {
		t.Errorf("expected gen to be %q, but was %q", deposedKey, gen)
	}
}

// This tests that when a CBD (C) resource depends on a non-CBD (B) resource that depends on another CBD resource (A)
// Check that create_before_destroy is still set on the B resource after only the B resource is updated
func TestContext2Apply_createBeforeDestroy_dependsNonCBDUpdate(t *testing.T) {
	m := testModule(t, "apply-cbd-depends-non-cbd-update")
	p := simpleMockProvider()

	// Set plan resource change to replace on test_number change
	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
		return providers.PlanResourceChangeResponse{
			PlannedState:    req.ProposedNewState,
			RequiresReplace: []cty.Path{cty.GetAttrPath("test_number")},
		}
	}
	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.A").Resource,
		&states.ResourceInstanceObjectSrc{
			Status: states.ObjectReady,
			AttrsJSON: []byte(`
			{
				"test_string": "A"
			}`),
			CreateBeforeDestroy: true,
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.B").Resource,
		&states.ResourceInstanceObjectSrc{
			Status: states.ObjectReady,
			AttrsJSON: []byte(`
			{
				"test_number": 0,
				"test_string": "A"
			}`),
			Dependencies: []addrs.ConfigResource{mustConfigResourceAddr("test_object.A")},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.C").Resource,
		&states.ResourceInstanceObjectSrc{
			Status: states.ObjectReady,
			AttrsJSON: []byte(`
			{
				"test_string": "C"
			}`),
			Dependencies:        []addrs.ConfigResource{mustConfigResourceAddr("test_object.A"), mustConfigResourceAddr("terraform_data.B")},
			CreateBeforeDestroy: true,
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
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	} else {
		t.Log(legacyDiffComparisonString(plan.Changes))
	}

	state, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	// Check that create_before_destroy was set on the foo resource
	foo := state.RootModule().Resources["test_object.B"].Instances[addrs.NoKey].Current
	if !foo.CreateBeforeDestroy {
		t.Fatalf("B resource should have create_before_destroy set")
	}
}

func TestContext2Apply_destroyWithDataSourceExpansion(t *testing.T) {
	// While managed resources store their destroy-time dependencies, data
	// sources do not. This means that if a provider were only included in a
	// destroy graph because of data sources, it could have dependencies which
	// are not correctly ordered. Here we verify that the provider is not
	// included in the destroy operation, and all dependency evaluations
	// succeed.

	m := testModuleInline(t, map[string]string{
		"main.tf": `
module "mod" {
  source = "./mod"
}

provider "other" {
  foo = module.mod.data
}

# this should not require the provider be present during destroy
data "other_data_source" "a" {
}
`,

		"mod/main.tf": `
data "test_data_source" "a" {
  count = 1
}

data "test_data_source" "b" {
  count = data.test_data_source.a[0].foo == "ok" ? 1 : 0
}

output "data" {
  value = data.test_data_source.a[0].foo == "ok" ? data.test_data_source.b[0].foo : "nope"
}
`,
	})

	testP := testProvider("test")
	otherP := testProvider("other")

	readData := func(req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
		return providers.ReadDataSourceResponse{
			State: cty.ObjectVal(map[string]cty.Value{
				"id":  cty.StringVal("data_source"),
				"foo": cty.StringVal("ok"),
			}),
		}
	}

	testP.ReadDataSourceFn = readData
	otherP.ReadDataSourceFn = readData

	ps := map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("test"):  testProviderFuncFixed(testP),
		addrs.NewDefaultProvider("other"): testProviderFuncFixed(otherP),
	}

	otherP.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) (resp providers.ConfigureProviderResponse) {
		foo := req.Config.GetAttr("foo")
		if foo.IsNull() || foo.AsString() != "ok" {
			resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("incorrect config val: %#v\n", foo))
		}
		return resp
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: ps,
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// now destroy the whole thing
	ctx = testContext2(t, &ContextOpts{
		Providers: ps,
	})

	plan, diags = ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.DestroyMode,
	})
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	otherP.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) (resp providers.ConfigureProviderResponse) {
		// should not be used to destroy data sources
		resp.Diagnostics = resp.Diagnostics.Append(errors.New("provider should not be used"))
		return resp
	}

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
}

func TestContext2Apply_destroyThenUpdate(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_instance" "a" {
	value = "updated"
}
`,
	})

	p := testProvider("test")
	p.PlanResourceChangeFn = testDiffFn

	var orderMu sync.Mutex
	var order []string
	p.ApplyResourceChangeFn = func(req providers.ApplyResourceChangeRequest) (resp providers.ApplyResourceChangeResponse) {
		id := req.PriorState.GetAttr("id").AsString()
		if id == "b" {
			// slow down the b destroy, since a should wait for it
			time.Sleep(100 * time.Millisecond)
		}

		orderMu.Lock()
		order = append(order, id)
		orderMu.Unlock()

		resp.NewState = req.PlannedState
		return resp
	}

	addrA := mustResourceInstanceAddr(`test_instance.a`)
	addrB := mustResourceInstanceAddr(`test_instance.b`)

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(addrA, &states.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{"id":"a","value":"old","type":"test"}`),
			Status:    states.ObjectReady,
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)

		// test_instance.b depended on test_instance.a, and therefor should be
		// destroyed before any changes to test_instance.a
		s.SetResourceInstanceCurrent(addrB, &states.ResourceInstanceObjectSrc{
			AttrsJSON:    []byte(`{"id":"b"}`),
			Status:       states.ObjectReady,
			Dependencies: []addrs.ConfigResource{addrA.ContainingResource().Config()},
		}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey)
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, DefaultPlanOpts)
	assertNoErrors(t, diags)

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	if order[0] != "b" {
		t.Fatalf("expected apply order [b, a], got: %v\n", order)
	}
}

// verify that dependencies are updated in the state during refresh and apply
func TestApply_updateDependencies(t *testing.T) {
	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)

	fooAddr := mustResourceInstanceAddr("aws_instance.foo")
	barAddr := mustResourceInstanceAddr("aws_instance.bar")
	bazAddr := mustResourceInstanceAddr("aws_instance.baz")
	bamAddr := mustResourceInstanceAddr("aws_instance.bam")
	binAddr := mustResourceInstanceAddr("aws_instance.bin")
	root.SetResourceInstanceCurrent(
		fooAddr.Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"foo"}`),
			Dependencies: []addrs.ConfigResource{
				bazAddr.ContainingResource().Config(),
			},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		binAddr.Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"bin","type":"aws_instance","unknown":"ok"}`),
			Dependencies: []addrs.ConfigResource{
				bazAddr.ContainingResource().Config(),
			},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		bazAddr.Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"baz"}`),
			Dependencies: []addrs.ConfigResource{
				// Existing dependencies should not be removed from orphaned instances
				bamAddr.ContainingResource().Config(),
			},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		barAddr.Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"bar","foo":"foo"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)

	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "aws_instance" "bar" {
  foo = aws_instance.foo.id
}

resource "aws_instance" "foo" {
}

resource "aws_instance" "bin" {
}
`,
	})

	p := testProvider("aws")

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, DefaultPlanOpts)
	assertNoErrors(t, diags)

	bar := plan.PriorState.ResourceInstance(barAddr)
	if len(bar.Current.Dependencies) == 0 || !bar.Current.Dependencies[0].Equal(fooAddr.ContainingResource().Config()) {
		t.Fatalf("bar should depend on foo after refresh, but got %s", bar.Current.Dependencies)
	}

	foo := plan.PriorState.ResourceInstance(fooAddr)
	if len(foo.Current.Dependencies) == 0 || !foo.Current.Dependencies[0].Equal(bazAddr.ContainingResource().Config()) {
		t.Fatalf("foo should depend on baz after refresh because of the update, but got %s", foo.Current.Dependencies)
	}

	bin := plan.PriorState.ResourceInstance(binAddr)
	if len(bin.Current.Dependencies) != 0 {
		t.Fatalf("bin should depend on nothing after refresh because there is no change, but got %s", bin.Current.Dependencies)
	}

	baz := plan.PriorState.ResourceInstance(bazAddr)
	if len(baz.Current.Dependencies) == 0 || !baz.Current.Dependencies[0].Equal(bamAddr.ContainingResource().Config()) {
		t.Fatalf("baz should depend on bam after refresh, but got %s", baz.Current.Dependencies)
	}

	state, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	bar = state.ResourceInstance(barAddr)
	if len(bar.Current.Dependencies) == 0 || !bar.Current.Dependencies[0].Equal(fooAddr.ContainingResource().Config()) {
		t.Fatalf("bar should still depend on foo after apply, but got %s", bar.Current.Dependencies)
	}

	foo = state.ResourceInstance(fooAddr)
	if len(foo.Current.Dependencies) != 0 {
		t.Fatalf("foo should have no deps after apply, but got %s", foo.Current.Dependencies)
	}

}

func TestContext2Apply_additionalSensitiveFromState(t *testing.T) {
	// Ensure we're not trying to double-mark values decoded from state
	m := testModuleInline(t, map[string]string{
		"main.tf": `
variable "secret" {
  sensitive = true
  default = ["secret"]
}

resource "test_resource" "a" {
  sensitive_attr = var.secret
}

resource "test_resource" "b" {
  value = test_resource.a.id
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
					"value": {
						Type:     cty.String,
						Optional: true,
					},
					"sensitive_attr": {
						Type:      cty.List(cty.String),
						Optional:  true,
						Sensitive: true,
					},
				},
			},
		},
	})

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			mustResourceInstanceAddr(`test_resource.a`),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"a","sensitive_attr":["secret"]}`),
				AttrSensitivePaths: []cty.PathValueMarks{
					{
						Path:  cty.GetAttrPath("sensitive_attr"),
						Marks: cty.NewValueMarks(marks.Sensitive),
					},
				},
				Status: states.ObjectReady,
			}, mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`), addrs.NoKey,
		)
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, SimplePlanOpts(plans.NormalMode, testInputValuesUnset(m.Module.Variables)))
	assertNoErrors(t, diags)

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatal(diags.ErrWithWarnings())
	}
}

func TestContext2Apply_sensitiveOutputPassthrough(t *testing.T) {
	// Ensure we're not trying to double-mark values decoded from state
	m := testModuleInline(t, map[string]string{
		"main.tf": `
module "mod" {
  source = "./mod"
}

resource "test_object" "a" {
  test_string = module.mod.out
}
`,

		"mod/main.tf": `
variable "in" {
  sensitive = true
  default = "foo"
}
output "out" {
  value = var.in
}
`,
	})

	p := simpleMockProvider()

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatal(diags.ErrWithWarnings())
	}

	obj := state.ResourceInstance(mustResourceInstanceAddr("test_object.a"))
	if len(obj.Current.AttrSensitivePaths) != 1 {
		t.Fatalf("Expected 1 sensitive mark for test_object.a, got %#v\n", obj.Current.AttrSensitivePaths)
	}

	plan, diags = ctx.Plan(context.Background(), m, state, DefaultPlanOpts)
	assertNoErrors(t, diags)

	// make sure the same marks are compared in the next plan as well
	for _, c := range plan.Changes.Resources {
		if c.Action != plans.NoOp {
			t.Errorf("Unexpcetd %s change for %s", c.Action, c.Addr)
		}
	}
}

func TestContext2Apply_ignoreImpureFunctionChanges(t *testing.T) {
	// The impure function call should not cause a planned change with
	// ignore_changes
	m := testModuleInline(t, map[string]string{
		"main.tf": `
variable "pw" {
  sensitive = true
  default = "foo"
}

resource "test_object" "x" {
  test_map = {
	string = "X${bcrypt(var.pw)}"
  }
  lifecycle {
    ignore_changes = [ test_map["string"] ]
  }
}

resource "test_object" "y" {
  test_map = {
	string = "X${bcrypt(var.pw)}"
  }
  lifecycle {
    ignore_changes = [ test_map ]
  }
}

`,
	})

	p := simpleMockProvider()

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), SimplePlanOpts(plans.NormalMode, testInputValuesUnset(m.Module.Variables)))
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)

	// FINAL PLAN:
	plan, diags = ctx.Plan(context.Background(), m, state, SimplePlanOpts(plans.NormalMode, testInputValuesUnset(m.Module.Variables)))
	assertNoErrors(t, diags)

	// make sure the same marks are compared in the next plan as well
	for _, c := range plan.Changes.Resources {
		if c.Action != plans.NoOp {
			t.Logf("marks before: %#v", c.BeforeValMarks)
			t.Logf("marks after:  %#v", c.AfterValMarks)
			t.Errorf("Unexpcetd %s change for %s", c.Action, c.Addr)
		}
	}
}

func TestContext2Apply_destroyWithDeposed(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "x" {
  test_string = "ok"
  lifecycle {
    create_before_destroy = true
  }
}`,
	})

	p := simpleMockProvider()

	deposedKey := states.NewDeposedKey()

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceDeposed(
		mustResourceInstanceAddr("test_object.x").Resource,
		deposedKey,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectTainted,
			AttrsJSON: []byte(`{"test_string":"deposed"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.DestroyMode,
	})
	if diags.HasErrors() {
		t.Fatalf("plan: %s", diags.Err())
	}

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("apply: %s", diags.Err())
	}

}

// This test is a copy and paste from TestContext2Apply_destroyWithDeposed
// with modifications to test the same scenario with a dynamic provider instance.
func TestContext2Apply_destroyWithDeposedWithDynamicProvider(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
provider "test" {
  alias = "for_eached"
  for_each = {a: {}}
}

resource "test_object" "x" {
  test_string = "ok"
  lifecycle {
    create_before_destroy = true
  }
  provider = test.for_eached["a"]
}`,
	})

	p := simpleMockProvider()

	deposedKey := states.NewDeposedKey()

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceDeposed(
		mustResourceInstanceAddr("test_object.x").Resource,
		deposedKey,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectTainted,
			AttrsJSON: []byte(`{"test_string":"deposed"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"].for_eached`),
		addrs.StringKey("a"),
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.DestroyMode,
	})
	if diags.HasErrors() {
		t.Fatalf("plan: %s", diags.Err())
	}

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("apply: %s", diags.Err())
	}
}

func TestContext2Apply_nullableVariables(t *testing.T) {
	m := testModule(t, "apply-nullable-variables")
	state := states.NewState()
	ctx := testContext2(t, &ContextOpts{})
	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{})
	if diags.HasErrors() {
		t.Fatalf("plan: %s", diags.Err())
	}
	state, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("apply: %s", diags.Err())
	}

	outputs := state.Module(addrs.RootModuleInstance).OutputValues
	// we check for null outputs be seeing that they don't exists
	if _, ok := outputs["nullable_null_default"]; ok {
		t.Error("nullable_null_default: expected no output value")
	}
	if _, ok := outputs["nullable_non_null_default"]; ok {
		t.Error("nullable_non_null_default: expected no output value")
	}
	if _, ok := outputs["nullable_no_default"]; ok {
		t.Error("nullable_no_default: expected no output value")
	}

	if v := outputs["non_nullable_default"].Value; v.AsString() != "ok" {
		t.Fatalf("incorrect 'non_nullable_default' output value: %#v\n", v)
	}
	if v := outputs["non_nullable_no_default"].Value; v.AsString() != "ok" {
		t.Fatalf("incorrect 'non_nullable_no_default' output value: %#v\n", v)
	}
}

func TestContext2Apply_targetedDestroyWithMoved(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
module "modb" {
  source = "./mod"
  for_each = toset(["a", "b"])
}
`,
		"./mod/main.tf": `
resource "test_object" "a" {
}

module "sub" {
  for_each = toset(["a", "b"])
  source = "./sub"
}

moved {
  from = module.old
  to = module.sub
}
`,
		"./mod/sub/main.tf": `
resource "test_object" "s" {
}
`})

	p := simpleMockProvider()

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)

	// destroy only a single instance not included in the moved statements
	_, diags = ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode:    plans.DestroyMode,
		Targets: []addrs.Targetable{mustResourceInstanceAddr(`module.modb["a"].test_object.a`)},
	})
	assertNoErrors(t, diags)
}

// This test is inspired by the above test TestContext2Apply_targetedDestroyWithMoved
func TestContext2Apply_excludedDestroyWithMoved(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
module "modb" {
  source = "./mod"
  for_each = toset(["a", "b"])
}
`,
		"./mod/main.tf": `
resource "test_object" "a" {
}

module "sub" {
  for_each = toset(["a", "b"])
  source = "./sub"
}

moved {
  from = module.old
  to = module.sub
}
`,
		"./mod/sub/main.tf": `
resource "test_object" "s" {
}
`})

	p := simpleMockProvider()

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)

	// destroy excluding the module in the moved statements
	_, diags = ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode:     plans.DestroyMode,
		Excludes: []addrs.Targetable{addrs.Module{"sub"}},
	})
	assertNoErrors(t, diags)
}

func TestContext2Apply_graphError(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "a" {
  test_string = "ok"
}

resource "test_object" "b" {
  test_string = test_object.a.test_string
}
`,
	})

	p := simpleMockProvider()

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.a").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectTainted,
			AttrsJSON: []byte(`{"test_string":"ok"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.b").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectTainted,
			AttrsJSON: []byte(`{"test_string":"ok"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.DestroyMode,
	})
	if diags.HasErrors() {
		t.Fatalf("plan: %s", diags.Err())
	}

	// We're going to corrupt the stored state so that the dependencies will
	// cause a cycle when building the apply graph.
	testObjA := plan.PriorState.Modules[""].Resources["test_object.a"].Instances[addrs.NoKey].Current
	testObjA.Dependencies = append(testObjA.Dependencies, mustResourceInstanceAddr("test_object.b").ContainingResource().Config())

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	if !diags.HasErrors() {
		t.Fatal("expected cycle error from apply")
	}
}

func TestContext2Apply_resourcePostcondition(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
variable "boop" {
  type = string
}

resource "test_resource" "a" {
	value = var.boop
}

resource "test_resource" "b" {
  value = test_resource.a.output
  lifecycle {
    postcondition {
      condition     = self.output != ""
      error_message = "Output must not be blank."
    }
  }
}

resource "test_resource" "c" {
  value = test_resource.b.output
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
	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
		m := req.ProposedNewState.AsValueMap()
		m["output"] = cty.UnknownVal(cty.String)

		resp.PlannedState = cty.ObjectVal(m)
		resp.LegacyTypeSystem = true
		return resp
	}
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
		if len(plan.Changes.Resources) != 3 {
			t.Fatalf("unexpected plan changes: %#v", plan.Changes)
		}

		p.ApplyResourceChangeFn = func(req providers.ApplyResourceChangeRequest) (resp providers.ApplyResourceChangeResponse) {
			m := req.PlannedState.AsValueMap()
			m["output"] = cty.StringVal(fmt.Sprintf("new-%s", m["value"].AsString()))

			resp.NewState = cty.ObjectVal(m)
			return resp
		}
		state, diags := ctx.Apply(context.Background(), plan, m, nil)
		assertNoErrors(t, diags)

		wantResourceAttrs := map[string]struct{ value, output string }{
			"a": {"boop", "new-boop"},
			"b": {"new-boop", "new-new-boop"},
			"c": {"new-new-boop", "new-new-new-boop"},
		}
		for name, attrs := range wantResourceAttrs {
			addr := mustResourceInstanceAddr(fmt.Sprintf("test_resource.%s", name))
			r := state.ResourceInstance(addr)
			rd, err := r.Current.Decode(cty.Object(map[string]cty.Type{
				"value":  cty.String,
				"output": cty.String,
			}))
			if err != nil {
				t.Fatalf("error decoding test_resource.a: %s", err)
			}
			want := cty.ObjectVal(map[string]cty.Value{
				"value":  cty.StringVal(attrs.value),
				"output": cty.StringVal(attrs.output),
			})
			if !cmp.Equal(want, rd.Value, valueComparer) {
				t.Errorf("wrong attrs for %s\n%s", addr, cmp.Diff(want, rd.Value, valueComparer))
			}
		}
	})
	t.Run("condition fail", func(t *testing.T) {
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
		if len(plan.Changes.Resources) != 3 {
			t.Fatalf("unexpected plan changes: %#v", plan.Changes)
		}

		p.ApplyResourceChangeFn = func(req providers.ApplyResourceChangeRequest) (resp providers.ApplyResourceChangeResponse) {
			m := req.PlannedState.AsValueMap()

			// For the resource with a constraint, fudge the output to make the
			// condition fail.
			if value := m["value"].AsString(); value == "new-boop" {
				m["output"] = cty.StringVal("")
			} else {
				m["output"] = cty.StringVal(fmt.Sprintf("new-%s", value))
			}

			resp.NewState = cty.ObjectVal(m)
			return resp
		}
		state, diags := ctx.Apply(context.Background(), plan, m, nil)
		if !diags.HasErrors() {
			t.Fatal("succeeded; want errors")
		}
		if got, want := diags.Err().Error(), "Resource postcondition failed: Output must not be blank."; got != want {
			t.Fatalf("wrong error:\ngot:  %s\nwant: %q", got, want)
		}

		// Resources a and b should still be recorded in state
		wantResourceAttrs := map[string]struct{ value, output string }{
			"a": {"boop", "new-boop"},
			"b": {"new-boop", ""},
		}
		for name, attrs := range wantResourceAttrs {
			addr := mustResourceInstanceAddr(fmt.Sprintf("test_resource.%s", name))
			r := state.ResourceInstance(addr)
			rd, err := r.Current.Decode(cty.Object(map[string]cty.Type{
				"value":  cty.String,
				"output": cty.String,
			}))
			if err != nil {
				t.Fatalf("error decoding test_resource.a: %s", err)
			}
			want := cty.ObjectVal(map[string]cty.Value{
				"value":  cty.StringVal(attrs.value),
				"output": cty.StringVal(attrs.output),
			})
			if !cmp.Equal(want, rd.Value, valueComparer) {
				t.Errorf("wrong attrs for %s\n%s", addr, cmp.Diff(want, rd.Value, valueComparer))
			}
		}

		// Resource c should not be in state
		if state.ResourceInstance(mustResourceInstanceAddr("test_resource.c")) != nil {
			t.Error("test_resource.c should not exist in state, but is")
		}
	})
}

func TestContext2Apply_outputValuePrecondition(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			variable "input" {
				type = string
			}

			module "child" {
				source = "./child"

				input = var.input
			}

			output "result" {
				value = module.child.result

				precondition {
					condition     = var.input != ""
					error_message = "Input must not be empty."
				}
			}
		`,
		"child/main.tf": `
			variable "input" {
				type = string
			}

			output "result" {
				value = var.input

				precondition {
					condition     = var.input != ""
					error_message = "Input must not be empty."
				}
			}
		`,
	})

	checkableObjects := []addrs.Checkable{
		addrs.OutputValue{Name: "result"}.Absolute(addrs.RootModuleInstance),
		addrs.OutputValue{Name: "result"}.Absolute(addrs.RootModuleInstance.Child("child", addrs.NoKey)),
	}

	t.Run("pass", func(t *testing.T) {
		ctx := testContext2(t, &ContextOpts{})
		plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"input": &InputValue{
					Value:      cty.StringVal("beep"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		assertNoDiagnostics(t, diags)

		for _, addr := range checkableObjects {
			result := plan.Checks.GetObjectResult(addr)
			if result == nil {
				t.Fatalf("no check result for %s in the plan", addr)
			}
			if got, want := result.Status, checks.StatusPass; got != want {
				t.Fatalf("wrong check status for %s during planning\ngot:  %s\nwant: %s", addr, got, want)
			}
		}

		state, diags := ctx.Apply(context.Background(), plan, m, nil)
		assertNoDiagnostics(t, diags)
		for _, addr := range checkableObjects {
			result := state.CheckResults.GetObjectResult(addr)
			if result == nil {
				t.Fatalf("no check result for %s in the final state", addr)
			}
			if got, want := result.Status, checks.StatusPass; got != want {
				t.Errorf("wrong check status for %s after apply\ngot:  %s\nwant: %s", addr, got, want)
			}
		}
	})

	t.Run("fail", func(t *testing.T) {
		// NOTE: This test actually catches a failure during planning and so
		// cannot proceed to apply, so it's really more of a plan test
		// than an apply test but better to keep all of these
		// thematically-related test cases together.
		ctx := testContext2(t, &ContextOpts{})
		_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"input": &InputValue{
					Value:      cty.StringVal(""),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		if !diags.HasErrors() {
			t.Fatalf("succeeded; want error")
		}

		const wantSummary = "Module output value precondition failed"
		found := false
		for _, diag := range diags {
			if diag.Severity() == tfdiags.Error && diag.Description().Summary == wantSummary {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("missing expected error\nwant summary: %s\ngot: %s", wantSummary, diags.Err().Error())
		}
	})
}

func TestContext2Apply_resourceConditionApplyTimeFail(t *testing.T) {
	// This tests the less common situation where a condition fails due to
	// a change in a resource other than the one the condition is attached to,
	// and the condition result is unknown during planning.
	//
	// This edge case is a tricky one because it relies on OpenTofu still
	// visiting test_resource.b (in the configuration below) to evaluate
	// its conditions even though there aren't any changes directly planned
	// for it, so that we can consider whether changes to test_resource.a
	// have changed the outcome.

	m := testModuleInline(t, map[string]string{
		"main.tf": `
			variable "input" {
				type = string
			}

			resource "test_resource" "a" {
				value = var.input
			}

			resource "test_resource" "b" {
				value = "beep"

				lifecycle {
					postcondition {
						condition     = test_resource.a.output == self.output
						error_message = "Outputs must match."
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
	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
		// Whenever "value" changes, "output" follows it during the apply step,
		// but is initially unknown during the plan step.

		m := req.ProposedNewState.AsValueMap()
		priorVal := cty.NullVal(cty.String)
		if !req.PriorState.IsNull() {
			priorVal = req.PriorState.GetAttr("value")
		}
		if m["output"].IsNull() || !priorVal.RawEquals(m["value"]) {
			m["output"] = cty.UnknownVal(cty.String)
		}

		resp.PlannedState = cty.ObjectVal(m)
		resp.LegacyTypeSystem = true
		return resp
	}
	p.ApplyResourceChangeFn = func(req providers.ApplyResourceChangeRequest) (resp providers.ApplyResourceChangeResponse) {
		m := req.PlannedState.AsValueMap()
		m["output"] = m["value"]
		resp.NewState = cty.ObjectVal(m)
		return resp
	}
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})
	instA := mustResourceInstanceAddr("test_resource.a")
	instB := mustResourceInstanceAddr("test_resource.b")

	// Preparation: an initial plan and apply with a correct input variable
	// should succeed and give us a valid and complete state to use for the
	// subsequent plan and apply that we'll expect to fail.
	var prevRunState *states.State
	{
		plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"input": &InputValue{
					Value:      cty.StringVal("beep"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		assertNoErrors(t, diags)
		planA := plan.Changes.ResourceInstance(instA)
		if planA == nil || planA.Action != plans.Create {
			t.Fatalf("incorrect initial plan for instance A\nwant a 'create' change\ngot: %s", spew.Sdump(planA))
		}
		planB := plan.Changes.ResourceInstance(instB)
		if planB == nil || planB.Action != plans.Create {
			t.Fatalf("incorrect initial plan for instance B\nwant a 'create' change\ngot: %s", spew.Sdump(planB))
		}

		state, diags := ctx.Apply(context.Background(), plan, m, nil)
		assertNoErrors(t, diags)

		stateA := state.ResourceInstance(instA)
		if stateA == nil || stateA.Current == nil || !bytes.Contains(stateA.Current.AttrsJSON, []byte(`"beep"`)) {
			t.Fatalf("incorrect initial state for instance A\ngot: %s", spew.Sdump(stateA))
		}
		stateB := state.ResourceInstance(instB)
		if stateB == nil || stateB.Current == nil || !bytes.Contains(stateB.Current.AttrsJSON, []byte(`"beep"`)) {
			t.Fatalf("incorrect initial state for instance B\ngot: %s", spew.Sdump(stateB))
		}
		prevRunState = state
	}

	// Now we'll run another plan and apply with a different value for
	// var.input that should cause the test_resource.b condition to be unknown
	// during planning and then fail during apply.
	{
		plan, diags := ctx.Plan(context.Background(), m, prevRunState, &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"input": &InputValue{
					Value:      cty.StringVal("boop"), // NOTE: This has changed
					SourceType: ValueFromCLIArg,
				},
			},
		})
		assertNoErrors(t, diags)
		planA := plan.Changes.ResourceInstance(instA)
		if planA == nil || planA.Action != plans.Update {
			t.Fatalf("incorrect initial plan for instance A\nwant an 'update' change\ngot: %s", spew.Sdump(planA))
		}
		planB := plan.Changes.ResourceInstance(instB)
		if planB == nil || planB.Action != plans.NoOp {
			t.Fatalf("incorrect initial plan for instance B\nwant a 'no-op' change\ngot: %s", spew.Sdump(planB))
		}

		_, diags = ctx.Apply(context.Background(), plan, m, nil)
		if !diags.HasErrors() {
			t.Fatal("final apply succeeded, but should've failed with a postcondition error")
		}
		if len(diags) != 1 {
			t.Fatalf("expected exactly one diagnostic, but got: %s", diags.Err().Error())
		}
		if got, want := diags[0].Description().Summary, "Resource postcondition failed"; got != want {
			t.Fatalf("wrong diagnostic summary\ngot:  %s\nwant: %s", got, want)
		}
	}
}

// pass an input through some expanded values, and back to a provider to make
// sure we can fully evaluate a provider configuration during a destroy plan.
func TestContext2Apply_destroyWithConfiguredProvider(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
variable "in" {
  type = map(string)
  default = {
    "a" = "first"
    "b" = "second"
  }
}

module "mod" {
  source = "./mod"
  for_each = var.in
  in = each.value
}

locals {
  config = [for each in module.mod : each.out]
}

provider "other" {
  output = [for each in module.mod : each.out]
  local = local.config
  var = var.in
}

resource "other_object" "other" {
}
`,
		"./mod/main.tf": `
variable "in" {
  type = string
}

data "test_object" "d" {
  test_string = var.in
}

resource "test_object" "a" {
  test_string = var.in
}

output "out" {
  value = data.test_object.d.output
}
`})

	testProvider := &MockProvider{
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
							"output": {
								Type:     cty.String,
								Computed: true,
							},
						},
					},
				},
			},
		},
	}

	testProvider.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) (resp providers.ReadDataSourceResponse) {
		cfg := req.Config.AsValueMap()
		s := cfg["test_string"].AsString()
		if !strings.Contains("firstsecond", s) {
			resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("expected 'first' or 'second', got %s", s))
			return resp
		}

		cfg["output"] = cty.StringVal(s + "-ok")
		resp.State = cty.ObjectVal(cfg)
		return resp
	}

	otherProvider := &MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			Provider: providers.Schema{
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"output": {
							Type:     cty.List(cty.String),
							Optional: true,
						},
						"local": {
							Type:     cty.List(cty.String),
							Optional: true,
						},
						"var": {
							Type:     cty.Map(cty.String),
							Optional: true,
						},
					},
				},
			},
			ResourceTypes: map[string]providers.Schema{
				"other_object": providers.Schema{Block: simpleTestSchema()},
			},
		},
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"):  testProviderFuncFixed(testProvider),
			addrs.NewDefaultProvider("other"): testProviderFuncFixed(otherProvider),
		},
	})

	opts := SimplePlanOpts(plans.NormalMode, testInputValuesUnset(m.Module.Variables))
	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), opts)
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)

	// Resource changes which have dependencies across providers which
	// themselves depend on resources can result in cycles.
	// Because other_object transitively depends on the module resources
	// through its provider, we trigger changes on both sides of this boundary
	// to ensure we can create a valid plan.
	//
	// Taint the object to make sure a replacement works in the plan.
	otherObjAddr := mustResourceInstanceAddr("other_object.other")
	otherObj := state.ResourceInstance(otherObjAddr)
	otherObj.Current.Status = states.ObjectTainted
	// Force a change which needs to be reverted.
	testObjAddr := mustResourceInstanceAddr(`module.mod["a"].test_object.a`)
	testObjA := state.ResourceInstance(testObjAddr)
	testObjA.Current.AttrsJSON = []byte(`{"test_bool":null,"test_list":null,"test_map":null,"test_number":null,"test_string":"changed"}`)

	_, diags = ctx.Plan(context.Background(), m, state, opts)
	assertNoErrors(t, diags)
	// TODO: unreachable code
	if 1 == 0 {
		otherProvider.ConfigureProviderCalled = false
		otherProvider.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) (resp providers.ConfigureProviderResponse) {
			// check that our config is complete, even during a destroy plan
			expected := cty.ObjectVal(map[string]cty.Value{
				"local":  cty.ListVal([]cty.Value{cty.StringVal("first-ok"), cty.StringVal("second-ok")}),
				"output": cty.ListVal([]cty.Value{cty.StringVal("first-ok"), cty.StringVal("second-ok")}),
				"var": cty.MapVal(map[string]cty.Value{
					"a": cty.StringVal("first"),
					"b": cty.StringVal("second"),
				}),
			})

			if !req.Config.RawEquals(expected) {
				resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf(
					`incorrect provider config:
expected: %#v
got:      %#v`,
					expected, req.Config))
			}

			return resp
		}

		opts.Mode = plans.DestroyMode
		// skip refresh so that we don't configure the provider before the destroy plan
		opts.SkipRefresh = true

		// destroy only a single instance not included in the moved statements
		_, diags = ctx.Plan(context.Background(), m, state, opts)
		assertNoErrors(t, diags)

		if !otherProvider.ConfigureProviderCalled {
			t.Fatal("failed to configure provider during destroy plan")
		}
	}
}

// check that a provider can verify a planned destroy
func TestContext2Apply_plannedDestroy(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "x" {
  test_string = "ok"
}`,
	})

	p := simpleMockProvider()
	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
		if !req.ProposedNewState.IsNull() {
			// we should only be destroying in this test
			resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("unexpected plan with %#v", req.ProposedNewState))
			return resp
		}

		resp.PlannedState = req.ProposedNewState
		// we're going to verify the destroy plan by inserting private data required for destroy
		resp.PlannedPrivate = append(resp.PlannedPrivate, []byte("planned")...)
		return resp
	}

	p.ApplyResourceChangeFn = func(req providers.ApplyResourceChangeRequest) (resp providers.ApplyResourceChangeResponse) {
		// if the value is nil, we return that directly to correspond to a delete
		if !req.PlannedState.IsNull() {
			resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("unexpected apply with %#v", req.PlannedState))
			return resp
		}

		resp.NewState = req.PlannedState

		// make sure we get our private data from the plan
		private := string(req.PlannedPrivate)
		if private != "planned" {
			resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("missing private data from plan, got %q", private))
		}
		return resp
	}

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.x").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"test_string":"ok"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.DestroyMode,
		// we don't want to refresh, because that actually runs a normal plan
		SkipRefresh: true,
	})
	if diags.HasErrors() {
		t.Fatalf("plan: %s", diags.Err())
	}

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("apply: %s", diags.Err())
	}
}

func TestContext2Apply_missingOrphanedResource(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
# changed resource address to create a new object
resource "test_object" "y" {
  test_string = "y"
}
`,
	})

	p := simpleMockProvider()

	// report the prior value is missing
	p.ReadResourceFn = func(req providers.ReadResourceRequest) (resp providers.ReadResourceResponse) {
		resp.NewState = cty.NullVal(req.PriorState.Type())
		return resp
	}

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.x").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"test_string":"x"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	opts := SimplePlanOpts(plans.NormalMode, nil)
	plan, diags := ctx.Plan(context.Background(), m, state, opts)
	assertNoErrors(t, diags)

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)
}

// Outputs should not cause evaluation errors during destroy
// Check eval from both root level outputs and module outputs, which are
// handled differently during apply.
func TestContext2Apply_outputsNotToEvaluate(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
module "mod" {
  source = "./mod"
  cond = false
}

output "from_resource" {
  value = module.mod.from_resource
}

output "from_data" {
  value = module.mod.from_data
}
`,

		"./mod/main.tf": `
variable "cond" {
  type = bool
}

module "mod" {
  source = "../mod2/"
  cond = var.cond
}

output "from_resource" {
  value = module.mod.resource
}

output "from_data" {
  value = module.mod.data
}
`,

		"./mod2/main.tf": `
variable "cond" {
  type = bool
}

resource "test_object" "x" {
  count = var.cond ? 0:1
}

data "test_object" "d" {
  count = var.cond ? 0:1
}

output "resource" {
  value = var.cond ? null : test_object.x.*.test_string[0]
}

output "data" {
  value = one(data.test_object.d[*].test_string)
}
`})

	p := simpleMockProvider()
	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) (resp providers.ReadDataSourceResponse) {
		resp.State = req.Config
		return resp
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	// apply the state
	opts := SimplePlanOpts(plans.NormalMode, nil)
	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), opts)
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)

	// and destroy
	opts = SimplePlanOpts(plans.DestroyMode, nil)
	plan, diags = ctx.Plan(context.Background(), m, state, opts)
	assertNoErrors(t, diags)

	state, diags = ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)

	// and destroy again with no state
	if !state.Empty() {
		t.Fatal("expected empty state, got", state)
	}

	opts = SimplePlanOpts(plans.DestroyMode, nil)
	plan, diags = ctx.Plan(context.Background(), m, state, opts)
	assertNoErrors(t, diags)

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)
}

// don't evaluate conditions on outputs when destroying
func TestContext2Apply_noOutputChecksOnDestroy(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
module "mod" {
  source = "./mod"
}

output "from_resource" {
  value = module.mod.from_resource
}
`,

		"./mod/main.tf": `
resource "test_object" "x" {
  test_string = "wrong val"
}

output "from_resource" {
  value = test_object.x.test_string
  precondition {
    condition     = test_object.x.test_string == "ok"
    error_message = "resource error"
  }
}
`})

	p := simpleMockProvider()

	state := states.NewState()
	mod := state.EnsureModule(addrs.RootModuleInstance.Child("mod", addrs.NoKey))
	mod.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.x").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"test_string":"wrong_val"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	opts := SimplePlanOpts(plans.DestroyMode, nil)
	plan, diags := ctx.Plan(context.Background(), m, state, opts)
	assertNoErrors(t, diags)

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)
}

// -refresh-only should update checks
func TestContext2Apply_refreshApplyUpdatesChecks(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "x" {
  test_string = "ok"
  lifecycle {
    postcondition {
      condition = self.test_string == "ok"
      error_message = "wrong val"
    }
  }
}

output "from_resource" {
  value = test_object.x.test_string
  precondition {
	condition     = test_object.x.test_string == "ok"
	error_message = "wrong val"
  }
}
`})

	p := simpleMockProvider()
	p.ReadResourceResponse = &providers.ReadResourceResponse{
		NewState: cty.ObjectVal(map[string]cty.Value{
			"test_string": cty.StringVal("ok"),
		}),
	}

	state := states.NewState()
	mod := state.EnsureModule(addrs.RootModuleInstance)
	mod.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_object.x").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"test_string":"wrong val"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	mod.SetOutputValue("from_resource", cty.StringVal("wrong val"), false, "")

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	opts := SimplePlanOpts(plans.RefreshOnlyMode, nil)
	plan, diags := ctx.Plan(context.Background(), m, state, opts)
	assertNoErrors(t, diags)

	state, diags = ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)

	resCheck := state.CheckResults.GetObjectResult(mustResourceInstanceAddr("test_object.x"))
	if resCheck.Status != checks.StatusPass {
		t.Fatalf("unexpected check %s: %s\n", resCheck.Status, resCheck.FailureMessages)
	}

	outAddr := addrs.AbsOutputValue{
		Module: addrs.RootModuleInstance,
		OutputValue: addrs.OutputValue{
			Name: "from_resource",
		},
	}
	outCheck := state.CheckResults.GetObjectResult(outAddr)
	if outCheck.Status != checks.StatusPass {
		t.Fatalf("unexpected check %s: %s\n", outCheck.Status, outCheck.FailureMessages)
	}
}

// NoOp changes may have conditions to evaluate, but should not re-plan and
// apply the entire resource.
func TestContext2Apply_noRePlanNoOp(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_object" "x" {
}

resource "test_object" "y" {
  # test_object.w is being re-created, so this precondition must be evaluated
  # during apply, however this resource should otherwise be a NoOp.
  lifecycle {
    precondition {
      condition     = test_object.x.test_string == null
      error_message = "test_object.x.test_string should be null"
    }
  }
}
`})

	p := simpleMockProvider()
	// make sure we can compute the attr
	testString := p.GetProviderSchemaResponse.ResourceTypes["test_object"].Block.Attributes["test_string"]
	testString.Computed = true
	testString.Optional = false

	yAddr := mustResourceInstanceAddr("test_object.y")

	state := states.NewState()
	mod := state.RootModule()
	mod.SetResourceInstanceCurrent(
		yAddr.Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"test_string":"y"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	opts := SimplePlanOpts(plans.NormalMode, nil)
	plan, diags := ctx.Plan(context.Background(), m, state, opts)
	assertNoErrors(t, diags)

	for _, c := range plan.Changes.Resources {
		if c.Addr.Equal(yAddr) && c.Action != plans.NoOp {
			t.Fatalf("unexpected %s change for test_object.y", c.Action)
		}
	}

	// test_object.y is a NoOp change from the plan, but is included in the
	// graph due to the conditions which must be evaluated. This however should
	// not cause the resource to be re-planned.
	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
		testString := req.ProposedNewState.GetAttr("test_string")
		if !testString.IsNull() && testString.AsString() == "y" {
			resp.Diagnostics = resp.Diagnostics.Append(errors.New("Unexpected apply-time plan for test_object.y. Original plan was a NoOp"))
		}
		resp.PlannedState = req.ProposedNewState
		return resp
	}

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)
}

// ensure all references from preconditions are tracked through plan and apply
func TestContext2Apply_preconditionErrorMessageRef(t *testing.T) {
	p := testProvider("test")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	m := testModuleInline(t, map[string]string{
		"main.tf": `
module "nested" {
  source = "./mod"
}

output "nested_a" {
  value = module.nested.a
}
`,

		"mod/main.tf": `
variable "boop" {
  default = "boop"
}

variable "msg" {
  default = "Incorrect boop."
}

output "a" {
  value     = "x"

  precondition {
    condition     = var.boop == "boop"
    error_message = var.msg
  }
}
`,
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
	})
	assertNoErrors(t, diags)
	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)
}

func TestContext2Apply_destroyNullModuleOutput(t *testing.T) {
	p := testProvider("test")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	m := testModuleInline(t, map[string]string{
		"main.tf": `
module "null_module" {
  source = "./mod"
}

locals {
  module_output = module.null_module.null_module_test
}

output "test_root" {
  value = module.null_module.test_output
}

output "root_module" {
  value = local.module_output #fails
}
`,

		"mod/main.tf": `
output "test_output" {
  value = "test"
}

output "null_module_test" {
  value = null
}
`,
	})

	// verify plan and apply
	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
	})
	assertNoErrors(t, diags)
	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)

	// now destroy
	plan, diags = ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.DestroyMode,
	})
	assertNoErrors(t, diags)
	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)
}

func TestContext2Apply_moduleOutputWithSensitiveAttrs(t *testing.T) {
	// Ensure that nested sensitive marks are stored when accessing non-root
	// module outputs, and that they do not cause the entire output value to
	// become sensitive.
	m := testModuleInline(t, map[string]string{
		"main.tf": `
module "mod" {
  source = "./mod"
}

resource "test_resource" "b" {
  // if the module output were wholly sensitive it would not be valid to use in
  // for_each
  for_each = module.mod.resources
  value = each.value.output
}

output "root_output" {
  // The root output cannot contain any sensitive marks at all.
  // Applying nonsensitive would fail here if the nested sensitive mark were
  // not maintained through the output.
  value = [ for k, v in module.mod.resources : nonsensitive(v.output) ]
}
`,
		"./mod/main.tf": `
resource "test_resource" "a" {
  for_each = {"key": "value"}
  value = each.key
}

output "resources" {
  value = test_resource.a
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
						Type:      cty.String,
						Sensitive: true,
						Computed:  true,
					},
				},
			},
		},
	})
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})
	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
	})
	assertNoErrors(t, diags)
	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)
}

func TestContext2Apply_timestamps(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_resource" "a" {
  id = "timestamp"
  value = timestamp()
}

resource "test_resource" "b" {
  id = "plantimestamp"
  value = plantimestamp()
}
`,
	})

	var plantime time.Time

	p := testProvider("test")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"test_resource": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Required: true,
					},
					"value": {
						Type:     cty.String,
						Required: true,
					},
				},
			},
		},
	})
	p.PlanResourceChangeFn = func(request providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
		values := request.ProposedNewState.AsValueMap()
		if id := values["id"]; id.AsString() == "plantimestamp" {
			var err error
			plantime, err = time.Parse(time.RFC3339, values["value"].AsString())
			if err != nil {
				t.Errorf("couldn't parse plan time: %s", err)
			}
		}

		return providers.PlanResourceChangeResponse{
			PlannedState: request.ProposedNewState,
		}
	}
	p.ApplyResourceChangeFn = func(request providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
		values := request.PlannedState.AsValueMap()
		if id := values["id"]; id.AsString() == "timestamp" {
			applytime, err := time.Parse(time.RFC3339, values["value"].AsString())
			if err != nil {
				t.Errorf("couldn't parse apply time: %s", err)
			}

			if applytime.Before(plantime) {
				t.Errorf("applytime (%s) should be after plantime (%s)", applytime.Format(time.RFC3339), plantime.Format(time.RFC3339))
			}
		} else if id.AsString() == "plantimestamp" {
			otherplantime, err := time.Parse(time.RFC3339, values["value"].AsString())
			if err != nil {
				t.Errorf("couldn't parse plan time: %s", err)
			}

			if !plantime.Equal(otherplantime) {
				t.Errorf("plantime changed from (%s) to (%s) during apply", plantime.Format(time.RFC3339), otherplantime.Format(time.RFC3339))
			}
		}

		return providers.ApplyResourceChangeResponse{
			NewState: request.PlannedState,
		}
	}
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})
	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
	})
	assertNoErrors(t, diags)

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)
}

func TestContext2Apply_destroyUnusedModuleProvider(t *testing.T) {
	// an unused provider within a module should not be called during destroy
	unusedProvider := testProvider("unused")
	testProvider := testProvider("test")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"):   testProviderFuncFixed(testProvider),
			addrs.NewDefaultProvider("unused"): testProviderFuncFixed(unusedProvider),
		},
	})

	unusedProvider.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) (resp providers.ConfigureProviderResponse) {
		resp.Diagnostics = resp.Diagnostics.Append(errors.New("configuration failed"))
		return resp
	}

	m := testModuleInline(t, map[string]string{
		"main.tf": `
module "mod" {
  source = "./mod"
}

resource "test_resource" "test" {
}
`,

		"mod/main.tf": `
provider "unused" {
}

resource "unused_resource" "test" {
}
`,
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.DestroyMode,
	})
	assertNoErrors(t, diags)
	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)
}

func TestContext2Apply_import(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_resource" "a" {
  id = "importable"
}

import {
  to = test_resource.a
  id = "importable" 
}
`,
	})

	p := testProvider("test")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"test_resource": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Required: true,
					},
				},
			},
		},
	})
	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
		return providers.PlanResourceChangeResponse{
			PlannedState: req.ProposedNewState,
		}
	}
	p.ImportResourceStateFn = func(req providers.ImportResourceStateRequest) providers.ImportResourceStateResponse {
		return providers.ImportResourceStateResponse{
			ImportedResources: []providers.ImportedResource{
				{
					TypeName: "test_instance",
					State: cty.ObjectVal(map[string]cty.Value{
						"id": cty.StringVal("importable"),
					}),
				},
			},
		}
	}
	hook := new(MockHook)
	ctx := testContext2(t, &ContextOpts{
		Hooks: []Hook{hook},
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})
	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
	})
	assertNoErrors(t, diags)

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	assertNoErrors(t, diags)

	if !hook.PreApplyImportCalled {
		t.Fatalf("PreApplyImport hook not called")
	}
	if addr, wantAddr := hook.PreApplyImportAddr, mustResourceInstanceAddr("test_resource.a"); !addr.Equal(wantAddr) {
		t.Errorf("expected addr to be %s, but was %s", wantAddr, addr)
	}

	if !hook.PostApplyImportCalled {
		t.Fatalf("PostApplyImport hook not called")
	}
	if addr, wantAddr := hook.PostApplyImportAddr, mustResourceInstanceAddr("test_resource.a"); !addr.Equal(wantAddr) {
		t.Errorf("expected addr to be %s, but was %s", wantAddr, addr)
	}
}

func TestContext2Apply_noExternalReferences(t *testing.T) {
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

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), nil)
	if diags.HasErrors() {
		t.Fatalf("expected no errors, but got %s", diags)
	}

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("expected no errors, but got %s", diags)
	}

	// We didn't specify any external references, so the unreferenced local
	// value should have been tidied up and never made it into the state.
	module := state.RootModule()
	if len(module.LocalValues) > 0 {
		t.Errorf("expected no local values in the state but found %d", len(module.LocalValues))
	}
}

func TestContext2Apply_withExternalReferences(t *testing.T) {
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

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		ExternalReferences: []*addrs.Reference{
			mustReference("local.local_value"),
		},
	})
	if diags.HasErrors() {
		t.Fatalf("expected no errors, but got %s", diags)
	}

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("expected no errors, but got %s", diags)
	}

	// We did specify the local value in the external references, so it should
	// have been preserved even though it is not referenced by anything directly
	// in the config.
	module := state.RootModule()
	if module.LocalValues["local_value"].AsString() != "foo" {
		t.Errorf("expected local value to be \"foo\" but was \"%s\"", module.LocalValues["local_value"].AsString())
	}
}

func TestContext2Apply_forgetOrphanAndDeposed(t *testing.T) {
	desposedKey := states.DeposedKey("deposed")
	addr := "aws_instance.baz"
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			removed {
				from = aws_instance.baz
			}
		`,
	})
	hook := new(MockHook)
	p := testProvider("aws")
	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr(addr).Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"bar"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceDeposed(
		mustResourceInstanceAddr(addr).Resource,
		desposedKey,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectTainted,
			AttrsJSON:    []byte(`{"id":"bar"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	p.PlanResourceChangeFn = testDiffFn

	plan, diags := ctx.Plan(context.Background(), m, state, DefaultPlanOpts)
	assertNoErrors(t, diags)

	s, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	if !s.Empty() {
		t.Fatalf("State should be empty")
	}

	if p.ApplyResourceChangeCalled {
		t.Fatalf("When we forget we don't call the provider's ApplyResourceChange unlike in destroy")
	}

	if hook.PostApplyCalled {
		t.Fatalf("PostApply hook should not be called as part of forget")
	}
}

// This test is a copy and paste from TestContext2Apply_forgetOrphanAndDeposed
// with modifications to test the same scenario with  a dynamic provider instance.
func TestContext2Apply_forgetOrphanAndDeposedWithDynamicProvider(t *testing.T) {
	desposedKey := states.DeposedKey("deposed")
	addr := "aws_instance.baz"
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			provider aws {
				alias = "for_eached"
				for_each = {a: {}}
			}

			removed {
				from = aws_instance.baz
			}
		`,
	})
	p := testProvider("aws")
	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr(addr).Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"bar"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"].for_eached`),
		addrs.StringKey("a"),
	)
	root.SetResourceInstanceDeposed(
		mustResourceInstanceAddr(addr).Resource,
		desposedKey,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectTainted,
			AttrsJSON:    []byte(`{"id":"bar"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"].for_eached`),
		addrs.StringKey("a"),
	)
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	p.PlanResourceChangeFn = testDiffFn

	plan, diags := ctx.Plan(context.Background(), m, state, DefaultPlanOpts)
	assertNoErrors(t, diags)

	s, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	if !s.Empty() {
		t.Fatalf("State should be empty")
	}
}

func TestContext2Apply_providerExpandWithTargetOrExclude(t *testing.T) {
	// This test is covering a potentially-tricky interaction between the
	// logic that updates the provider instance references for resource
	// instances in state snapshots, and the -target/-exclude features which
	// cause OpenTofu to skip visiting certain objects.
	//
	// The main priority is that we never leave the final state snapshot
	// in a form that would cause errors or incorrect behavior on a future
	// plan/apply round. This test covers the current way we resolve the
	// ambiguity at the time of writing -- by updating the provider
	// instance addresses of all instances of any resource where at least
	// one instance is included in the plan -- but if future changes make
	// this test fail then it might be valid to introduce a different rule
	// as long as it still guarantees to create a valid final state snapshot.

	rsrcFirst := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "mock",
		Name: "first",
	}.Absolute(addrs.RootModuleInstance)
	rsrcFirstInstA := rsrcFirst.Instance(addrs.StringKey("a"))
	rsrcFirstInstB := rsrcFirst.Instance(addrs.StringKey("b"))
	rsrcSecond := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "mock",
		Name: "second",
	}.Absolute(addrs.RootModuleInstance)
	rsrcSecondInstA := rsrcSecond.Instance(addrs.StringKey("a"))
	rsrcSecondInstB := rsrcSecond.Instance(addrs.StringKey("b"))

	// We use the same test sequence for both -target and -exclude, with
	// makeStep2PlanOpts providing whichever filter is appropriate for
	// each test.
	// For correct operation the plan options must cause OpenTofu to
	// skip both instances of mock.second and visit at least one instance
	// of mock.first.
	runTest := func(t *testing.T, makeStep2PlanOpts func(plans.Mode) *PlanOpts) {
		mockProviderAddr := addrs.NewBuiltInProvider("mock")
		providerConfigBefore := addrs.AbsProviderConfig{
			Module:   addrs.RootModule,
			Provider: mockProviderAddr,
			Alias:    "before",
		}
		providerConfigAfter := addrs.AbsProviderConfig{
			Module:   addrs.RootModule,
			Provider: mockProviderAddr,
			Alias:    "after",
		}
		normalPlanOpts := &PlanOpts{
			Mode: plans.NormalMode,
		}
		p := &MockProvider{
			GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
				Provider: providers.Schema{
					Block: &configschema.Block{},
				},
				ResourceTypes: map[string]providers.Schema{
					"mock": {
						Block: &configschema.Block{},
					},
				},
			},
		}

		// For this particular test we'll also save state snapshots in JSON format,
		// so that we're exercising the state snapshot writer and reader in similar way
		// to how OpenTofu CLI would do it, in case its normalization rules and
		// consistency checks cause any problems that we can't notice when we're just
		// passing pointers to the live data structure.
		var stateJSON []byte
		readStateSnapshot := func(t *testing.T) *states.State {
			t.Helper()
			if len(stateJSON) == 0 {
				return states.NewState()
			}
			ret, err := statefile.Read(bytes.NewReader(stateJSON), encryption.StateEncryptionDisabled())
			if err != nil {
				t.Fatalf("failed to read latest state snapshot: %s", err)
			}
			return ret.State
		}
		writeStateSnapshot := func(t *testing.T, state *states.State) {
			t.Helper()
			f := &statefile.File{
				State:            state,
				TerraformVersion: version.SemVer,
			}
			buf := bytes.NewBuffer(nil)
			err := statefile.Write(f, buf, encryption.StateEncryptionDisabled())
			if err != nil {
				t.Fatalf("failed to write new state snapshot: %s", err)
			}
			stateJSON = buf.Bytes()
		}
		assertResourceInstanceProviderInstance := func(
			t *testing.T,
			state *states.State,
			addr addrs.AbsResourceInstance,
			wantConfigAddr addrs.AbsProviderConfig,
			wantKey addrs.InstanceKey,
		) {
			t.Helper()
			rsrcAddr := addr.ContainingResource()
			r := state.Resource(rsrcAddr)
			if r == nil {
				t.Fatalf("state has no record of %s", rsrcAddr)
			}
			ri := r.Instance(addr.Resource.Key)
			if ri == nil {
				t.Fatalf("state has no record of %s", addr)
			}
			ok := true
			if got, want := r.ProviderConfig, wantConfigAddr; got.String() != want.String() {
				t.Errorf(
					"%s has incorrect provider configuration address\ngot:  %s\nwant: %s",
					rsrcAddr, got, want,
				)
				ok = false
			}
			if got, want := ri.ProviderKey, wantKey; got != want {
				t.Errorf(
					"%s has incorrect provider instance key\ngot:  %s\nwant: %s",
					addr, got, want,
				)
				ok = false
			}
			if !ok {
				t.FailNow()
			}
		}

		t.Log("Step 1: Apply with a multi-instance provider config and two resources to create our initial state")
		{
			state := readStateSnapshot(t)
			m := testModuleInline(t, map[string]string{
				"main.tf": `
				terraform {
					required_providers {
						mock = {
							source = "terraform.io/builtin/mock"
						}
					}
				}

				locals {
					instances = toset(["a", "b"])
				}

				provider "mock" {
				    alias    = "before"
					for_each = local.instances
				}

				resource "mock" "first" {
					# NOTE: This is a bad example to follow in a real config,
					# since it would not be possible to remove elements from
					# local.instances without encountering an error. We don't
					# intend to do that for this test, though.
					for_each = local.instances
					provider = mock.before[each.key]
				}

				resource "mock" "second" {
					for_each = local.instances
					provider = mock.before[each.key]
				}
			`,
			})
			ctx := testContext2(t, &ContextOpts{
				Providers: map[addrs.Provider]providers.Factory{
					addrs.NewBuiltInProvider("mock"): testProviderFuncFixed(p),
				},
			})
			plan, diags := ctx.Plan(context.Background(), m, state, normalPlanOpts)
			assertNoErrors(t, diags)

			newState, diags := ctx.Apply(context.Background(), plan, m, nil)
			assertNoErrors(t, diags)

			assertResourceInstanceProviderInstance(
				t, newState,
				rsrcFirstInstA,
				providerConfigBefore, addrs.StringKey("a"),
			)
			assertResourceInstanceProviderInstance(
				t, newState,
				rsrcFirstInstB,
				providerConfigBefore, addrs.StringKey("b"),
			)
			assertResourceInstanceProviderInstance(
				t, newState,
				rsrcSecondInstA,
				providerConfigBefore, addrs.StringKey("a"),
			)
			assertResourceInstanceProviderInstance(
				t, newState,
				rsrcSecondInstB,
				providerConfigBefore, addrs.StringKey("b"),
			)

			writeStateSnapshot(t, newState)
		}

		t.Log("Step 2: Change the provider configuration address in config but then apply with some graph nodes excluded")
		{
			state := readStateSnapshot(t)
			m := testModuleInline(t, map[string]string{
				"main.tf": `
				terraform {
					required_providers {
						mock = {
							source = "terraform.io/builtin/mock"
						}
					}
				}

				locals {
					instances = toset(["a", "b"])
				}

				provider "mock" {
				    alias    = "after"
					for_each = local.instances
				}

				resource "mock" "first" {
					for_each = local.instances
					provider = mock.after[each.key]
				}

				resource "mock" "second" {
					for_each = local.instances
					provider = mock.after[each.key]
				}
			`,
			})
			ctx := testContext2(t, &ContextOpts{
				Providers: map[addrs.Provider]providers.Factory{
					addrs.NewBuiltInProvider("mock"): testProviderFuncFixed(p),
				},
			})
			plan, diags := ctx.Plan(context.Background(), m, state, makeStep2PlanOpts(plans.NormalMode))
			assertNoErrors(t, diags)

			newState, diags := ctx.Apply(context.Background(), plan, m, nil)
			assertNoErrors(t, diags)

			// Because makeStep2PlanOpts told us to retain at least one
			// instance of mock.first, both instances should've been
			// updated to refer to the new provider instance addresses.
			assertResourceInstanceProviderInstance(
				t, newState,
				rsrcFirstInstA,
				providerConfigAfter, addrs.StringKey("a"),
			)
			assertResourceInstanceProviderInstance(
				t, newState,
				rsrcFirstInstB,
				providerConfigAfter, addrs.StringKey("b"),
			)
			// Because makeStep2PlanOpts told us to exclude both instances
			// of mock.second, they continue to refer to the original
			// provider instance addresses.
			assertResourceInstanceProviderInstance(
				t, newState,
				rsrcSecondInstA,
				providerConfigBefore, addrs.StringKey("a"),
			)
			assertResourceInstanceProviderInstance(
				t, newState,
				rsrcSecondInstB,
				providerConfigBefore, addrs.StringKey("b"),
			)

			writeStateSnapshot(t, newState)
		}

		t.Log("Step 3: Remove the mock.first resource completely to destroy both instances using the updated provider config")
		{
			state := readStateSnapshot(t)
			m := testModuleInline(t, map[string]string{
				"main.tf": `
				terraform {
					required_providers {
						mock = {
							source = "terraform.io/builtin/mock"
						}
					}
				}

				locals {
					instances = toset(["a", "b"])
				}

				provider "mock" {
				    alias    = "after"
					for_each = local.instances
				}

				# mock.first intentionally removed, which should succeed
				# because the incoming state snapshot should remember that
				# it was associated with mock.after .

				resource "mock" "second" {
					for_each = local.instances
					provider = mock.after[each.key]
				}
			`,
			})
			ctx := testContext2(t, &ContextOpts{
				Providers: map[addrs.Provider]providers.Factory{
					addrs.NewBuiltInProvider("mock"): testProviderFuncFixed(p),
				},
			})
			plan, diags := ctx.Plan(context.Background(), m, state, normalPlanOpts)
			assertNoErrors(t, diags)

			newState, diags := ctx.Apply(context.Background(), plan, m, nil)
			assertNoErrors(t, diags)

			// The whole resource state for mock.first should've been removed now.
			if rs := newState.Resource(rsrcFirst); rs != nil {
				t.Errorf("final state still contains %s", rsrcFirst)
			}

			// We didn't target or exclude anything this time, so we
			// should now have both instances of mock.second updated
			// to refer to the new provider config.
			assertResourceInstanceProviderInstance(
				t, newState,
				rsrcSecondInstA,
				providerConfigAfter, addrs.StringKey("a"),
			)
			assertResourceInstanceProviderInstance(
				t, newState,
				rsrcSecondInstB,
				providerConfigAfter, addrs.StringKey("b"),
			)

			writeStateSnapshot(t, newState)
		}
	}

	t.Run("with target", func(t *testing.T) {
		runTest(t, func(planMode plans.Mode) *PlanOpts {
			return &PlanOpts{
				Mode: planMode,
				Targets: []addrs.Targetable{
					rsrcFirstInstA,
				},
			}
		})
	})
	t.Run("with exclude", func(t *testing.T) {
		runTest(t, func(planMode plans.Mode) *PlanOpts {
			return &PlanOpts{
				Mode: planMode,
				Excludes: []addrs.Targetable{
					rsrcSecondInstA,
					rsrcSecondInstB,
				},
			}
		})
	})
}

// All exclude flag tests in this file, from here forward, are inspired by some counterpart target flag test
// either from this file or from context_apply_test.go
func TestContext2Apply_moduleProviderAliasExcludes(t *testing.T) {
	m := testModule(t, "apply-module-provider-alias")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.ConfigResource{
				Module: addrs.Module{"child"},
				Resource: addrs.Resource{
					Mode: addrs.ManagedResourceMode,
					Type: "aws_instance",
					Name: "foo",
				},
			},
		},
	})
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	actual := strings.TrimSpace(state.String())
	expected := strings.TrimSpace(`
<no state>
	`)
	if actual != expected {
		t.Fatalf("wrong result\n\ngot:\n%s\n\nwant:\n%s", actual, expected)
	}
}

func TestContext2Apply_moduleProviderAliasExcludesNonExistent(t *testing.T) {
	m := testModule(t, "apply-module-provider-alias")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.ConfigResource{
				Module: addrs.RootModule,
				Resource: addrs.Resource{
					Mode: addrs.ManagedResourceMode,
					Type: "nonexistent",
					Name: "thing",
				},
			},
		},
	})
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	actual := strings.TrimSpace(state.String())
	expected := strings.TrimSpace(testTofuApplyModuleProviderAliasStr)
	if actual != expected {
		t.Fatalf("wrong result\n\ngot:\n%s\n\nwant:\n%s", actual, expected)
	}
}

// Tests that a module can be excluded and everything is properly created.
// This adds to the plan test to also just verify that apply works.
func TestContext2Apply_moduleExclude(t *testing.T) {
	m := testModule(t, "plan-targeted-cross-module")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Child("B", addrs.NoKey),
		},
	})
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	checkStateString(t, state, `
<no state>
module.A:
  aws_instance.foo:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    foo = bar
    type = aws_instance`)
}

// Tests that a module can be excluded, and dependent resources and modules are excluded as well
// This adds to the plan test to also just verify that apply works.
func TestContext2Apply_moduleExcludeDependent(t *testing.T) {
	m := testModule(t, "plan-targeted-cross-module")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Child("A", addrs.NoKey),
		},
	})
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	checkStateString(t, state, `
<no state>
`)
}

// Tests that non-existent module can be excluded, and that the apply happens fully
// This adds to the plan test to also just verify that apply works.
func TestContext2Apply_moduleExcludeNonExistent(t *testing.T) {
	m := testModule(t, "plan-targeted-cross-module")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Child("C", addrs.NoKey),
		},
	})
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	checkStateString(t, state, `
<no state>
module.A:
  aws_instance.foo:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    foo = bar
    type = aws_instance

  Outputs:

  value = foo
module.B:
  aws_instance.bar:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    foo = foo
    type = aws_instance

    Dependencies:
      module.A.aws_instance.foo
	`)
}

func TestContext2Apply_destroyExcludedNonExistentWithModuleVariableAndCount(t *testing.T) {
	m := testModule(t, "apply-destroy-mod-var-and-count")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn

	var state *states.State
	{
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
			},
		})

		// First plan and apply a create operation
		plan, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
		assertNoErrors(t, diags)

		state, diags = ctx.Apply(context.Background(), plan, m, nil)
		if diags.HasErrors() {
			t.Fatalf("apply err: %s", diags.Err())
		}
	}

	{
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
			},
		})

		plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.DestroyMode,
			Excludes: []addrs.Targetable{
				addrs.RootModuleInstance.Child("child", addrs.NoKey),
			},
		})
		if diags.HasErrors() {
			t.Fatalf("plan err: %s", diags)
		}
		if len(diags) != 1 {
			// Should have one warning that targeting is in effect.
			t.Fatalf("got %d diagnostics in plan; want 1", len(diags))
		}
		if got, want := diags[0].Severity(), tfdiags.Warning; got != want {
			t.Errorf("wrong diagnostic severity %#v; want %#v", got, want)
		}
		if got, want := diags[0].Description().Summary, "Resource targeting is in effect"; got != want {
			t.Errorf("wrong diagnostic summary %#v; want %#v", got, want)
		}

		// Destroy, excluding the module explicitly
		state, diags = ctx.Apply(context.Background(), plan, m, nil)
		if diags.HasErrors() {
			t.Fatalf("destroy apply err: %s", diags)
		}
		if len(diags) != 1 {
			t.Fatalf("got %d diagnostics; want 1", len(diags))
		}
		if got, want := diags[0].Severity(), tfdiags.Warning; got != want {
			t.Errorf("wrong diagnostic severity %#v; want %#v", got, want)
		}
		if got, want := diags[0].Description().Summary, "Applied changes may be incomplete"; got != want {
			t.Errorf("wrong diagnostic summary %#v; want %#v", got, want)
		}
	}

	// Test that things were destroyed
	actual := strings.TrimSpace(state.String())
	expected := strings.TrimSpace(`
<no state>
module.child:
  aws_instance.foo.0:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    type = aws_instance
  aws_instance.foo.1:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    type = aws_instance
  aws_instance.foo.2:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    type = aws_instance`)
	if actual != expected {
		t.Fatalf("expected: \n%s\n\nbad: \n%s", expected, actual)
	}
}

func TestContext2Apply_destroyExcludedWithModuleVariableAndCount(t *testing.T) {
	m := testModule(t, "apply-destroy-mod-var-and-count")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn

	var state *states.State
	{
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
			},
		})

		// First plan and apply a create operation
		plan, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
		assertNoErrors(t, diags)

		state, diags = ctx.Apply(context.Background(), plan, m, nil)
		if diags.HasErrors() {
			t.Fatalf("apply err: %s", diags.Err())
		}
	}

	{
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
			},
		})

		plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.DestroyMode,
			Excludes: []addrs.Targetable{
				addrs.RootModuleInstance.Child("non-existent-child", addrs.NoKey),
			},
		})
		if diags.HasErrors() {
			t.Fatalf("plan err: %s", diags)
		}
		if len(diags) != 1 {
			// Should have one warning that targeting is in effect.
			t.Fatalf("got %d diagnostics in plan; want 1", len(diags))
		}
		if got, want := diags[0].Severity(), tfdiags.Warning; got != want {
			t.Errorf("wrong diagnostic severity %#v; want %#v", got, want)
		}
		if got, want := diags[0].Description().Summary, "Resource targeting is in effect"; got != want {
			t.Errorf("wrong diagnostic summary %#v; want %#v", got, want)
		}

		// Destroy, excluding the module explicitly
		state, diags = ctx.Apply(context.Background(), plan, m, nil)
		if diags.HasErrors() {
			t.Fatalf("destroy apply err: %s", diags)
		}
		if len(diags) != 1 {
			t.Fatalf("got %d diagnostics; want 1", len(diags))
		}
		if got, want := diags[0].Severity(), tfdiags.Warning; got != want {
			t.Errorf("wrong diagnostic severity %#v; want %#v", got, want)
		}
		if got, want := diags[0].Description().Summary, "Applied changes may be incomplete"; got != want {
			t.Errorf("wrong diagnostic summary %#v; want %#v", got, want)
		}
	}

	// Test that things were destroyed
	actual := strings.TrimSpace(state.String())
	expected := strings.TrimSpace(`<no state>`)
	if actual != expected {
		t.Fatalf("expected: \n%s\n\nbad: \n%s", expected, actual)
	}
}

func TestContext2Apply_excluded(t *testing.T) {
	m := testModule(t, "apply-targeted")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Resource(
				addrs.ManagedResourceMode, "aws_instance", "bar",
			),
		},
	})
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	mod := state.RootModule()
	if len(mod.Resources) != 1 {
		t.Fatalf("expected 1 resource, got: %#v", mod.Resources)
	}

	checkStateString(t, state, `
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  num = 2
  type = aws_instance
	`)
}

func TestContext2Apply_excludedCount(t *testing.T) {
	m := testModule(t, "apply-targeted-count")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Resource(
				addrs.ManagedResourceMode, "aws_instance", "bar",
			),
		},
	})
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	checkStateString(t, state, `
aws_instance.foo.0:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
aws_instance.foo.1:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
aws_instance.foo.2:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
	`)
}

func TestContext2Apply_excludedCountIndex(t *testing.T) {
	m := testModule(t, "apply-targeted-count")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.ResourceInstance(
				addrs.ManagedResourceMode, "aws_instance", "foo", addrs.IntKey(1),
			),
		},
	})
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	checkStateString(t, state, `
aws_instance.bar.0:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
aws_instance.bar.1:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
aws_instance.bar.2:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
aws_instance.foo.0:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
aws_instance.foo.2:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance`)
}

func TestContext2Apply_excludedDestroy(t *testing.T) {
	m := testModule(t, "destroy-targeted")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn

	var state *states.State
	{
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
			},
		})

		// First plan and apply a create operation
		if diags := ctx.Validate(context.Background(), m); diags.HasErrors() {
			t.Fatalf("validate errors: %s", diags.Err())
		}

		plan, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
		assertNoErrors(t, diags)

		state, diags = ctx.Apply(context.Background(), plan, m, nil)
		if diags.HasErrors() {
			t.Fatalf("apply err: %s", diags.Err())
		}
	}

	{
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
			},
		})

		plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.DestroyMode,
			Excludes: []addrs.Targetable{
				addrs.RootModuleInstance.Resource(
					addrs.ManagedResourceMode, "aws_instance", "a",
				),
			},
		})
		assertNoErrors(t, diags)

		state, diags = ctx.Apply(context.Background(), plan, m, nil)
		if diags.HasErrors() {
			t.Fatalf("diags: %s", diags.Err())
		}
	}

	// The output should not be removed, as the aws_instance resource it relies on is excluded
	checkStateString(t, state, `
aws_instance.a:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance

Outputs:

out = foo`)
}

func TestContext2Apply_excludedDestroyDependent(t *testing.T) {
	m := testModule(t, "destroy-targeted")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn

	var state *states.State
	{
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
			},
		})

		// First plan and apply a create operation
		if diags := ctx.Validate(context.Background(), m); diags.HasErrors() {
			t.Fatalf("validate errors: %s", diags.Err())
		}

		plan, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
		assertNoErrors(t, diags)

		state, diags = ctx.Apply(context.Background(), plan, m, nil)
		if diags.HasErrors() {
			t.Fatalf("apply err: %s", diags.Err())
		}
	}

	{
		ctx := testContext2(t, &ContextOpts{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
			},
		})

		plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.DestroyMode,
			Excludes: []addrs.Targetable{
				addrs.RootModuleInstance.Child("child", addrs.NoKey),
			},
		})
		assertNoErrors(t, diags)

		state, diags = ctx.Apply(context.Background(), plan, m, nil)
		if diags.HasErrors() {
			t.Fatalf("diags: %s", diags.Err())
		}
	}

	// The output should not be removed, as the aws_instance resource it relies on is excluded
	checkStateString(t, state, `
aws_instance.a:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance

Outputs:

out = foo

module.child:
  aws_instance.b:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    foo = foo
    type = aws_instance

    Dependencies:
      aws_instance.a`)
}

func TestContext2Apply_excludedDestroyCountDeps(t *testing.T) {
	m := testModule(t, "apply-destroy-targeted-count")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo[0]").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"i-bcd345"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo[1]").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"i-cde345"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo[2]").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"i-def345"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.bar").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"i-abc123"}`),
			Dependencies: []addrs.ConfigResource{mustConfigResourceAddr("aws_instance.foo")},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.DestroyMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Resource(
				addrs.ManagedResourceMode, "aws_instance", "foo",
			),
		},
	})
	assertNoErrors(t, diags)

	state, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	checkStateString(t, state, `
aws_instance.foo.0:
  ID = i-bcd345
  provider = provider["registry.opentofu.org/hashicorp/aws"]
aws_instance.foo.1:
  ID = i-cde345
  provider = provider["registry.opentofu.org/hashicorp/aws"]
aws_instance.foo.2:
  ID = i-def345
  provider = provider["registry.opentofu.org/hashicorp/aws"]`)
}

func TestContext2Apply_excludedDependentDestroyCountDeps(t *testing.T) {
	m := testModule(t, "apply-destroy-targeted-count")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo[0]").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"i-bcd345"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo[1]").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"i-cde345"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo[2]").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"i-def345"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.bar").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"i-abc123"}`),
			Dependencies: []addrs.ConfigResource{mustConfigResourceAddr("aws_instance.foo")},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.DestroyMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Resource(
				addrs.ManagedResourceMode, "aws_instance", "bar",
			),
		},
	})
	assertNoErrors(t, diags)

	state, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	checkStateString(t, state, `
aws_instance.bar:
  ID = i-abc123
  provider = provider["registry.opentofu.org/hashicorp/aws"]

  Dependencies:
    aws_instance.foo
aws_instance.foo.0:
  ID = i-bcd345
  provider = provider["registry.opentofu.org/hashicorp/aws"]
aws_instance.foo.1:
  ID = i-cde345
  provider = provider["registry.opentofu.org/hashicorp/aws"]
aws_instance.foo.2:
  ID = i-def345
  provider = provider["registry.opentofu.org/hashicorp/aws"]`)
}

func TestContext2Apply_excludedDestroyModule(t *testing.T) {
	m := testModule(t, "apply-targeted-module")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"i-bcd345"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.bar").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"i-abc123"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	child := state.EnsureModule(addrs.RootModuleInstance.Child("child", addrs.NoKey))
	child.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"i-bcd345"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	child.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.bar").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"i-abc123"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.DestroyMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Child("child", addrs.NoKey).Resource(
				addrs.ManagedResourceMode, "aws_instance", "foo",
			),
		},
	})
	assertNoErrors(t, diags)

	state, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	checkStateString(t, state, `
<no state>
module.child:
  aws_instance.foo:
    ID = i-bcd345
    provider = provider["registry.opentofu.org/hashicorp/aws"]`)
}

func TestContext2Apply_excludedDestroyCountIndex(t *testing.T) {
	m := testModule(t, "apply-targeted-count")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn

	foo := &states.ResourceInstanceObjectSrc{
		Status:    states.ObjectReady,
		AttrsJSON: []byte(`{"id":"i-bcd345"}`),
	}
	bar := &states.ResourceInstanceObjectSrc{
		Status:    states.ObjectReady,
		AttrsJSON: []byte(`{"id":"i-abc123"}`),
	}

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo[0]").Resource,
		foo,
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo[1]").Resource,
		foo,
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo[2]").Resource,
		foo,
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.bar[0]").Resource,
		bar,
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.bar[1]").Resource,
		bar,
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.bar[2]").Resource,
		bar,
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.DestroyMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.ResourceInstance(
				addrs.ManagedResourceMode, "aws_instance", "foo", addrs.IntKey(2),
			),
			addrs.RootModuleInstance.ResourceInstance(
				addrs.ManagedResourceMode, "aws_instance", "bar", addrs.IntKey(1),
			),
		},
	})
	assertNoErrors(t, diags)

	state, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	checkStateString(t, state, `
aws_instance.bar.1:
  ID = i-abc123
  provider = provider["registry.opentofu.org/hashicorp/aws"]
aws_instance.foo.2:
  ID = i-bcd345
  provider = provider["registry.opentofu.org/hashicorp/aws"]
	`)
}

func TestContext2Apply_excludedModule(t *testing.T) {
	m := testModule(t, "apply-targeted-module")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Child("child", addrs.NoKey),
		},
	})
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	mod := state.Module(addrs.RootModuleInstance.Child("child", addrs.NoKey))
	if mod != nil {
		t.Fatalf("child module should not be in state, but was found in the state!\n\n%#v", state)
	}

	checkStateString(t, state, `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
	`)
}

func TestContext2Apply_excludedModuleResourceDep(t *testing.T) {
	m := testModule(t, "apply-targeted-module-dep")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Child("child", addrs.NoKey).Resource(
				addrs.ManagedResourceMode, "aws_instance", "mod",
			),
		},
	})
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	} else {
		t.Logf("Diff: %s", legacyDiffComparisonString(plan.Changes))
	}

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	checkStateString(t, state, `
<no state>
`)
}

func TestContext2Apply_excludedResourceDependentOnModule(t *testing.T) {
	m := testModule(t, "apply-targeted-module-dep")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Resource(addrs.ManagedResourceMode, "aws_instance", "foo"),
		},
	})
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	} else {
		t.Logf("Diff: %s", legacyDiffComparisonString(plan.Changes))
	}

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	checkStateString(t, state, `
<no state>
module.child:
  aws_instance.mod:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    type = aws_instance
`)
}

func TestContext2Apply_excludedModuleDep(t *testing.T) {
	m := testModule(t, "apply-targeted-module-dep")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Child("child", addrs.NoKey),
		},
	})
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	} else {
		t.Logf("Diff: %s", legacyDiffComparisonString(plan.Changes))
	}

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	checkStateString(t, state, `
<no state>
`)
}

func TestContext2Apply_excludedModuleUnrelatedOutputs(t *testing.T) {
	m := testModule(t, "apply-targeted-module-unrelated-outputs")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn

	state := states.NewState()

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			// Excluding aws_instance.foo should also exclude module.child1, which is dependent on it
			addrs.RootModuleInstance.Resource(addrs.ManagedResourceMode, "aws_instance", "foo"),
		},
	})
	assertNoErrors(t, diags)

	s, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	// - module.child1's instance_id output is dropped because we don't preserve
	//   non-root module outputs between runs (they can be recalculated from config)
	// - module.child2's instance_id is updated because its dependency is updated
	// - child2_id is updated because if its transitive dependency via module.child2
	checkStateString(t, s, `
<no state>
Outputs:

child2_id = foo

module.child2:
  aws_instance.foo:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    type = aws_instance

  Outputs:

  instance_id = foo
`)
}

func TestContext2Apply_excludedModuleResource(t *testing.T) {
	m := testModule(t, "apply-targeted-module-resource")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Child("child", addrs.NoKey).Resource(
				addrs.ManagedResourceMode, "aws_instance", "foo",
			),
		},
	})
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %s", diags.Err())
	}

	mod := state.Module(addrs.RootModuleInstance.Child("child", addrs.NoKey))
	if mod == nil || len(mod.Resources) != 1 {
		t.Fatalf("expected 1 resource, got: %#v", mod)
	}

	checkStateString(t, state, `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance

module.child:
  aws_instance.bar:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    num = 2
    type = aws_instance
	`)
}

func TestContext2Apply_excludedResourceOrphanModule(t *testing.T) {
	m := testModule(t, "apply-targeted-resource-orphan-module")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn

	state := states.NewState()
	child := state.EnsureModule(addrs.RootModuleInstance.Child("parent", addrs.NoKey))
	child.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.bar").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"abc","type":"aws_instance"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Child("parent", addrs.NoKey).Resource(
				addrs.ManagedResourceMode, "aws_instance", "bar",
			),
		},
	})
	assertNoErrors(t, diags)

	state, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("apply errors: %s", diags.Err())
	}

	checkStateString(t, state, `
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance

module.parent:
  aws_instance.bar:
    ID = abc
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    type = aws_instance
`)
}

func TestContext2Apply_excludedOrphanModule(t *testing.T) {
	m := testModule(t, "apply-targeted-resource-orphan-module")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn

	state := states.NewState()
	child := state.EnsureModule(addrs.RootModuleInstance.Child("parent", addrs.NoKey))
	child.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.bar").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"abc","type":"aws_instance"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Child("parent", addrs.NoKey),
		},
	})
	assertNoErrors(t, diags)

	state, diags = ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("apply errors: %s", diags.Err())
	}

	checkStateString(t, state, `
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance

module.parent:
  aws_instance.bar:
    ID = abc
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    type = aws_instance
`)
}

func TestContext2Apply_excludedWithTaintedInState(t *testing.T) {
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn
	m, snap := testModuleWithSnapshot(t, "apply-tainted-targets")

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.ifailedprovisioners").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectTainted,
			AttrsJSON: []byte(`{"id":"ifailedprovisioners"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Resource(
				addrs.ManagedResourceMode, "aws_instance", "ifailedprovisioners",
			),
		},
	})
	if diags.HasErrors() {
		t.Fatalf("err: %s", diags.Err())
	}

	// Write / Read plan to simulate running it through a Plan file
	ctxOpts, m, plan, err := contextOptsForPlanViaFile(t, snap, plan)
	if err != nil {
		t.Fatalf("failed to round-trip through planfile: %s", err)
	}

	ctxOpts.Providers = map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
	}

	ctx, diags = NewContext(ctxOpts)
	if diags.HasErrors() {
		t.Fatalf("err: %s", diags.Err())
	}

	s, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("err: %s", diags.Err())
	}

	actual := strings.TrimSpace(s.String())
	expected := strings.TrimSpace(`
aws_instance.iambeingadded:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
aws_instance.ifailedprovisioners: (tainted)
  ID = ifailedprovisioners
  provider = provider["registry.opentofu.org/hashicorp/aws"]
		`)
	if actual != expected {
		t.Fatalf("expected state: \n%s\ngot: \n%s", expected, actual)
	}
}

func TestContext2Apply_excludedModuleRecursive(t *testing.T) {
	m := testModule(t, "apply-targeted-module-recursive")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn
	p.ApplyResourceChangeFn = testApplyFn
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Child("child", addrs.NoKey),
		},
	})
	assertNoErrors(t, diags)

	state, diags := ctx.Apply(context.Background(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("err: %s", diags.Err())
	}

	mod := state.Module(
		addrs.RootModuleInstance.Child("child", addrs.NoKey).Child("subchild", addrs.NoKey),
	)
	if mod != nil {
		t.Fatalf("subchild module should not exist in the state, but was found!\n\n%#v", state)
	}

	checkStateString(t, state, `
<no state>
	`)
}

func TestContext2Apply_providerResourceIteration(t *testing.T) {
	localComplete := `
locals {
	direct = "primary"
	providers = { "primary": "eu-west-1", "secondary": "eu-west-2" }
	resources = ["primary", "secondary"]
}
`
	localPartial := `
locals {
	direct = "primary"
	providers = { "primary": "eu-west-1", "secondary": "eu-west-2" }
	resources = ["primary"]
}
`
	localMissing := `
locals {
	direct = "primary"
	providers = { "primary": "eu-west-1"}
	resources = ["primary", "secondary"]
}
`
	providerConfig := `
provider "test" {
  alias = "al"
  for_each = local.providers
  region = each.value
}
`
	resourceConfig := `
resource "test_instance" "a" {
  for_each = toset(local.resources)
  provider = test.al[each.key]
}
data "test_data_source" "b" {
  for_each = toset(local.resources)
  provider = test.al[each.key]
}

resource "test_instance" "a_direct" {
  for_each = toset(local.resources)
  provider = test.al[local.direct]
}
data "test_data_source" "b_direct" {
  for_each = toset(local.resources)
  provider = test.al[local.direct]
}
`
	complete := testModuleInline(t, map[string]string{
		"locals.tofu":    localComplete,
		"providers.tofu": providerConfig,
		"resources.tofu": resourceConfig,
	})
	partial := testModuleInline(t, map[string]string{
		"locals.tofu":    localPartial,
		"providers.tofu": providerConfig,
		"resources.tofu": resourceConfig,
	})
	removed := testModuleInline(t, map[string]string{
		"locals.tofu":    localPartial,
		"providers.tofu": providerConfig,
	})
	missingKey := testModuleInline(t, map[string]string{
		"locals.tofu":    localMissing,
		"providers.tofu": providerConfig,
		"resources.tofu": resourceConfig,
	})
	empty := testModuleInline(t, nil)

	provider := testProvider("test")
	provider.ReadDataSourceResponse = &providers.ReadDataSourceResponse{
		State: cty.ObjectVal(map[string]cty.Value{
			"id":  cty.StringVal("data_source"),
			"foo": cty.StringVal("ok"),
		}),
	}
	provider.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
		var resp providers.ConfigureProviderResponse

		region := req.Config.GetAttr("region")
		if region.AsString() != "eu-west-1" && region.AsString() != "eu-west-2" {
			resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("incorrect config val: %#v\n", region))
		}
		return resp
	}
	ps := map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("test"): testProviderFuncFixed(provider),
	}

	apply := func(t *testing.T, m *configs.Config, prevState *states.State) (*states.State, tfdiags.Diagnostics) {
		t.Helper()
		ctx := testContext2(t, &ContextOpts{
			Providers: ps,
		})

		plan, diags := ctx.Plan(context.Background(), m, prevState, DefaultPlanOpts)
		if diags.HasErrors() {
			return nil, diags
		}

		return ctx.Apply(context.Background(), plan, m, nil)
	}

	destroy := func(t *testing.T, m *configs.Config, prevState *states.State) (*states.State, tfdiags.Diagnostics) {
		ctx := testContext2(t, &ContextOpts{
			Providers: ps,
		})

		plan, diags := ctx.Plan(context.Background(), m, prevState, &PlanOpts{
			Mode: plans.DestroyMode,
		})
		if diags.HasErrors() {
			return nil, diags
		}

		return ctx.Apply(context.Background(), plan, m, nil)
	}

	primaryResource := mustResourceInstanceAddr(`test_instance.a["primary"]`)
	secondaryResource := mustResourceInstanceAddr(`test_instance.a["secondary"]`)

	t.Run("apply_destroy", func(t *testing.T) {
		state, diags := apply(t, complete, states.NewState())
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		if state.ResourceInstance(primaryResource).ProviderKey != addrs.StringKey("primary") {
			t.Fatal("Wrong provider key")
		}
		if state.ResourceInstance(secondaryResource).ProviderKey != addrs.StringKey("secondary") {
			t.Fatal("Wrong provider key")
		}

		_, diags = destroy(t, complete, state)
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}
	})

	t.Run("apply_removed", func(t *testing.T) {
		state, diags := apply(t, complete, states.NewState())
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		state, diags = apply(t, removed, state)
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		// Expect destroyed
		if state.ResourceInstance(primaryResource) != nil {
			t.Fatal(primaryResource.String())
		}
		if state.ResourceInstance(secondaryResource) != nil {
			t.Fatal(secondaryResource.String())
		}
	})

	t.Run("apply_orphan_destroy", func(t *testing.T) {
		state, diags := apply(t, complete, states.NewState())
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		state, diags = apply(t, partial, state)
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		// Expect primary
		if state.ResourceInstance(primaryResource) == nil {
			t.Fatal(primaryResource.String())
		}
		// Missing secondary
		if state.ResourceInstance(secondaryResource) != nil {
			t.Fatal(secondaryResource.String())
		}

		_, diags = destroy(t, partial, state)
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}
	})

	t.Run("provider_key_removed_apply", func(t *testing.T) {
		state, diags := apply(t, complete, states.NewState())
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		_, diags = apply(t, missingKey, state)
		if !diags.HasErrors() {
			t.Fatal("expected diags")
		}
		for _, diag := range diags {
			if diag.Description().Summary == "Provider instance not present" {
				return
			}
		}
		t.Fatal(diags.Err())
	})

	t.Run("empty", func(t *testing.T) {
		state, diags := apply(t, complete, states.NewState())
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		_, diags = apply(t, empty, state)
		if !diags.HasErrors() {
			t.Fatal("expected diags")
		}
		for _, diag := range diags {
			if diag.Description().Summary == "Provider configuration not present" {
				return
			}
		}
		t.Fatal(diags.Err())
	})
}

func TestContext2Apply_providerModuleIteration(t *testing.T) {
	localComplete := `
locals {
	direct = "primary"
	providers = { "primary": "eu-west-1", "secondary": "eu-west-2" }
	mods = ["primary", "secondary"]
}
`
	localPartial := `
locals {
	direct = "primary"
	providers = { "primary": "eu-west-1", "secondary": "eu-west-2" }
	mods = ["primary"]
}
`
	localMissing := `
locals {
	direct = "primary"
	providers = { "primary": "eu-west-1"}
	mods = ["primary", "secondary"]
}
`
	providerConfig := `
provider "test" {
  alias = "al"
  for_each = local.providers
  region = each.value
}
`
	moduleCall := `
module "mod" {
  source = "./mod"
  for_each = toset(local.mods)
  providers = {
    test = test.al[each.key]
  }
}

module "mod_direct" {
  source = "./mod"
  for_each = toset(local.mods)
  providers = {
    test = test.al[local.direct]
  }
}
`
	resourceConfig := `
resource "test_instance" "a" {
}
data "test_data_source" "b" {
}
`
	complete := testModuleInline(t, map[string]string{
		"locals.tofu":        localComplete,
		"providers.tofu":     providerConfig,
		"modules.tofu":       moduleCall,
		"mod/resources.tofu": resourceConfig,
	})
	partial := testModuleInline(t, map[string]string{
		"locals.tofu":        localPartial,
		"providers.tofu":     providerConfig,
		"modules.tofu":       moduleCall,
		"mod/resources.tofu": resourceConfig,
	})
	removed := testModuleInline(t, map[string]string{
		"locals.tofu":    localPartial,
		"providers.tofu": providerConfig,
	})
	missingKey := testModuleInline(t, map[string]string{
		"locals.tofu":        localMissing,
		"providers.tofu":     providerConfig,
		"modules.tofu":       moduleCall,
		"mod/resources.tofu": resourceConfig,
	})
	empty := testModuleInline(t, nil)

	provider := testProvider("test")
	provider.ReadDataSourceResponse = &providers.ReadDataSourceResponse{
		State: cty.ObjectVal(map[string]cty.Value{
			"id":  cty.StringVal("data_source"),
			"foo": cty.StringVal("ok"),
		}),
	}
	provider.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
		var resp providers.ConfigureProviderResponse
		region := req.Config.GetAttr("region")
		if region.AsString() != "eu-west-1" && region.AsString() != "eu-west-2" {
			resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("incorrect config val: %#v\n", region))
		}
		return resp
	}
	ps := map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("test"): testProviderFuncFixed(provider),
	}

	apply := func(t *testing.T, m *configs.Config, prevState *states.State) (*states.State, tfdiags.Diagnostics) {
		t.Helper()
		ctx := testContext2(t, &ContextOpts{
			Providers: ps,
		})

		plan, diags := ctx.Plan(context.Background(), m, prevState, DefaultPlanOpts)
		if diags.HasErrors() {
			return nil, diags
		}

		return ctx.Apply(context.Background(), plan, m, nil)
	}

	destroy := func(t *testing.T, m *configs.Config, prevState *states.State) (*states.State, tfdiags.Diagnostics) {
		ctx := testContext2(t, &ContextOpts{
			Providers: ps,
		})

		plan, diags := ctx.Plan(context.Background(), m, prevState, &PlanOpts{
			Mode: plans.DestroyMode,
		})
		if diags.HasErrors() {
			return nil, diags
		}

		return ctx.Apply(context.Background(), plan, m, nil)
	}

	primaryResource := mustResourceInstanceAddr(`module.mod["primary"].test_instance.a`)
	secondaryResource := mustResourceInstanceAddr(`module.mod["secondary"].test_instance.a`)

	t.Run("apply_destroy", func(t *testing.T) {
		state, diags := apply(t, complete, states.NewState())
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		if state.ResourceInstance(primaryResource).ProviderKey != addrs.StringKey("primary") {
			t.Fatal("Wrong provider key")
		}
		if state.ResourceInstance(secondaryResource).ProviderKey != addrs.StringKey("secondary") {
			t.Fatal("Wrong provider key")
		}

		_, diags = destroy(t, complete, state)
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}
	})

	t.Run("apply_removed", func(t *testing.T) {
		state, diags := apply(t, complete, states.NewState())
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		state, diags = apply(t, removed, state)
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		// Expect destroyed
		if state.ResourceInstance(primaryResource) != nil {
			t.Fatal(primaryResource.String())
		}
		if state.ResourceInstance(secondaryResource) != nil {
			t.Fatal(secondaryResource.String())
		}
	})

	t.Run("apply_orphan_destroy", func(t *testing.T) {
		state, diags := apply(t, complete, states.NewState())
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		state, diags = apply(t, partial, state)
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		// Expect primary
		if state.ResourceInstance(primaryResource) == nil {
			t.Fatal(primaryResource.String())
		}
		// Missing secondary
		if state.ResourceInstance(secondaryResource) != nil {
			t.Fatal(secondaryResource.String())
		}

		_, diags = destroy(t, partial, state)
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}
	})

	t.Run("provider_key_removed_apply", func(t *testing.T) {
		state, diags := apply(t, complete, states.NewState())
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		_, diags = apply(t, missingKey, state)
		if !diags.HasErrors() {
			t.Fatal("expected diags")
		}
		for _, diag := range diags {
			if diag.Description().Summary == "Provider instance not present" {
				return
			}
		}
		t.Fatal(diags.Err())
	})

	t.Run("empty", func(t *testing.T) {
		state, diags := apply(t, complete, states.NewState())
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		_, diags = apply(t, empty, state)
		if !diags.HasErrors() {
			t.Fatal("expected diags")
		}
		for _, diag := range diags {
			if diag.Description().Summary == "Provider configuration not present" {
				return
			}
		}
		t.Fatal(diags)
	})
}

// Test variable references to other variables
func TestContext2Apply_variableValidateReferences(t *testing.T) {
	valid := testModuleInline(t, map[string]string{
		"main.tofu": `
locals {
  value = 10
  err = "error"
}
variable "root_var" {
  type = number
  validation {
    condition = var.root_var == local.value
    error_message = "${local.err} root"
  }
}
data "test_data_source" "res_parent" {
}
data "test_data_source" "res" {
	count = length(data.test_data_source.res_parent.id)
}
module "mod" {
  source = "./mod"
  mod_var = local.value + length(data.test_data_source.res[0].id)
}
`,
		"mod/mod.tofu": `
locals {
  expected = 21
  err = "error"
}
variable "mod_var" {
  type = number
  validation {
    condition = var.mod_var == local.expected
    error_message = "${local.err} mod"
  }
}
`,
	})
	circular := testModuleInline(t, map[string]string{
		"main.tofu": `
variable "root_var" {
  type = number
  validation {
    condition = var.root_var == var.other_var
    error_message = "root"
  }
}
variable "other_var" {
  type = number
  default = 10
  validation {
    condition = var.root_var == var.other_var
    error_message = "root"
  }
}
module "mod" {
  source = "./mod"
  mod_var = 10
}
`,
		"mod/mod.tofu": `
variable "mod_var" {
  type = number
  validation {
    condition = var.mod_var == var.other_var
    error_message = "mod"
  }
}
variable "other_var" {
  type = number
  default = 10
  validation {
    condition = var.mod_var == var.other_var
    error_message = "mod"
  }
}
`,
	})

	t.Run("valid", func(t *testing.T) {
		input := InputValuesFromCaller(map[string]cty.Value{"root_var": cty.NumberIntVal(10)})

		provider := testProvider("test")

		provider.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
			return providers.ReadDataSourceResponse{
				State: cty.ObjectVal(map[string]cty.Value{
					"id":  cty.StringVal("data_source"),
					"foo": cty.StringVal("ok"),
				}),
			}
		}

		ps := map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(provider),
		}

		ctx := testContext2(t, &ContextOpts{
			Providers: ps,
		})

		plan, diags := ctx.Plan(context.Background(), valid, nil, &PlanOpts{
			SetVariables: input,
		})
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		state, diags := ctx.Apply(context.Background(), plan, valid, nil)
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		plan, diags = ctx.Plan(context.Background(), valid, state, &PlanOpts{
			Mode:         plans.DestroyMode,
			SetVariables: input,
		})
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}

		_, diags = ctx.Apply(context.Background(), plan, valid, nil)
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}
	})

	t.Run("circular", func(t *testing.T) {
		input := InputValuesFromCaller(map[string]cty.Value{
			"root_var":  cty.NumberIntVal(10),
			"other_var": cty.NumberIntVal(10),
		})

		ctx := testContext2(t, &ContextOpts{})

		_, diags := ctx.Plan(context.Background(), circular, nil, &PlanOpts{
			SetVariables: input,
		})
		if !diags.HasErrors() || len(diags) != 2 {
			t.Fatal("expected cycle failure")
		}
		for _, diag := range diags {
			if !strings.HasPrefix(diag.Description().Summary, "Cycle: ") {
				t.Fatalf("Expected cycle error, got %#v", diag.Description())
			}
		}
	})
}

// Test unknown variable (timefn?)
// Test variable references to resource that's not/is available
func TestContext2Apply_variableValidateDataSource(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tofu": `
variable "expected" {}
resource "test_instance" "a" { }
module "mod" {
  source = "./mod"
  res_data = test_instance.a.id
  expected = var.expected
}
`,
		"mod/mod.tofu": `
variable "expected" {}
variable "res_data" {
  type = string
  validation {
    condition = var.res_data == var.expected
    error_message = "does not match"
  }
}
`,
	})

	provider := testProvider("test")
	provider.PlanResourceChangeFn = testDiffFn
	provider.ApplyResourceChangeFn = testApplyFn
	ps := map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("test"): testProviderFuncFixed(provider),
	}

	var input InputValues

	apply := func(t *testing.T, m *configs.Config, prevState *states.State) (*states.State, tfdiags.Diagnostics) {
		ctx := testContext2(t, &ContextOpts{
			Providers: ps,
		})

		plan, diags := ctx.Plan(context.Background(), m, prevState, &PlanOpts{
			Mode:         plans.NormalMode,
			SetVariables: input,
		})
		if diags.HasErrors() {
			return nil, diags
		}

		return ctx.Apply(context.Background(), plan, m, nil)
	}

	destroy := func(t *testing.T, m *configs.Config, prevState *states.State) tfdiags.Diagnostics {
		ctx := testContext2(t, &ContextOpts{
			Providers: ps,
		})

		plan, diags := ctx.Plan(context.Background(), m, prevState, &PlanOpts{
			Mode:         plans.DestroyMode,
			SetVariables: input,
		})
		if diags.HasErrors() {
			return diags
		}

		_, diags = ctx.Apply(context.Background(), plan, m, nil)
		return diags
	}

	resAddr := mustResourceInstanceAddr(`test_instance.a`)

	// Invalid validation condition
	input = InputValuesFromCaller(map[string]cty.Value{
		"expected": cty.StringVal("bar"),
	})
	_, diags := apply(t, m, states.NewState())
	if !diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	if got, want := diags[0].Description().Summary, "Invalid value for variable"; got != want {
		t.Fatalf("Expected: %q, got %q", want, got)
	}

	// Valid validation condition
	input = InputValuesFromCaller(map[string]cty.Value{
		"expected": cty.StringVal("foo"),
	})
	state, diags := apply(t, m, states.NewState())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	if !strings.Contains(string(state.ResourceInstance(resAddr).Current.AttrsJSON), `"id":"foo"`) {
		t.Fatalf("Wrong resource data: %s", string(state.ResourceInstance(resAddr).Current.AttrsJSON))
	}

	// Make sure subsequent applies are happy
	state, diags = apply(t, m, state)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Succesful Destroy
	diags = destroy(t, m, state)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Invalid Destroy
	input = InputValuesFromCaller(map[string]cty.Value{
		"expected": cty.StringVal("bar"),
	})
	diags = destroy(t, m, state)
	if !diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	if got, want := diags[0].Description().Summary, "Invalid value for variable"; got != want {
		t.Fatalf("Expected: %q, got %q", want, got)
	}
}

func TestContext2Apply_deprecationWarnings(t *testing.T) {
	const singleDeprecatedOutput = `
output "test-child" {
	value = "test-child"
	deprecated = "Don't use me"
}`

	tests := map[string]struct {
		module       map[string]string
		expectedWarn tfdiags.Description
	}{
		"simpleModCall": {
			expectedWarn: tfdiags.Description{
				Address: "test_object.test",
				Summary: "Value derived from a deprecated source",
				Detail:  "This value is derived from module.mod.test-child, which is deprecated with the following message:\n\nDon't use me",
			},
			module: map[string]string{
				"main.tf": `
		module "mod" {
			source = "./mod"
		}

		resource "test_object" "test" {
			test_string = module.mod.test-child
		}
		`,
				"./mod/main.tf": singleDeprecatedOutput,
			},
		},
		"modCallThroughLocal": {
			expectedWarn: tfdiags.Description{
				Summary: "Value derived from a deprecated source",
				Detail:  "This value's attribute test-child is derived from module.mod.test-child, which is deprecated with the following message:\n\nDon't use me",
			},
			module: map[string]string{
				"main.tf": `
		module "mod" {
			source = "./mod"
		}

		locals {
			a = module.mod
		}

		resource "test_object" "test" {
			test_string = "a-${local.a.test-child}.txt"
		}
		`,
				"./mod/main.tf": singleDeprecatedOutput,
			},
		},
		"modForEach": {
			expectedWarn: tfdiags.Description{
				Address: "test_object.test",
				Summary: "Value derived from a deprecated source",
				Detail:  "This value is derived from module.mod[\"a\"].test-child, which is deprecated with the following message:\n\nDon't use me",
			},
			module: map[string]string{
				"main.tf": `
		module "mod" {
			for_each = toset([ "a" ])
			source = "./mod"
		}

		resource "test_object" "test" {
			test_string = "a-${module.mod["a"].test-child}.txt"
		}`,
				"./mod/main.tf": singleDeprecatedOutput,
			},
		},
		"multipleModCalls": {
			expectedWarn: tfdiags.Description{
				Summary: "Value derived from a deprecated source",
				Detail:  "This value is derived from module.mod.test-child, which is deprecated with the following message:\n\nDon't use me",
			},
			module: map[string]string{
				"main.tf": `
module "mod" {
	source = "./mod"
}

resource "test_object" "test" {
	test_string = module.mod.test
}`,
				"./mod/mod/main.tf": singleDeprecatedOutput,
				"./mod/main.tf": `
module "mod" {
	source = "./mod"
}

output "test" {
	value = module.mod.test-child
}
				`,
			},
		},
		"variableCondition": {
			expectedWarn: tfdiags.Description{
				Summary: "Value derived from a deprecated source",
				Detail:  "This value is derived from module.mod.test-child, which is deprecated with the following message:\n\nDon't use me",
			},
			module: map[string]string{
				"main.tf": `
module "mod" {
  source = "./mod"
}

variable "a" {
  default = "a"
  validation {
    condition     = var.a != module.mod.test-child
	error_message = "invalid"
  }
}
`,
				"./mod/main.tf": singleDeprecatedOutput,
			},
		},
		"variableErrorMessage": {
			expectedWarn: tfdiags.Description{
				Summary: "Value derived from a deprecated source",
				Detail:  "This value is derived from module.mod.test-child, which is deprecated with the following message:\n\nDon't use me",
			},
			module: map[string]string{
				"main.tf": `
module "mod" {
  source = "./mod"
}

variable "a" {
  default = "a"
  validation {
    condition     = var.a == "a"
	error_message = module.mod.test-child
  }
}
`,
				"./mod/main.tf": singleDeprecatedOutput,
			},
		},
		"checkCondition": {
			expectedWarn: tfdiags.Description{
				Summary: "Value derived from a deprecated source",
				Detail:  "This value is derived from module.mod.test-child, which is deprecated with the following message:\n\nDon't use me",
			},
			module: map[string]string{
				"main.tf": `
module "mod" {
  source = "./mod"
}

check "test" {
  assert {
    condition = module.mod.test-child != "a"
    error_message = "invalid"
  }
}
`,
				"./mod/main.tf": singleDeprecatedOutput,
			},
		},
		"checkErrorMessage": {
			expectedWarn: tfdiags.Description{
				Summary: "Value derived from a deprecated source",
				Detail:  "This value is derived from module.mod.test-child, which is deprecated with the following message:\n\nDon't use me",
			},
			module: map[string]string{
				"main.tf": `
module "mod" {
  source = "./mod"
}

locals {
  a = "a"
}

check "test" {
  assert {
    condition = local.a == "a"
    error_message = module.mod.test-child
  }
}
`,
				"./mod/main.tf": singleDeprecatedOutput,
			},
		},
		"deprecatedInForEach": {
			expectedWarn: tfdiags.Description{
				Summary: "Value derived from a deprecated source",
				Detail:  "This value is derived from module.mod.test-child, which is deprecated with the following message:\n\nDon't use me",
			},
			module: map[string]string{
				"main.tf": `
module "mod" {
  source = "./mod"
}

module "modfe" {
  source = "./mod"
  for_each = toset([ module.mod.test-child ])
}
`,
				"./mod/main.tf": singleDeprecatedOutput,
			},
		},
	}

	p := simpleMockProvider()
	p.ApplyResourceChangeFn = func(arcr providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
		return providers.ApplyResourceChangeResponse{
			NewState: arcr.PlannedState,
		}
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			ctx := testContext2(t, &ContextOpts{
				Providers: map[addrs.Provider]providers.Factory{
					addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
				},
			})

			mod := testModuleInline(t, test.module)

			_, diags := ctx.Plan(context.Background(), mod, states.NewState(), SimplePlanOpts(plans.NormalMode, testInputValuesUnset(mod.Module.Variables)))
			if diags.HasErrors() {
				t.Fatalf("Unexpected error(s) during plan: %v", diags.Err())
			}

			if len(diags) != 1 {
				t.Fatalf("Expected a single warning, got: %v", diags.ErrWithWarnings())
			}

			if diags[0].Severity() != tfdiags.Warning {
				t.Fatalf("Expected a warning, got: %v", diags.ErrWithWarnings())
			}

			if got, want := diags[0].Description(), test.expectedWarn; !got.Equal(want) {
				t.Fatalf("Unexpected warning. Want:\n%v\nGot:\n%v\n", want, got)
			}
		})
	}
}

func TestContext2Apply_variableDeprecation(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
variable "var" {
	type = string
	deprecated = "this is deprecated"
}
module "call" {
	source = "./mod"
	input = "val"
}
`,
		"mod/main.tf": `
	variable "input" {
		type = string
		deprecated = "This variable is deprecated"
	}
	output "out" {
		value = "output the variable 'input' value: ${var.input}"
	}
`,
	})

	ctx := testContext2(t, &ContextOpts{})
	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		SetVariables: InputValues{
			"var": {
				Value:      cty.StringVal("from cli"),
				SourceType: ValueFromCLIArg,
			},
		},
	})
	assertDiagnosticsMatch(t, diags, tfdiags.Diagnostics{}.Append(
		&hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Summary:  `Variable marked as deprecated by the module author`,
			Detail:   "Variable \"input\" is marked as deprecated with the following message:\nThis variable is deprecated",
			Subject: &hcl.Range{
				Filename: fmt.Sprintf("%s/main.tf", m.Module.SourceDir),
				Start:    hcl.Pos{Line: 8, Column: 10, Byte: 113},
				End:      hcl.Pos{Line: 8, Column: 15, Byte: 118},
			},
		},
		&hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Summary:  `Deprecated variable used from the root module`,
			Detail:   `The root variable "var" is deprecated with the following message: this is deprecated`,
			Subject: &hcl.Range{
				Filename: fmt.Sprintf("%s/main.tf", m.Module.SourceDir),
				Start:    hcl.Pos{Line: 2, Column: 1, Byte: 1},
				End:      hcl.Pos{Line: 2, Column: 15, Byte: 15},
			},
		},
	))

	_, diags = ctx.Apply(context.Background(), plan, m, nil)
	assertDiagnosticsMatch(t, diags, tfdiags.Diagnostics{})
}

// Test if check block is being expanded right when module count is zero
func TestContext2Apply_moduleCountZeroChecks(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
module "check_module" {
  count  = 0
  source = "./check-module"
}
`,
		"check-module/main.tf": `
check "http_check" {
  data "http" "tofu" {
    url = "https://opentofu.org/"
  }

  assert {
    condition     = data.http.tofu.status_code == 200
    error_message = "${data.http.tofu.url} returned an unhealthy status code"
  }
}
`,
	})

	provider := testProvider("test")
	provider.PlanResourceChangeFn = testDiffFn
	provider.ApplyResourceChangeFn = testApplyFn
	ps := map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("test"): testProviderFuncFixed(provider),
		addrs.NewDefaultProvider("http"): testProviderFuncFixed(provider),
	}

	apply := func(t *testing.T, m *configs.Config, prevState *states.State) (*states.State, tfdiags.Diagnostics) {
		ctx := testContext2(t, &ContextOpts{
			Providers: ps,
		})

		plan, diags := ctx.Plan(context.Background(), m, prevState, &PlanOpts{
			Mode: plans.NormalMode,
		})
		if diags.HasErrors() {
			return nil, diags
		}

		return ctx.Apply(context.Background(), plan, m, nil)
	}

	destroy := func(t *testing.T, m *configs.Config, prevState *states.State) tfdiags.Diagnostics {
		ctx := testContext2(t, &ContextOpts{
			Providers: ps,
		})

		plan, diags := ctx.Plan(context.Background(), m, prevState, &PlanOpts{
			Mode: plans.DestroyMode,
		})
		if diags.HasErrors() {
			return diags
		}

		_, diags = ctx.Apply(context.Background(), plan, m, nil)
		return diags
	}

	// Valid validation condition
	state, diags := apply(t, m, states.NewState())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Make sure subsequent applies are happy
	state, diags = apply(t, m, state)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Succesful Destroy
	diags = destroy(t, m, state)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
}

// Test if check block is being expanded right when module count is zero (nested)
func TestContext2Apply_moduleCountZeroChecksNested(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
module "sub_module" {
  count  = 1
  source = "./sub-module"
}
`,
		"sub-module/main.tf": `
module "check_module" {
  count  = 0
  source = "./check-module"
}
`,
		"sub-module/check-module/main.tf": `
check "http_check" {
  data "http" "tofu" {
    url = "https://opentofu.org/"
  }

  assert {
    condition     = data.http.tofu.status_code == 200
    error_message = "${data.http.tofu.url} returned an unhealthy status code"
  }
}
`,
	})

	provider := testProvider("test")
	provider.PlanResourceChangeFn = testDiffFn
	provider.ApplyResourceChangeFn = testApplyFn
	ps := map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("test"): testProviderFuncFixed(provider),
		addrs.NewDefaultProvider("http"): testProviderFuncFixed(provider),
	}

	apply := func(t *testing.T, m *configs.Config, prevState *states.State) (*states.State, tfdiags.Diagnostics) {
		ctx := testContext2(t, &ContextOpts{
			Providers: ps,
		})

		plan, diags := ctx.Plan(context.Background(), m, prevState, &PlanOpts{
			Mode: plans.NormalMode,
		})
		if diags.HasErrors() {
			return nil, diags
		}

		return ctx.Apply(context.Background(), plan, m, nil)
	}

	destroy := func(t *testing.T, m *configs.Config, prevState *states.State) tfdiags.Diagnostics {
		ctx := testContext2(t, &ContextOpts{
			Providers: ps,
		})

		plan, diags := ctx.Plan(context.Background(), m, prevState, &PlanOpts{
			Mode: plans.DestroyMode,
		})
		if diags.HasErrors() {
			return diags
		}

		_, diags = ctx.Apply(context.Background(), plan, m, nil)
		return diags
	}

	// Valid validation condition
	state, diags := apply(t, m, states.NewState())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Make sure subsequent applies are happy
	state, diags = apply(t, m, state)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Succesful Destroy
	diags = destroy(t, m, state)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
}

// Test if check block is being expanded right when module for_each is empty
func TestContext2Apply_moduleEmptyForEachChecks(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		"main.tf": `
module "check_module" {
  for_each  = {}
  source = "./check-module"
}
`,
		"check-module/main.tf": `
check "http_check" {
  data "http" "tofu" {
    url = "https://opentofu.org/"
  }

  assert {
    condition     = data.http.tofu.status_code == 200
    error_message = "${data.http.tofu.url} returned an unhealthy status code"
  }
}
`,
	})

	provider := testProvider("test")
	provider.PlanResourceChangeFn = testDiffFn
	provider.ApplyResourceChangeFn = testApplyFn
	ps := map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("test"): testProviderFuncFixed(provider),
		addrs.NewDefaultProvider("http"): testProviderFuncFixed(provider),
	}

	apply := func(t *testing.T, m *configs.Config, prevState *states.State) (*states.State, tfdiags.Diagnostics) {
		ctx := testContext2(t, &ContextOpts{
			Providers: ps,
		})

		plan, diags := ctx.Plan(context.Background(), m, prevState, &PlanOpts{
			Mode: plans.NormalMode,
		})
		if diags.HasErrors() {
			return nil, diags
		}

		return ctx.Apply(context.Background(), plan, m, nil)
	}

	destroy := func(t *testing.T, m *configs.Config, prevState *states.State) tfdiags.Diagnostics {
		ctx := testContext2(t, &ContextOpts{
			Providers: ps,
		})

		plan, diags := ctx.Plan(context.Background(), m, prevState, &PlanOpts{
			Mode: plans.DestroyMode,
		})
		if diags.HasErrors() {
			return diags
		}

		_, diags = ctx.Apply(context.Background(), plan, m, nil)
		return diags
	}

	// Valid validation condition
	state, diags := apply(t, m, states.NewState())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Make sure subsequent applies are happy
	state, diags = apply(t, m, state)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Succesful Destroy
	diags = destroy(t, m, state)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
}

// TestContext2Apply_ephemeralResourcesLifecycleCheck is checking the hook calls
// and the state to be sure that the expected information is there.
func TestContext2Apply_ephemeralResourcesLifecycleCheck(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		`main.tf`: `
ephemeral "test_ephemeral_resource" "a" {
}
`,
	})

	provider := testProvider("test")
	provider.OpenEphemeralResourceResponse = &providers.OpenEphemeralResourceResponse{
		Result: cty.ObjectVal(map[string]cty.Value{
			"id":     cty.StringVal("id val"),
			"secret": cty.StringVal("val"),
		}),
	}

	ps := map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("test"): testProviderFuncFixed(provider),
	}

	h := &testHook{}
	apply := func(t *testing.T, m *configs.Config, prevState *states.State) (*states.State, tfdiags.Diagnostics) {
		ctx := testContext2(t, &ContextOpts{
			Providers: ps,
			Hooks:     []Hook{h},
		})

		plan, diags := ctx.Plan(context.Background(), m, prevState, &PlanOpts{
			Mode: plans.NormalMode,
		})
		if diags.HasErrors() {
			return nil, diags
		}

		return ctx.Apply(context.Background(), plan, m, nil)
	}

	newState, diags := apply(t, m, states.NewState())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	addr := mustAbsResourceAddr("ephemeral.test_ephemeral_resource.a")
	gotRes := newState.Resource(addr)
	wantRes := &states.Resource{
		Addr: addr,
		Instances: map[addrs.InstanceKey]*states.ResourceInstance{
			addrs.NoKey: {
				Current: &states.ResourceInstanceObjectSrc{
					AttrsJSON:          []byte(`{"id":"id val","secret":"val"}`),
					Status:             states.ObjectReady,
					AttrSensitivePaths: []cty.PathValueMarks{},
					Dependencies:       []addrs.ConfigResource{},
				},
				Deposed: map[states.DeposedKey]*states.ResourceInstanceObjectSrc{},
			},
		},
		ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
	}
	if diff := cmp.Diff(wantRes, gotRes); diff != "" {
		t.Errorf("unexpected ephemeral resource content in the state:\n%s", diff)
	}

	if got, want := len(h.Calls), 8; got != want {
		t.Fatalf("want %d hook calls but got %d", want, got)
	}
	wantCalls := []*testHookCall{
		{Action: "PreOpen", InstanceID: addr.String()},
		{Action: "PostOpen", InstanceID: addr.String()},
		{Action: "PreClose", InstanceID: addr.String()},
		{Action: "PostClose", InstanceID: addr.String()},
		{Action: "PreOpen", InstanceID: addr.String()},
		{Action: "PostOpen", InstanceID: addr.String()},
		{Action: "PreClose", InstanceID: addr.String()},
		{Action: "PostClose", InstanceID: addr.String()},
	}
	if diff := cmp.Diff(wantCalls, h.Calls); diff != "" {
		t.Fatalf("unexpected hook calls:\n%s", diff)
	}
}

// TestContext2Apply_planVariablesAndApplyArgsGetMergedCorrectly checks that
// the variable values given in ApplyArgs are getting merged correctly with
// the plan ones.
func TestContext2Apply_planVariablesAndApplyArgsGetMergedCorrectly(t *testing.T) {
	m := testModuleInline(t, map[string]string{
		`main.tf`: `
# NOTE: When a variable do have a default value as null it is not written to the plan, doesn't matter if it's ephemeral or not
variable "regular_required" {
  type = string
}

variable "ephemeral_required" {
  type = string
  ephemeral = true
}

variable "regular_optional" {
  type = string
  default = null
}

variable "ephemeral_optional" {
  type = string
  ephemeral = true
  default = null
}

output "regular_required" {
  value = var.regular_required
}

output "regular_optional" {
  value = var.regular_optional
}
`,
	})

	cases := map[string]struct {
		planSetVariables InputValues
		applyOpts        *ApplyOpts
		// Set this to "true" to test a flow similar with the one ran when running `tofu plan -out <planfile>` followed by `tofu apply <planfile>`.
		// "False" it means that it will run the test like running directly `tofu apply -auto-approve`
		simulatePlanRoundtrip bool

		expectedApplyErrors []string
		expectedOutputs     map[string]*states.OutputValue
	}{
		// //////// The tests in this section check real life scenarios, where a plan is written and
		// then read from the file during `tofu apply <planfile>`. This is the only way for the
		// ApplyOpts to be passed into the context.Apply()
		"mutate plan to be similar with the one loaded from file and apply without any opts": {
			planSetVariables: map[string]*InputValue{
				"ephemeral_required": {Value: cty.StringVal("eph val")},
				"ephemeral_optional": {},
				"regular_required":   {Value: cty.StringVal("reg val")},
				"regular_optional":   {},
			},
			simulatePlanRoundtrip: true,
			applyOpts:             nil,
			expectedApplyErrors:   []string{"No value for required variable - Variable \"ephemeral_required\" is configured as ephemeral. This type of variables need to be given a value during `tofu plan` and also during `tofu apply`."},
		},
		"mutate plan to be similar with the one loaded from file and apply with opts containing the ephemeral_required": {
			planSetVariables: map[string]*InputValue{
				"ephemeral_required": {Value: cty.StringVal("eph val"), SourceType: ValueFromPlan},
				"ephemeral_optional": {SourceType: ValueFromPlan},
				"regular_required":   {Value: cty.StringVal("reg val"), SourceType: ValueFromPlan},
				"regular_optional":   {SourceType: ValueFromPlan},
			},
			applyOpts:             &ApplyOpts{SetVariables: InputValues{"ephemeral_required": &InputValue{Value: cty.StringVal("from applyopts"), SourceType: ValueFromCLIArg}}},
			simulatePlanRoundtrip: true,
			expectedApplyErrors:   nil,
		},
		// //////// The tests below are not actually reproducible IRL, they just test the defensive implementation of mergePlanAndApplyVariables.
		// These are not reproducible IRL because in a direct `tofu apply` command, the ApplyOpts and PlanOpts will not exist together.

		// Validates that even when the optional variables have null values during plan creation, the applyOpts do not override the null values.
		"apply configuration directly where planOpts satisfies completely the variables therefore applyOpts don't get used": {
			planSetVariables: map[string]*InputValue{
				"ephemeral_required": {Value: cty.StringVal("eph val"), SourceType: ValueFromPlan},
				"ephemeral_optional": {SourceType: ValueFromPlan}, // This will not get into the plan since it's optional
				"regular_required":   {Value: cty.StringVal("reg val"), SourceType: ValueFromPlan},
				"regular_optional":   {SourceType: ValueFromPlan}, // This will not get into the plan since it's optional
			},
			applyOpts: &ApplyOpts{SetVariables: InputValues{
				"ephemeral_optional": &InputValue{Value: cty.StringVal("eph from applyopts"), SourceType: ValueFromCLIArg},
				"regular_optional":   &InputValue{Value: cty.StringVal("regular from applyopts"), SourceType: ValueFromCLIArg},
			}},
			expectedApplyErrors: nil,
			expectedOutputs: map[string]*states.OutputValue{
				"regular_required": {Value: cty.StringVal("reg val")},
			},
		},
		"apply setVariables contain value for undefined variable": {
			planSetVariables: map[string]*InputValue{
				"ephemeral_required": {Value: cty.StringVal("eph val"), SourceType: ValueFromPlan},
				"ephemeral_optional": {SourceType: ValueFromPlan},
				"regular_required":   {Value: cty.StringVal("reg val"), SourceType: ValueFromPlan},
				"regular_optional":   {SourceType: ValueFromPlan},
			},
			applyOpts: &ApplyOpts{
				SetVariables: map[string]*InputValue{
					"undefined_variable": {SourceType: ValueFromCLIArg, Value: cty.StringVal("test")},
				},
			},
			expectedApplyErrors: []string{
				`Missing variable in configuration - Variable "undefined_variable" not found in the given configuration`,
			},
		},
		"same variable has different values in applyOpts and planOpts": {
			planSetVariables: map[string]*InputValue{
				"ephemeral_required": {Value: cty.StringVal("eph val"), SourceType: ValueFromPlan},
				"ephemeral_optional": {Value: cty.StringVal("eph optional val"), SourceType: ValueFromPlan},
				"regular_required":   {Value: cty.StringVal("reg val"), SourceType: ValueFromPlan},
				"regular_optional":   {Value: cty.StringVal("reg optional val"), SourceType: ValueFromPlan},
			},
			applyOpts: &ApplyOpts{
				SetVariables: map[string]*InputValue{
					"ephemeral_required": {Value: cty.StringVal("eph val 2"), SourceType: ValueFromPlan},
					"ephemeral_optional": {Value: cty.StringVal("eph optional val 2"), SourceType: ValueFromPlan},
					"regular_required":   {Value: cty.StringVal("reg val 2"), SourceType: ValueFromPlan},
					"regular_optional":   {Value: cty.StringVal("reg optional val 2"), SourceType: ValueFromPlan},
				},
			},
			expectedApplyErrors: []string{
				`Mismatch between input and plan variable value - Value saved in the plan file for variable "ephemeral_required" is different from the one given to the current command.`,
				`Mismatch between input and plan variable value - Value saved in the plan file for variable "ephemeral_optional" is different from the one given to the current command.`,
				`Mismatch between input and plan variable value - Value saved in the plan file for variable "regular_required" is different from the one given to the current command.`,
				`Mismatch between input and plan variable value - Value saved in the plan file for variable "regular_optional" is different from the one given to the current command.`,
			},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			h := &testHook{}
			ctx := testContext2(t, &ContextOpts{
				Hooks: []Hook{h},
			})

			// check plan
			plan, diags := ctx.Plan(t.Context(), m, states.NewState(), &PlanOpts{
				Mode:         plans.NormalMode,
				SetVariables: tt.planSetVariables,
			})
			if diags.HasErrors() {
				t.Fatalf("unexepected diagnostics from plan: %s", diags)
			}

			if tt.simulatePlanRoundtrip {
				// By deleting the ephemeral variables from the VariableValues, we change the given plan
				// to match a plan read from the file. While loading the plan, the variables stored with
				// a null value are only stored in EphemeralVariables but not in VariableValues.
				for vName, ok := range plan.EphemeralVariables {
					if !ok {
						continue
					}
					delete(plan.VariableValues, vName)
				}
			}

			// check apply
			newState, diags := ctx.Apply(t.Context(), plan, m, tt.applyOpts)
			var gotErrors []string
			for _, diag := range diags {
				gotErrors = append(gotErrors, fmt.Sprintf("%s - %s", diag.Description().Summary, diag.Description().Detail))
			}
			slices.Sort(gotErrors)
			slices.Sort(tt.expectedApplyErrors)
			if diff := cmp.Diff(tt.expectedApplyErrors, gotErrors); diff != "" {
				t.Errorf("wrong errors received:\n%s", diff)
			}
			if tt.expectedOutputs != nil {
				cp := newState.DeepCopy()
				cp.Modules[addrs.RootModule.String()].OutputValues = tt.expectedOutputs

				gotState := newState.String()
				wantState := cp.String()
				if diff := cmp.Diff(gotState, wantState); diff != "" {
					t.Fatalf("got different state than expected:\n%s", diff)
				}
			}
		})
	}
}

func TestMergePlanAndApplyVariables(t *testing.T) {
	mustDynamicVarVal := func(val string) plans.DynamicValue {
		ret, err := plans.NewDynamicValue(cty.StringVal(val), cty.DynamicPseudoType)
		if err != nil {
			panic(err)
		}
		return ret
	}

	cases := map[string]struct {
		config               *configs.Config
		plan                 *plans.Plan
		opts                 *ApplyOpts
		expectedVals         InputValues
		expectedDiagsDetails []tfdiags.Description
	}{
		"backwards compatibility test - missing values from plan are set to nil": {
			&configs.Config{
				Module: &configs.Module{
					Variables: map[string]*configs.Variable{
						"var1": {},
						"var2": {},
					},
				},
			},
			&plans.Plan{
				VariableValues: map[string]plans.DynamicValue{
					"var1": mustDynamicVarVal("var1 value"),
				},
			},
			nil,
			InputValues{
				"var1": &InputValue{
					Value:      cty.StringVal("var1 value"),
					SourceType: ValueFromPlan,
				},
				"var2": &InputValue{
					Value:      cty.NilVal,
					SourceType: ValueFromPlan,
				},
			},
			[]tfdiags.Description{},
		},
		"backwards compatibility test - malformed plan variable value generates error": {
			&configs.Config{
				Module: &configs.Module{
					Variables: map[string]*configs.Variable{
						"var1": {},
					},
				},
			},
			&plans.Plan{
				VariableValues: map[string]plans.DynamicValue{
					"var1": []byte("test"),
				},
			},
			nil,
			InputValues{
				// Initially, before the introduction of mergePlanAndApplyVariables, the returned InputValues had no entries.
				// But due to the new way this is implemented, now these are returned with their nil value.
				// This is not an issue, functionally speaking, because the call to this new method must check
				// first for the diags and after user the returned values.
				"var1": &InputValue{SourceType: ValueFromPlan},
			},
			[]tfdiags.Description{
				{
					Summary: "Invalid variable value in plan",
					Detail:  `Invalid value for variable "var1" recorded in plan file: msgpack: invalid code=74 decoding array length.`,
				},
			},
		},
		// this should not happen in real life since this should be caught in an early stage of the execution,
		// but we check this here too to be sure that we don't allow other failures downstream
		"variable value in plan but not in config": {
			&configs.Config{
				Module: &configs.Module{
					Variables: map[string]*configs.Variable{},
				},
			},
			&plans.Plan{
				VariableValues: map[string]plans.DynamicValue{
					"var1": mustDynamicVarVal("var1 value"),
				},
			},
			nil,
			nil,
			[]tfdiags.Description{
				{
					Summary: "Missing variable in configuration",
					Detail:  `Plan variable "var1" not found in the given configuration`,
				},
			},
		},
		"variable value in apply opts but not in config": {
			&configs.Config{
				Module: &configs.Module{
					Variables: map[string]*configs.Variable{},
				},
			},
			&plans.Plan{
				VariableValues: map[string]plans.DynamicValue{},
			},
			&ApplyOpts{
				SetVariables: InputValues{
					"var1": &InputValue{
						Value:      cty.StringVal("var1 value"),
						SourceType: ValueFromCLIArg,
					},
				},
			},
			nil,
			[]tfdiags.Description{
				{
					Summary: "Missing variable in configuration",
					Detail:  `Variable "var1" not found in the given configuration`,
				},
			},
		},
		"value not in plan but in apply opts": {
			&configs.Config{
				Module: &configs.Module{
					Variables: map[string]*configs.Variable{
						"var1": {},
					},
				},
			},
			&plans.Plan{
				VariableValues: map[string]plans.DynamicValue{},
			},
			&ApplyOpts{
				SetVariables: InputValues{
					"var1": &InputValue{
						Value:      cty.StringVal("var1 value"),
						SourceType: ValueFromCLIArg,
					},
				},
			},
			InputValues{
				"var1": &InputValue{
					Value:      cty.StringVal("var1 value"),
					SourceType: ValueFromCLIArg,
				},
			},
			[]tfdiags.Description{},
		},
		"ephemeral with no default must have no value in plan but must have value in applyOpts": {
			&configs.Config{
				Module: &configs.Module{
					Variables: map[string]*configs.Variable{
						"var1": {},
					},
				},
			},
			&plans.Plan{
				VariableValues:     map[string]plans.DynamicValue{},
				EphemeralVariables: map[string]bool{"var1": true},
			},
			nil,
			InputValues{
				// This is returned strictly due to the way mergePlanAndApplyVariables is implemented. Check
				// the comment on the method
				"var1": &InputValue{
					SourceType: ValueFromPlan,
				},
			},
			[]tfdiags.Description{
				{
					Summary: "No value for required variable",
					Detail:  "Variable \"var1\" is configured as ephemeral. This type of variables need to be given a value during `tofu plan` and also during `tofu apply`.",
				},
			},
		},
		"ephemeral with default can have no value in applyOpts": {
			&configs.Config{
				Module: &configs.Module{
					Variables: map[string]*configs.Variable{
						"var1": {
							Default: cty.NullVal(cty.String),
						},
					},
				},
			},
			&plans.Plan{
				VariableValues:     map[string]plans.DynamicValue{},
				EphemeralVariables: map[string]bool{"var1": true},
			},
			nil,
			InputValues{
				"var1": &InputValue{
					Value:      cty.NilVal,
					SourceType: ValueFromPlan,
				},
			},
			[]tfdiags.Description{},
		},
		"value in applyOpts and the one in plan needs to be equal for non-ephemeral variables": {
			&configs.Config{
				Module: &configs.Module{
					Variables: map[string]*configs.Variable{
						"var1": {},
					},
				},
			},
			&plans.Plan{
				VariableValues: map[string]plans.DynamicValue{
					"var1": mustDynamicVarVal("value from plan"),
				},
			},
			&ApplyOpts{
				SetVariables: InputValues{
					"var1": &InputValue{
						Value:      cty.StringVal("value from apply opts"),
						SourceType: ValueFromCLIArg,
					},
				},
			},
			InputValues{
				"var1": &InputValue{
					Value:      cty.StringVal("value from plan"),
					SourceType: ValueFromPlan,
				},
			},
			[]tfdiags.Description{
				{
					Summary: "Mismatch between input and plan variable value",
					Detail:  `Value saved in the plan file for variable "var1" is different from the one given to the current command.`,
				},
			},
		},
		"successfully merge values for multiple variables": {
			&configs.Config{
				Module: &configs.Module{
					Variables: map[string]*configs.Variable{
						"regular_with_default": {
							Default: cty.NullVal(cty.String),
						},
						"regular_without_default": {},
						"ephemeral_with_default": {
							Default: cty.NullVal(cty.String),
						},
						"ephemeral_without_default": {},
					},
				},
			},
			&plans.Plan{
				VariableValues: map[string]plans.DynamicValue{
					"regular_without_default": mustDynamicVarVal("regular value from plan"),
				},
				EphemeralVariables: map[string]bool{
					"ephemeral_with_default":    true,
					"ephemeral_without_default": true,
				},
			},
			&ApplyOpts{
				SetVariables: InputValues{
					"ephemeral_without_default": &InputValue{
						Value:      cty.StringVal("ephemeral value from apply opts"),
						SourceType: ValueFromCLIArg,
					},
				},
			},
			InputValues{
				"regular_with_default": {
					Value:      cty.NilVal,
					SourceType: ValueFromPlan,
				},
				"regular_without_default": {
					Value:      cty.StringVal("regular value from plan"),
					SourceType: ValueFromPlan,
				},
				"ephemeral_with_default": {
					Value:      cty.NilVal,
					SourceType: ValueFromPlan,
				},
				"ephemeral_without_default": {
					Value:      cty.StringVal("ephemeral value from apply opts"),
					SourceType: ValueFromCLIArg,
				},
			},
			[]tfdiags.Description{},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			c := &Context{}
			got, gotDiags := c.mergePlanAndApplyVariables(tt.config, tt.plan, tt.opts)
			if diff := cmp.Diff(tt.expectedVals, got, cmpopts.EquateComparable(cty.Value{})); diff != "" {
				t.Errorf("invalid input values returned\n%s", diff)
			}
			diagsDesc := make([]tfdiags.Description, len(gotDiags))
			for i, diag := range gotDiags {
				diagsDesc[i] = diag.Description()
			}
			if diff := cmp.Diff(tt.expectedDiagsDetails, diagsDesc); diff != "" {
				t.Errorf("invalid diags returned\n%s", diff)
			}
		})
	}
}

func TestContext2Apply_enabledForResource(t *testing.T) {
	m := testModule(t, "apply-enabled-resource")
	p := &MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			ResourceTypes: map[string]providers.Schema{
				"test": {
					Block: &configschema.Block{
						Attributes: map[string]*configschema.Attribute{
							"name": {
								Type:     cty.String,
								Required: true,
							},
						},
					},
				},
			},
		},
	}
	p.PlanResourceChangeFn = func(prcr providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
		return providers.PlanResourceChangeResponse{
			PlannedState: prcr.ProposedNewState,
		}
	}
	p.ApplyResourceChangeFn = func(arcr providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
		return providers.ApplyResourceChangeResponse{
			NewState: arcr.PlannedState,
		}
	}
	tfCtx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})
	resourceInstAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test",
		Name: "test",
	}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)
	outputAddr := addrs.OutputValue{
		Name: "result",
	}.Absolute(addrs.RootModuleInstance)

	// We'll overwrite this after each round, but it starts empty.
	state := states.NewState()

	{
		t.Logf("First round: var.on = false")

		plan, diags := tfCtx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"on": &InputValue{
					Value: cty.False,
				},
			},
		})
		assertNoDiagnostics(t, diags)

		if change := plan.Changes.ResourceInstance(resourceInstAddr); change != nil {
			t.Fatalf("unexpected plan for %s (should be disabled)", resourceInstAddr)
		}

		newState, diags := tfCtx.Apply(context.Background(), plan, m, nil)
		assertNoDiagnostics(t, diags)

		if instState := newState.ResourceInstance(resourceInstAddr); instState != nil {
			t.Fatalf("unexpected state entry for %s (should be disabled)", resourceInstAddr)
		}

		outputState := newState.OutputValue(outputAddr)
		if outputState == nil {
			t.Errorf("missing state entry for %s", outputAddr)
		} else if got, want := outputState.Value, cty.StringVal("default"); !want.RawEquals(got) {
			t.Errorf("unexpected value for %s %#v; want null", outputAddr, got)
		}

		state = newState // "persist" the state for the next round
	}
	{
		t.Logf("Second round: var.on = true")

		plan, diags := tfCtx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"on": &InputValue{
					Value: cty.True,
				},
			},
		})
		assertNoDiagnostics(t, diags)

		change := plan.Changes.ResourceInstance(resourceInstAddr)
		if change == nil {
			t.Fatalf("missing plan for %s", resourceInstAddr)
		}
		if got, want := change.Action, plans.Create; got != want {
			t.Fatalf("plan for %s has wrong action %s; want %s", resourceInstAddr, got, want)
		}

		newState, diags := tfCtx.Apply(context.Background(), plan, m, nil)
		assertNoDiagnostics(t, diags)

		instState := newState.ResourceInstance(resourceInstAddr)
		if instState == nil {
			t.Fatalf("missing state entry for %s", resourceInstAddr)
		}

		outputState := newState.OutputValue(outputAddr)
		if outputState == nil {
			t.Errorf("missing state entry for %s", outputAddr)
		} else if got, want := outputState.Value, cty.StringVal("boop"); !want.RawEquals(got) {
			t.Errorf("unexpected value for %s\ngot:  %#v\nwant: %#v", outputAddr, got, want)
		}

		state = newState // "persist" the state for the next round
	}
	{
		t.Logf("Third round: var.on = false, again")

		plan, diags := tfCtx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"on": &InputValue{
					Value: cty.False,
				},
			},
		})
		assertNoDiagnostics(t, diags)

		change := plan.Changes.ResourceInstance(resourceInstAddr)
		if change == nil {
			t.Fatalf("missing plan for %s", resourceInstAddr)
		}
		if got, want := change.Action, plans.Delete; got != want {
			t.Fatalf("plan for %s has wrong action %s; want %s", resourceInstAddr, got, want)
		}

		if got, want := change.ActionReason, plans.ResourceInstanceDeleteBecauseEnabledFalse; got != want {
			t.Errorf("wrong action reason for %s %s; want %s", resourceInstAddr, got, want)
		}

		newState, diags := tfCtx.Apply(context.Background(), plan, m, nil)
		assertNoDiagnostics(t, diags)

		if instState := newState.ResourceInstance(resourceInstAddr); instState != nil {
			t.Fatalf("unexpected state entry for %s (should be disabled)", resourceInstAddr)
		}

		outputState := newState.OutputValue(outputAddr)
		if outputState == nil {
			t.Errorf("missing state entry for %s", outputAddr)
		} else if got, want := outputState.Value, cty.StringVal("default"); !want.RawEquals(got) {
			t.Errorf("unexpected value for %s\ngot:  %#v\nwant: %#v", outputAddr, got, want)
		}
	}
}

func TestContext2Apply_enabledForModule(t *testing.T) {
	m := testModule(t, "apply-enabled-module")

	provider := testProvider("test")
	provider.PlanResourceChangeFn = testDiffFn
	provider.ApplyResourceChangeFn = testApplyFn
	ps := map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("test"): testProviderFuncFixed(provider),
	}
	tfCtx := testContext2(t, &ContextOpts{
		Providers: ps,
	})

	resourceInstAddr := mustResourceInstanceAddr(`module.mod1.test_instance.a`)
	// We'll overwrite this after each round, but it starts empty.
	state := states.NewState()

	{
		t.Logf("First round: var.on = false")

		plan, diags := tfCtx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"on": &InputValue{
					Value: cty.False,
				},
			},
		})
		assertNoDiagnostics(t, diags)

		if instPlan := plan.Changes.ResourceInstance(resourceInstAddr); instPlan != nil {
			t.Fatalf("unexpected plan for %s (should be disabled)", resourceInstAddr)
		}

		newState, diags := tfCtx.Apply(context.Background(), plan, m, nil)
		assertNoDiagnostics(t, diags)

		if instState := newState.ResourceInstance(resourceInstAddr); instState != nil {
			t.Fatalf("unexpected state entry for %s (should be disabled)", resourceInstAddr)
		}
	}
	{
		t.Logf("Second round: var.on = true")

		plan, diags := tfCtx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"on": &InputValue{
					Value: cty.True,
				},
			},
		})
		assertNoDiagnostics(t, diags)

		instPlan := plan.Changes.ResourceInstance(resourceInstAddr)
		if instPlan == nil {
			t.Fatalf("missing plan for %s", resourceInstAddr)
		}
		if got, want := instPlan.Action, plans.Create; got != want {
			t.Fatalf("plan for %s has wrong action %s; want %s", resourceInstAddr, got, want)
		}

		newState, diags := tfCtx.Apply(context.Background(), plan, m, nil)
		assertNoDiagnostics(t, diags)

		instState := newState.ResourceInstance(resourceInstAddr)
		if instState == nil {
			t.Fatalf("missing state entry for %s", resourceInstAddr)
		}

		state = newState // "persist" the state for the next round
	}
	{
		t.Logf("Third round: var.on = false, again")

		plan, diags := tfCtx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"on": &InputValue{
					Value: cty.False,
				},
			},
		})
		assertNoDiagnostics(t, diags)

		instPlan := plan.Changes.ResourceInstance(resourceInstAddr)
		if instPlan == nil {
			t.Fatalf("missing plan for %s", resourceInstAddr)
		}
		if got, want := instPlan.Action, plans.Delete; got != want {
			t.Fatalf("plan for %s has wrong action %s; want %s", resourceInstAddr, got, want)
		}

		newState, diags := tfCtx.Apply(context.Background(), plan, m, nil)
		assertNoDiagnostics(t, diags)

		if instState := newState.ResourceInstance(resourceInstAddr); instState != nil {
			t.Fatalf("unexpected state entry for %s (should be disabled)", resourceInstAddr)
		}
	}
}
