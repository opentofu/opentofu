// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
)

func TestSkipDestroy_planAndApply_stateFlagChecks(t *testing.T) {
	m := testModule(t, "skip-destroy")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn

	state := states.NewState()

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(t.Context(), m, state, DefaultPlanOpts)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	if len(plan.Changes.Resources) != 1 {
		t.Fatalf("expected 1 resource; got %d", len(plan.Changes.Resources))
	}

	change := plan.Changes.Resources[0]
	if change.Action != plans.Create {
		t.Fatalf("\n%-15s: %10q\n%-15s: %10q\n", "expected action", plans.Create, "got", change.Action)
	}

	if !plan.PlannedState.RootModule().Resources["aws_instance.foo"].Instance(addrs.NoKey).Current.SkipDestroy {
		t.Fatal("skip_destroy wasn't set correctly in state")
	}

	appliedState, diags := ctx.Apply(t.Context(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}
	if !appliedState.RootModule().Resources["aws_instance.foo"].Instance(addrs.NoKey).Current.SkipDestroy {
		t.Fatal("skip_destroy wasn't set correctly in state")
	}
}

func TestSkipDestroy_resourceReplace(t *testing.T) {
	m := testModule(t, "skip-destroy")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"baz","require_new":"old","type":"aws_instance"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(t.Context(), m, state, DefaultPlanOpts)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	if len(plan.Changes.Resources) != 1 {
		t.Fatalf("expected 1 resource; got %d", len(plan.Changes.Resources))
	}

	change := plan.Changes.Resources[0]
	if change.Action != plans.ForgetThenCreate {
		t.Fatalf("\n%-15s: %10q\n%-15s: %10q\n", "expected action", plans.ForgetThenCreate, "got", change.Action)
	}

	appliedState, diags := ctx.Apply(t.Context(), plan, m, nil)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	// Applied state after replace with skip destroy set in the config should contain a single resource with the skip_destroy flag set
	if !appliedState.RootModule().Resources["aws_instance.foo"].Instance(addrs.NoKey).Current.SkipDestroy {
		t.Fatal("skip_destroy wasn't set correctly in state")
	}

	if len(appliedState.RootModule().Resources) != 1 {
		t.Fatalf("expected 1 resource; got %d", len(appliedState.RootModule().Resources))
	}
}

func TestSkipDestroy_destroy(t *testing.T) {
	m := testModule(t, "skip-destroy")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:      states.ObjectReady,
			AttrsJSON:   []byte(`{"id":"baz","require_new":"old","type":"aws_instance"}`),
			SkipDestroy: true,
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(t.Context(), m, state, &PlanOpts{
		Mode: plans.DestroyMode,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	if len(plan.Changes.Resources) != 1 {
		t.Fatalf("expected 1 resource; got %d", len(plan.Changes.Resources))
	}

	change := plan.Changes.Resources[0]
	if change.Action != plans.Forget {
		t.Fatalf("\n%-15s: %10q\n%-15s: %10q\n", "expected action", plans.Forget, "got", change.Action)
	}

	appliedState, diags := ctx.Apply(t.Context(), plan, m, nil)
	if !diags.HasErrors() {
		t.Fatalf("expected errors when leaving behind forgotten resource instance; got none")
	}

	if !appliedState.Empty() {
		t.Fatalf("\nexpected plannedState to be empty; got %q\n", appliedState.String())
	}

}

func TestSkipDestroy_removedFromConfig(t *testing.T) {
	m := testModule(t, "empty")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:      states.ObjectReady,
			AttrsJSON:   []byte(`{"id":"baz","require_new":"old","type":"aws_instance"}`),
			SkipDestroy: true,
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(t.Context(), m, state, DefaultPlanOpts)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	if len(plan.Changes.Resources) != 1 {
		t.Fatalf("expected 1 resource; got %d", len(plan.Changes.Resources))
	}

	change := plan.Changes.Resources[0]
	if change.Action != plans.Forget {
		t.Fatalf("\n%-15s: %10q\n%-15s: %10q\n", "expected action", plans.Forget, "got", change.Action)
	}

}

func TestSkipDestroy_plan_deposedAndOrphaned(t *testing.T) {
	m := testModule(t, "empty")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:      states.ObjectReady,
			AttrsJSON:   []byte(`{"id":"baz","require_new":"old","type":"aws_instance"}`),
			SkipDestroy: true,
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceDeposed(
		mustResourceInstanceAddr("aws_instance.foo").Resource,
		states.DeposedKey("00000001"),
		&states.ResourceInstanceObjectSrc{
			Status:      states.ObjectReady,
			AttrsJSON:   []byte(`{"id":"baz","require_new":"old","type":"aws_instance"}`),
			SkipDestroy: true,
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(t.Context(), m, state, DefaultPlanOpts)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	if len(plan.Changes.Resources) != 2 {
		t.Fatalf("expected 2 resource; got %d", len(plan.Changes.Resources))
	}

	for _, change := range plan.Changes.Resources {
		if change.Action != plans.Forget {
			t.Fatalf("\n%-15s: %10q\n%-15s: %10q\n", "expected action", plans.Forget, "got", change.Action)
		}
	}
	if !plan.PlannedState.Empty() {
		t.Fatalf("expected planned state to be empty; got %q\n", plan.PlannedState.String())
	}
}

// In case we have a deposed instance without `skip_destroy` set, but the corresponding config block has `destroy=false`
// We need to retain the deposed instance to be safe
func TestSkipDestroy_plan_deposedAndInConfig_deposedWithoutFlag(t *testing.T) {
	m := testModule(t, "skip-destroy")
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("aws_instance.foo").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"baz","require_new":"new","type":"aws_instance"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceDeposed(
		mustResourceInstanceAddr("aws_instance.foo").Resource,
		states.DeposedKey("00000001"),
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"baz","require_new":"old","type":"aws_instance"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(t.Context(), m, state, DefaultPlanOpts)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	if len(plan.Changes.Resources) != 2 {
		t.Fatalf("expected 2 resource; got %d", len(plan.Changes.Resources))
	}

	for _, change := range plan.Changes.Resources {
		if change.DeposedKey.String() != "" {
			// Check we forget the deposed instance
			if change.Action != plans.Forget {
				t.Fatalf("\n%-15s: %10q\n%-15s: %10q\n", "expected action", plans.Forget, "got", change.Action)
			}
		} else {
			// For the resource still in config we should have no-op
			if change.Action != plans.NoOp {
				t.Fatalf("expected no op; got %q\n", change.Action)
			}
		}
	}
}
