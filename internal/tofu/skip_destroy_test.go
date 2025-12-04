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

type skipStateInstance struct {
	addr        string
	attrsJSON   string
	skipDestroy bool
	deposedKey  string            // if set, creates a deposed instance
	instanceKey addrs.InstanceKey // optional, defaults to NoKey
}

type skipExpectedChange struct {
	addr       string
	action     plans.Action
	deposedKey string
}

type skipDestroyTestCase struct {
	name            string
	config          string
	stateInstances  []skipStateInstance
	planMode        plans.Mode
	expectedChanges []skipExpectedChange
	expectPlanError bool
	// For apply tests
	runApply         bool
	expectApplyError bool
	expectEmptyState bool
}

func setupSkipTestState(t *testing.T, instances []skipStateInstance) *states.State {
	t.Helper()
	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)

	for _, inst := range instances {
		if inst.attrsJSON == "" {
			inst.attrsJSON = `{"id":"baz","type":"aws_instance"}`
		}

		instanceKey := inst.instanceKey
		if instanceKey == nil {
			instanceKey = addrs.NoKey
		}

		obj := &states.ResourceInstanceObjectSrc{
			Status:      states.ObjectReady,
			AttrsJSON:   []byte(inst.attrsJSON),
			SkipDestroy: inst.skipDestroy,
		}

		if inst.deposedKey != "" {
			root.SetResourceInstanceDeposed(
				mustResourceInstanceAddr(inst.addr).Resource,
				states.DeposedKey(inst.deposedKey),
				obj,
				mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
				instanceKey,
			)
		} else {
			root.SetResourceInstanceCurrent(
				mustResourceInstanceAddr(inst.addr).Resource,
				obj,
				mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
				instanceKey,
			)
		}
	}

	return state
}

func setupSkipTestDefaultContext(t *testing.T) (*Context, *MockProvider) {
	t.Helper()
	p := testProvider("aws")
	p.PlanResourceChangeFn = testDiffFn

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	return ctx, p
}

func runSkipDestroyTestCase(t *testing.T, tc skipDestroyTestCase) {
	t.Helper()

	m := testModuleInline(t, map[string]string{
		"main.tf": tc.config,
	})
	ctx, _ := setupSkipTestDefaultContext(t)
	state := setupSkipTestState(t, tc.stateInstances)

	planOpts := DefaultPlanOpts
	// We may also need to set destroy mode here
	if tc.planMode != 0 {
		planOpts = &PlanOpts{Mode: tc.planMode}
	}

	plan, diags := ctx.Plan(t.Context(), m, state, planOpts)

	if tc.expectPlanError {
		if !diags.HasErrors() {
			t.Fatal("expected plan error, got none")
		}
		return
	}

	if diags.HasErrors() {
		t.Fatalf("unexpected plan errors: %s", diags.Err())
	}

	verifySkipPlanChanges(t, plan, tc.expectedChanges)

	if tc.runApply {
		appliedState, applyDiags := ctx.Apply(t.Context(), plan, m, nil)

		if tc.expectApplyError {
			if !applyDiags.HasErrors() {
				t.Fatal("expected apply error, got none")
			}
		} else if applyDiags.HasErrors() {
			t.Fatalf("unexpected apply errors: %s", applyDiags.Err())
		}

		if tc.expectEmptyState && !appliedState.Empty() {
			t.Fatalf("expected empty state, got %s", appliedState.String())
		}
	}
}

func verifySkipPlanChanges(t *testing.T, plan *plans.Plan, expected []skipExpectedChange) {
	t.Helper()

	if len(expected) > 0 && len(plan.Changes.Resources) != len(expected) {
		t.Fatalf("expected number %d changes, got %d", len(expected), len(plan.Changes.Resources))
	}

	for _, exp := range expected {
		found := false
		for _, change := range plan.Changes.Resources {
			addrMatch := change.Addr.String() == exp.addr
			deposedMatch := string(change.DeposedKey) == exp.deposedKey

			if addrMatch && deposedMatch {
				found = true
				if change.Action != exp.action {
					t.Errorf("resource %s (deposed=%s): expected %s, got %s",
						exp.addr, exp.deposedKey, exp.action, change.Action)
				}
				break
			}
		}
		if !found {
			t.Errorf("did not find expected change for %s (deposed=%s)", exp.addr, exp.deposedKey)
		}
	}
}

// Destroy Mode Tests
func TestSkipDestroy_DestroyMode(t *testing.T) {
	tests := []skipDestroyTestCase{
		{
			// The simplest case: resource with SkipDestroy=true in state and in config.
			// Check that in destroy mode we forget the resource.
			name: "ConfigAndStateFlag",
			config: `
				resource "aws_instance" "foo" {
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", skipDestroy: true},
			},
			planMode: plans.DestroyMode,
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.Forget},
			},
		},
		{
			// The resource instance has no SkipDestroy attribute in the state, but it comes from config.
			// Check that in destroy mode we forget the resource and respect the latest config
			// corresponding to this resource.
			name: "ConfigFlagOnly",
			config: `
				resource "aws_instance" "foo" {
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", skipDestroy: false},
			},
			planMode: plans.DestroyMode,
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.Forget},
			},
		},
		{
			// This first updates the configuration and then runs destroy.
			// Since config has destroy=true (same as no attribute), this should result in resource deletion,
			// even though state has SkipDestroy=true.
			name: "StateFlagOnly_ShouldDelete",
			config: `
				resource "aws_instance" "foo" {
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", skipDestroy: true},
			},
			planMode: plans.DestroyMode,
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.Delete},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runSkipDestroyTestCase(t, tc)
		})
	}
}

func TestSkipDestroy_DestroyMode_Deposed(t *testing.T) {
	tests := []skipDestroyTestCase{
		{
			// Deposed objects are a special case, since they correspond to old instances.
			// Deposed objects in the state should respect the SkipDestroy attribute set in the state.
			// In case both (state and config) SkipDestroy attributes are present, if either is true,
			// the resource should be forgotten. This is for security, to avoid accidental deletions
			// of resource instances that once had SkipDestroy set.
			//
			// Here we have no config (empty module), so deposed instances respect their state attribute:
			// - 00000001 has SkipDestroy=false -> Delete
			// - 00000002 has SkipDestroy=true -> Forget
			name: "NoConfig_RespectStateFlags",
			config: `
				# Empty
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", deposedKey: "00000001", skipDestroy: false},
				{addr: "aws_instance.foo", deposedKey: "00000002", skipDestroy: true},
			},
			planMode: plans.DestroyMode,
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", deposedKey: "00000001", action: plans.Delete},
				{addr: "aws_instance.foo", deposedKey: "00000002", action: plans.Forget},
			},
		},
		{
			// Same as above, but here we have SkipDestroy=true from config and two deposed instances
			// in the state, one with SkipDestroy=true and one with SkipDestroy=false.
			// Either way, both should be forgotten in this case because the attribute from config takes precedence.
			name: "ConfigFlag_BothForget",
			config: `
				resource "aws_instance" "foo" {
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", deposedKey: "00000001", skipDestroy: false},
				{addr: "aws_instance.foo", deposedKey: "00000002", skipDestroy: true},
			},
			planMode: plans.DestroyMode,
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", deposedKey: "00000001", action: plans.Forget},
				{addr: "aws_instance.foo", deposedKey: "00000002", action: plans.Forget},
			},
		},
		{
			// Same as the two above, but here we have SkipDestroy=false from config (destroy=true) and still
			// two deposed instances in state, one with SkipDestroy=true and one with SkipDestroy=false.
			// We check that the deposed instance state attributes are respected and not overridden by
			// config when config says destroy=true. The state attribute protects the instance.
			name: "NegativeConfigFlag_RespectStateFlags",
			config: `
				resource "aws_instance" "foo" {
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", deposedKey: "00000001", skipDestroy: false},
				{addr: "aws_instance.foo", deposedKey: "00000002", skipDestroy: true},
			},
			planMode: plans.DestroyMode,
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", deposedKey: "00000001", action: plans.Delete},
				{addr: "aws_instance.foo", deposedKey: "00000002", action: plans.Forget},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runSkipDestroyTestCase(t, tc)
		})
	}
}

// TestSkipDestroy_DestroyMode_ErrorOnForgotten checks that applies in destroy mode
// return an error if there are forgotten resources.
func TestSkipDestroy_DestroyMode_ErrorOnForgotten(t *testing.T) {
	tc := []skipDestroyTestCase{
		{
			name: "ErrorOnForgotten",
			config: `
				resource "aws_instance" "foo" {
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", skipDestroy: true},
			},
			planMode: plans.DestroyMode,
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.Forget},
			},
			runApply:         true,
			expectApplyError: true,
			expectEmptyState: true,
		},
		{
			name: "NoErrorOnDelete",
			config: `
				resource "aws_instance" "foo" {
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo"},
			},
			planMode: plans.DestroyMode,
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.Delete},
			},
			runApply:         true,
			expectApplyError: false,
			expectEmptyState: true,
		},
	}

	for _, tc := range tc {
		t.Run(tc.name, func(t *testing.T) {
			runSkipDestroyTestCase(t, tc)
		})
	}
}

// Orphan Resource-Specific Tests
//
// An "orphan" resource is one that has been removed from configuration but
// is still present in state.

func TestSkipDestroy_Orphan(t *testing.T) {
	tests := []skipDestroyTestCase{
		{
			// Resource aws_instance.foo has been removed from configuration and is only present in state
			// If the SkipDestroy attribute is set in state, we should plan to Forget the resource.
			name: "StateFlag_Forget",
			config: `
				# Empty
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", skipDestroy: true},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.Forget},
			},
		},
		{
			// Sanity check: if the orphan resource does not have SkipDestroy attribute in state,
			// we should plan to Delete it.
			name: "NoStateFlag_Delete",
			config: `
				# Empty
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", skipDestroy: false},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.Delete},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runSkipDestroyTestCase(t, tc)
		})
	}
}

// TestSkipDestroy_Orphan_RemovedBlockOverrides tests that orphan resource's
// SkipDestroy attribute in state should be overridden by removed block in config.
func TestSkipDestroy_Orphan_RemovedBlockOverrides(t *testing.T) {
	tc := skipDestroyTestCase{
		name: "RemovedBlockOverridesStateFlag",
		config: `
			removed {
				from = aws_instance.skip_destroy_not_set
				lifecycle {
					destroy = true
				}
			}
			removed {
				from = aws_instance.skip_destroy_set
				lifecycle {
					destroy = false
				}
			}
		`,
		stateInstances: []skipStateInstance{
			// The corresponding removed block has destroy=true in config;
			// this should override SkipDestroy=true from state, resulting in Delete
			{addr: "aws_instance.skip_destroy_not_set", skipDestroy: true},
			// The corresponding removed block has destroy=false in config;
			// this should override SkipDestroy=false from state, resulting in Forget
			{addr: "aws_instance.skip_destroy_set", skipDestroy: false},
		},
		expectedChanges: []skipExpectedChange{
			{addr: "aws_instance.skip_destroy_not_set", action: plans.Delete},
			{addr: "aws_instance.skip_destroy_set", action: plans.Forget},
		},
	}

	t.Run(tc.name, func(t *testing.T) {
		runSkipDestroyTestCase(t, tc)
	})
}

// Resource Replacement Tests
//
// When the current resource instance has a corresponding config present, the differences
// between the state and config SkipDestroy values should not matter, since the newest configuration applies.

func TestSkipDestroy_Replace(t *testing.T) {
	tests := []skipDestroyTestCase{
		{
			// Basic case: resource with SkipDestroy=true in config (destroy=false).
			// Check that in replacement we plan to ForgetThenCreate the resource.
			name: "ForgetThenCreate",
			config: `
				resource "aws_instance" "foo" {
					require_new = "yes"
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", skipDestroy: false},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.ForgetThenCreate},
			},
		},
		{
			// We have a resource instance in state with SkipDestroy=true, but the
			// corresponding resource config exists with no destroy attribute (equivalent to destroy=true).
			// Value from config should overrule the state attribute, and the action should be DeleteThenCreate.
			name: "NoConfigFlag_DeleteThenCreate",
			config: `
				resource "aws_instance" "foo" {
					require_new = "yes"
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", skipDestroy: true},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.DeleteThenCreate},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runSkipDestroyTestCase(t, tc)
		})
	}
}

// Deposed Instance Tests
//
// Deposed instances are old instances that were replaced but not yet destroyed.
// We handle them a bit differently, since they correspond to old instances, we choose to be safe
// and respect the SkipDestroy attribute in state even if the current config has no destroy attribute set.
// Hence, SkipDestroy set to true either in state or config should retain the instance.

// tests the combinations of state/config attributes:
// 1. Deposed with attribute, config without -> Forget (state attribute protects)
// 2. Deposed without attribute, config with -> Forget
// 3. Both with attribute -> Forget
// 4. Neither with attribute -> Delete
// 5. Orphaned deposed instance with attribute -> Forget
func TestSkipDestroy_Deposed(t *testing.T) {
	tests := []skipDestroyTestCase{
		{
			// Config has destroy=true (no attribute). Deposed instance having SkipDestroy=true in state.
			// We respect the state attribute and forget the instance.
			name: "NoConfigFlag_Forget",
			config: `
				resource "aws_instance" "foo" {
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", deposedKey: "00000001", skipDestroy: true},
				{addr: "aws_instance.foo", skipDestroy: true},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", deposedKey: "00000001", action: plans.Forget},
				// This has the exact corresponding state instance and config, so no action
				{addr: "aws_instance.foo", action: plans.NoOp},
			},
		},
		{
			// Config has destroy=false. Deposed instance without state attribute should still
			// be forgotten because config attribute applies.
			name: "ConfigFlag_Forget",
			config: `
				resource "aws_instance" "foo" {
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", deposedKey: "00000001", skipDestroy: false},
				{addr: "aws_instance.foo", skipDestroy: true},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", deposedKey: "00000001", action: plans.Forget},
				// This has no exact state instance, so the plan creates a new one
				{addr: "aws_instance.foo", action: plans.NoOp},
			},
		},
		{
			// Deposed instance with SkipDestroy=true in state and in config.
			// Basic case: deposed instance should be forgotten.
			name: "StateFlag_Forget",
			config: `
				resource "aws_instance" "foo" {
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", deposedKey: "00000001", skipDestroy: true},
				{addr: "aws_instance.foo", skipDestroy: false},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", deposedKey: "00000001", action: plans.Forget},
				{addr: "aws_instance.foo", action: plans.NoOp},
			},
		},
		{
			// Config has destroy=true (no attribute). Deposed instance having SkipDestroy=false in state.
			// We must delete the instance.
			name: "NoConfigFlag_Delete",
			config: `
				resource "aws_instance" "foo" {
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", deposedKey: "00000001", skipDestroy: false},
				{addr: "aws_instance.foo", skipDestroy: false},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", deposedKey: "00000001", action: plans.Delete},
				{addr: "aws_instance.foo", action: plans.NoOp},
			},
		},
		{
			// Orphaned deposed instance (no config) with SkipDestroy=true should be forgotten.
			name: "Orphaned_Forget",
			config: `
				# Empty, use this for any test that requires a module but no config.
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", deposedKey: "00000001", skipDestroy: true},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", deposedKey: "00000001", action: plans.Forget},
			},
		},
		{
			// Orphaned deposed instance (no config) with SkipDestroy=false should be deleted.
			name: "Orphaned_Delete",
			config: `
				# Empty, use this for any test that requires a module but no config.
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", deposedKey: "00000001", skipDestroy: false},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", deposedKey: "00000001", action: plans.Delete},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runSkipDestroyTestCase(t, tc)
		})
	}
}

// Multi-Instance Tests (count/for_each/enabled)
//
// Tests for count and for_each resources with various attribute combinations.
// Config attribute: destroy=true/false in lifecycle block
// State attribute: SkipDestroy=true/false stored in state per instance
//
// Flag interaction rules:
// - For kept instances: config attribute determines the stored state attribute
// - When instances are removed/reduced state attribute protects instances even if config has destroy=true

func TestSkipDestroy_Enabled_Toggle(t *testing.T) {
	tests := []skipDestroyTestCase{
		{
			// Resource exists in state with SkipDestroy=false, but config has enabled=false and destroy=false.
			// The resource should be forgotten instead of deleted.
			name: "EnabledFalse_Forget",
			config: `
				resource "aws_instance" "foo" {
					lifecycle {
						enabled = false
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", skipDestroy: false},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.Forget},
			},
		},
		{
			// Resource exists in state with SkipDestroy=false, but config has enabled=false and destroy=true.
			// The resource should be deleted.
			name: "EnabledFalse_Delete",
			config: `
				resource "aws_instance" "foo" {
					lifecycle {
						enabled = false
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", skipDestroy: false},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.Delete},
			},
		},
		{
			// Resource exists in state with SkipDestroy=true, but config has enabled=false and destroy=true.
			// The resource should be forgotten because the state attribute protects it.
			// The only way to delete it is to apply destroy=true with enabled=true. And then change it to enabled=false if needed.
			name: "EnabledFalse_StateTrue_Forget",
			config: `
				resource "aws_instance" "foo" {
					lifecycle {
						enabled = false
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", skipDestroy: true},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.Forget},
			},
		},
		{
			// Simple case of toggling enabled back to true should create the resource
			name: "EnabledTrue_Create",
			config: `
				resource "aws_instance" "foo" {
					lifecycle {
						enabled = true
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.Create},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runSkipDestroyTestCase(t, tc)
		})
	}
}

func TestSkipDestroy_Count_Reduction(t *testing.T) {
	tcs := []skipDestroyTestCase{
		// Config has destroy=false (skip-destroy-count)
		{
			// Config: destroy=false, State: all SkipDestroy=true
			// Removed instance (index 2) should be Forget because both prevent destruction
			name: "ConfigFalse_StateTrue_OrphanForgotten",
			config: `
				resource "aws_instance" "foo" {
					count = 2
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo[0]", skipDestroy: true, instanceKey: addrs.IntKey(0)},
				{addr: "aws_instance.foo[1]", skipDestroy: true, instanceKey: addrs.IntKey(1)},
				{addr: "aws_instance.foo[2]", skipDestroy: true, instanceKey: addrs.IntKey(2)},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo[0]", action: plans.NoOp},
				{addr: "aws_instance.foo[1]", action: plans.NoOp},
				{addr: "aws_instance.foo[2]", action: plans.Forget},
			},
		},
		{
			// Config: destroy=false, State: all SkipDestroy=false
			// Removed instance should still be Forget because config destroy=false is enough
			name: "ConfigFalse_StateFalse_OrphanForgotten",
			config: `
				resource "aws_instance" "foo" {
					count = 2
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo[0]", skipDestroy: false, instanceKey: addrs.IntKey(0)},
				{addr: "aws_instance.foo[1]", skipDestroy: false, instanceKey: addrs.IntKey(1)},
				{addr: "aws_instance.foo[2]", skipDestroy: false, instanceKey: addrs.IntKey(2)},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo[0]", action: plans.NoOp},
				{addr: "aws_instance.foo[1]", action: plans.NoOp},
				{addr: "aws_instance.foo[2]", action: plans.Forget},
			},
		},
		{
			// Config: destroy=false, State: mixed attributes (index 2 has SkipDestroy=false)
			// Removed instance should still be Forget because config destroy=false should be enough
			// Quite similar to the previous test, but ensures mixed state attributes are handled correctly, unlikely though this case may be
			name: "ConfigFalse_StateMixed_OrphanForgotten",
			config: `
				resource "aws_instance" "foo" {
					count = 2
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo[0]", skipDestroy: true, instanceKey: addrs.IntKey(0)},
				{addr: "aws_instance.foo[1]", skipDestroy: false, instanceKey: addrs.IntKey(1)},
				{addr: "aws_instance.foo[2]", skipDestroy: false, instanceKey: addrs.IntKey(2)},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo[0]", action: plans.NoOp},
				{addr: "aws_instance.foo[1]", action: plans.NoOp},
				{addr: "aws_instance.foo[2]", action: plans.Forget},
			},
		},

		// Config has destroy=true
		{
			// Config: destroy=true, State: all SkipDestroy=false
			// Removed instance should be Deleted because both config and state say destroy
			name: "ConfigTrue_StateFalse_OrphanDeleted",
			config: `
				resource "aws_instance" "foo" {
					count = 2
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo[0]", skipDestroy: false, instanceKey: addrs.IntKey(0)},
				{addr: "aws_instance.foo[1]", skipDestroy: false, instanceKey: addrs.IntKey(1)},
				{addr: "aws_instance.foo[2]", skipDestroy: false, instanceKey: addrs.IntKey(2)},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo[0]", action: plans.NoOp},
				{addr: "aws_instance.foo[1]", action: plans.NoOp},
				{addr: "aws_instance.foo[2]", action: plans.Delete},
			},
		},
		{
			// Config: destroy=true, State: removed instance has SkipDestroy=true
			// State attribute protects the removed instance -> Forget instead of Delete
			//
			// This is a side effect of our design choice to respect state attributes for orphans.
			//
			// If for some reason they want to change the SkipDestroy value in state and then reduce count to cause resource destruction, they can do it in two steps.
			// First apply a config with the desired destroy value for all instances, then reduce count.
			// This use case seems rare enough to not complicate the logic further.
			name: "ConfigTrue_StateTrue_OrphanProtectedByStateFlag",
			config: `
				resource "aws_instance" "foo" {
					count = 2
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo[0]", skipDestroy: false, instanceKey: addrs.IntKey(0)},
				{addr: "aws_instance.foo[1]", skipDestroy: false, instanceKey: addrs.IntKey(1)},
				{addr: "aws_instance.foo[2]", skipDestroy: true, instanceKey: addrs.IntKey(2)},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo[0]", action: plans.NoOp},
				{addr: "aws_instance.foo[1]", action: plans.NoOp},
				{addr: "aws_instance.foo[2]", action: plans.Forget},
			},
		},
		{
			// Config: destroy=true, State: mixed attributes for removed instances
			// Multiple orphans with different state attributes - each respects its own attribute
			name: "ConfigTrue_StateMixed_MultipleOrphans",
			config: `
				resource "aws_instance" "foo" {
					count = 2
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo[0]", skipDestroy: false, instanceKey: addrs.IntKey(0)},
				{addr: "aws_instance.foo[1]", skipDestroy: false, instanceKey: addrs.IntKey(1)},
				// Two removed: one protected, one not
				{addr: "aws_instance.foo[2]", skipDestroy: true, instanceKey: addrs.IntKey(2)},
				{addr: "aws_instance.foo[3]", skipDestroy: false, instanceKey: addrs.IntKey(3)},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo[0]", action: plans.NoOp},
				{addr: "aws_instance.foo[1]", action: plans.NoOp},
				{addr: "aws_instance.foo[2]", action: plans.Forget}, // protected by state attribute
				{addr: "aws_instance.foo[3]", action: plans.Delete}, // not protected
			},
		},

		// Destroy mode with count
		{
			// Destroy mode: Config destroy=false, all should be forgotten and apply in destroy mode should return error
			name: "DestroyMode_ConfigFalse_AllForgotten",
			config: `
				resource "aws_instance" "foo" {
					count = 2
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo[0]", skipDestroy: true, instanceKey: addrs.IntKey(0)},
				{addr: "aws_instance.foo[1]", skipDestroy: true, instanceKey: addrs.IntKey(1)},
			},
			planMode: plans.DestroyMode,
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo[0]", action: plans.Forget},
				{addr: "aws_instance.foo[1]", action: plans.Forget},
			},
			runApply:         true,
			expectEmptyState: true,
			expectApplyError: true,
		},
		{
			// Destroy mode: Config destroy=true, state attributes mixed
			// In destroy mode, config destroy=true takes precedence since it still corresponds to current instances in state.
			// Meaning we will apply the latest config and then run destroy, resulting in deletions.
			name: "DestroyMode_ConfigTrue_AllDeleted",
			config: `
				resource "aws_instance" "foo" {
					count = 2
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo[0]", skipDestroy: true, instanceKey: addrs.IntKey(0)},
				{addr: "aws_instance.foo[1]", skipDestroy: false, instanceKey: addrs.IntKey(1)},
			},
			planMode: plans.DestroyMode,
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo[0]", action: plans.Delete}, // config destroy=true overrides state attribute
				{addr: "aws_instance.foo[1]", action: plans.Delete},
			},
			runApply:         true,
			expectEmptyState: true,
			expectApplyError: false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			runSkipDestroyTestCase(t, tc)
		})
	}
}

func TestSkipDestroy_ForEach(t *testing.T) {
	tests := []skipDestroyTestCase{
		{
			// Config: destroy=false, State: all SkipDestroy=true
			// Resource instance c should be Forgotten
			name: "ConfigFalse_StateTrue_OrphanForgotten",
			config: `
				resource "aws_instance" "foo" {
					for_each = toset(["a", "b"])
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: `aws_instance.foo["a"]`, skipDestroy: true, instanceKey: addrs.StringKey("a")},
				{addr: `aws_instance.foo["b"]`, skipDestroy: true, instanceKey: addrs.StringKey("b")},
				{addr: `aws_instance.foo["c"]`, skipDestroy: true, instanceKey: addrs.StringKey("c")},
			},
			expectedChanges: []skipExpectedChange{
				{addr: `aws_instance.foo["a"]`, action: plans.NoOp},
				{addr: `aws_instance.foo["b"]`, action: plans.NoOp},
				{addr: `aws_instance.foo["c"]`, action: plans.Forget},
			},
		},
		{
			// Config: destroy=false, State: removed instance c has SkipDestroy=false
			// Config destroy=false still takes precedence -> Forget
			name: "ConfigFalse_StateFalse_OrphanForgotten",
			config: `
				resource "aws_instance" "foo" {
					for_each = toset(["a", "b"])
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: `aws_instance.foo["a"]`, skipDestroy: false, instanceKey: addrs.StringKey("a")},
				{addr: `aws_instance.foo["b"]`, skipDestroy: false, instanceKey: addrs.StringKey("b")},
				{addr: `aws_instance.foo["c"]`, skipDestroy: false, instanceKey: addrs.StringKey("c")},
			},
			expectedChanges: []skipExpectedChange{
				{addr: `aws_instance.foo["a"]`, action: plans.NoOp},
				{addr: `aws_instance.foo["b"]`, action: plans.NoOp},
				{addr: `aws_instance.foo["c"]`, action: plans.Forget},
			},
		},
		{
			// State has keys a, c. Config has keys a, b.
			// instance c is removed (Forget), b is added (Create), a is kept (NoOp).
			name: "ConfigFalse_AddAndRemove",
			config: `
				resource "aws_instance" "foo" {
					for_each = toset(["a", "b"])
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: `aws_instance.foo["a"]`, skipDestroy: true, instanceKey: addrs.StringKey("a")},
				{addr: `aws_instance.foo["c"]`, skipDestroy: true, instanceKey: addrs.StringKey("c")},
			},
			expectedChanges: []skipExpectedChange{
				{addr: `aws_instance.foo["a"]`, action: plans.NoOp},
				{addr: `aws_instance.foo["b"]`, action: plans.Create},
				{addr: `aws_instance.foo["c"]`, action: plans.Forget},
			},
		},

		{
			// Config: destroy=true, State: all SkipDestroy=false
			// Removed instance c should be Deleted
			name: "ConfigTrue_StateFalse_OrphanDeleted",
			config: `
				resource "aws_instance" "foo" {
					for_each = toset(["a", "b"])
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: `aws_instance.foo["a"]`, skipDestroy: false, instanceKey: addrs.StringKey("a")},
				{addr: `aws_instance.foo["b"]`, skipDestroy: false, instanceKey: addrs.StringKey("b")},
				{addr: `aws_instance.foo["c"]`, skipDestroy: false, instanceKey: addrs.StringKey("c")},
			},
			expectedChanges: []skipExpectedChange{
				{addr: `aws_instance.foo["a"]`, action: plans.NoOp},
				{addr: `aws_instance.foo["b"]`, action: plans.NoOp},
				{addr: `aws_instance.foo["c"]`, action: plans.Delete},
			},
		},
		{
			// Config: destroy=true, State: removed instance c has SkipDestroy=true
			// State attribute protects the removed instance -> Forget instead of Delete
			name: "ConfigTrue_StateTrue_OrphanProtected",
			config: `
				resource "aws_instance" "foo" {
					for_each = toset(["a", "b"])
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: `aws_instance.foo["a"]`, skipDestroy: false, instanceKey: addrs.StringKey("a")},
				{addr: `aws_instance.foo["b"]`, skipDestroy: false, instanceKey: addrs.StringKey("b")},
				{addr: `aws_instance.foo["c"]`, skipDestroy: true, instanceKey: addrs.StringKey("c")},
			},
			expectedChanges: []skipExpectedChange{
				{addr: `aws_instance.foo["a"]`, action: plans.NoOp},
				{addr: `aws_instance.foo["b"]`, action: plans.NoOp},
				{addr: `aws_instance.foo["c"]`, action: plans.Forget},
			},
		},
		{
			// Config: destroy=true, State: mixed attributes for removed instances
			// Multiple orphans with different state attributes - each respects its own attributes
			name: "ConfigTrue_StateMixed_MultipleOrphans",
			config: `
				resource "aws_instance" "foo" {
					for_each = toset(["a", "b"])
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: `aws_instance.foo["a"]`, skipDestroy: false, instanceKey: addrs.StringKey("a")},
				{addr: `aws_instance.foo["b"]`, skipDestroy: false, instanceKey: addrs.StringKey("b")},
				// Two orphans with different state attributes
				{addr: `aws_instance.foo["c"]`, skipDestroy: true, instanceKey: addrs.StringKey("c")},
				{addr: `aws_instance.foo["d"]`, skipDestroy: false, instanceKey: addrs.StringKey("d")},
			},
			expectedChanges: []skipExpectedChange{
				{addr: `aws_instance.foo["a"]`, action: plans.NoOp},
				{addr: `aws_instance.foo["b"]`, action: plans.NoOp},
				// Orphans
				{addr: `aws_instance.foo["c"]`, action: plans.Forget},
				{addr: `aws_instance.foo["d"]`, action: plans.Delete},
			},
		},

		// Keys changed to a new set
		{
			// State has keys x, y. Config has keys a, b.
			// We should Forget the removed instances (x, y) because config destroy=false,
			name: "ConfigFalse_CompleteKeyChange",
			config: `
				resource "aws_instance" "foo" {
					for_each = toset(["a", "b"])
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: `aws_instance.foo["x"]`, skipDestroy: true, instanceKey: addrs.StringKey("x"),
					attrsJSON: `{"id":"baz","require_new":"new","type":"aws_instance"}`},
				{addr: `aws_instance.foo["y"]`, skipDestroy: false, instanceKey: addrs.StringKey("y"),
					attrsJSON: `{"id":"baz","require_new":"new","type":"aws_instance"}`},
			},
			expectedChanges: []skipExpectedChange{
				// config destroy=false -> Both get Forgotten
				{addr: `aws_instance.foo["x"]`, action: plans.Forget},
				{addr: `aws_instance.foo["y"]`, action: plans.Forget},
				// New instances get created
				{addr: `aws_instance.foo["a"]`, action: plans.Create},
				{addr: `aws_instance.foo["b"]`, action: plans.Create},
			},
		},
		{
			// State has keys x, y. Config has keys a, b with destroy=true.
			// Removed instances (now orphans) respect their individual state attributes.
			name: "ConfigTrue_CompleteKeyChange_MixedStateFlags",
			config: `
				resource "aws_instance" "foo" {
					for_each = toset(["a", "b"])
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: `aws_instance.foo["x"]`, skipDestroy: true, instanceKey: addrs.StringKey("x"),
					attrsJSON: `{"id":"baz","require_new":"new","type":"aws_instance"}`},
				{addr: `aws_instance.foo["y"]`, skipDestroy: false, instanceKey: addrs.StringKey("y"),
					attrsJSON: `{"id":"baz","require_new":"new","type":"aws_instance"}`},
			},
			expectedChanges: []skipExpectedChange{
				{addr: `aws_instance.foo["x"]`, action: plans.Forget},
				{addr: `aws_instance.foo["y"]`, action: plans.Delete},
				{addr: `aws_instance.foo["a"]`, action: plans.Create},
				{addr: `aws_instance.foo["b"]`, action: plans.Create},
			},
		},

		// Destroy mode with for_each
		{
			// Destroy mode: Config destroy=false, all should be forgotten
			name: "DestroyMode_ConfigFalse_AllForgotten",
			config: `
				resource "aws_instance" "foo" {
					for_each = toset(["a", "b"])
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: `aws_instance.foo["a"]`, skipDestroy: true, instanceKey: addrs.StringKey("a"),
					attrsJSON: `{"id":"baz","require_new":"new","type":"aws_instance"}`},
				{addr: `aws_instance.foo["b"]`, skipDestroy: true, instanceKey: addrs.StringKey("b"),
					attrsJSON: `{"id":"baz","require_new":"new","type":"aws_instance"}`},
			},
			planMode: plans.DestroyMode,
			expectedChanges: []skipExpectedChange{
				{addr: `aws_instance.foo["a"]`, action: plans.Forget},
				{addr: `aws_instance.foo["b"]`, action: plans.Forget},
			},

			runApply:         true,
			expectEmptyState: true,
			expectApplyError: true,
		},
		{
			// Destroy mode: Config destroy=true, state attributes mixed
			// In destroy mode, config destroy=true takes precedence - all current instances deleted
			// Orphaned instances respect their state attributes
			name: "DestroyMode_ConfigTrue_AllDeleted",
			config: `
				resource "aws_instance" "foo" {
					for_each = toset(["a", "b"])
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: `aws_instance.foo["a"]`, skipDestroy: true, instanceKey: addrs.StringKey("a"),
					attrsJSON: `{"id":"baz","require_new":"new","type":"aws_instance"}`},
				{addr: `aws_instance.foo["b"]`, skipDestroy: false, instanceKey: addrs.StringKey("b"),
					attrsJSON: `{"id":"baz","require_new":"new","type":"aws_instance"}`},
				{addr: `aws_instance.foo["c"]`, skipDestroy: true, instanceKey: addrs.StringKey("c"),
					attrsJSON: `{"id":"baz","require_new":"new","type":"aws_instance"}`},
				{addr: `aws_instance.foo["d"]`, skipDestroy: false, instanceKey: addrs.StringKey("d"),
					attrsJSON: `{"id":"baz","require_new":"new","type":"aws_instance"}`},
			},
			planMode: plans.DestroyMode,
			expectedChanges: []skipExpectedChange{
				{addr: `aws_instance.foo["a"]`, action: plans.Delete},
				{addr: `aws_instance.foo["b"]`, action: plans.Delete},

				{addr: `aws_instance.foo["c"]`, action: plans.Forget}, // protected by state attribute
				{addr: `aws_instance.foo["d"]`, action: plans.Delete}, // not protected
			},
			runApply:         true,
			expectEmptyState: true,
			expectApplyError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runSkipDestroyTestCase(t, tc)
		})
	}
}

// Removed Block Tests

// The removed block takes precedence over both resource config and state attributes.
// Interaction with removed block in config:
// 1. Resource destroy=true, Removed destroy=true -> Delete
// 2. Resource destroy=true, Removed destroy=false -> Forget
// 3. Resource destroy=false, Removed destroy=true -> Delete
// 4. Resource destroy=false, Removed destroy=false -> Forget

func TestSkipDestroy_RemovedBlock(t *testing.T) {
	tests := []skipDestroyTestCase{
		{
			// State SkipDestroy=true,
			// Config: removed block with destroy=true
			// Expected: Delete
			name: "RemovedDestroyTrue_Delete",
			config: `
				removed {
					from = aws_instance.foo
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", skipDestroy: true},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.Delete},
			},
		},

		{
			// State SkipDestroy=true,			// Config: removed block with destroy=false
			// Expected: Forget
			name: "RemovedDestroyFalse_Forget",
			config: `
				removed {
					from = aws_instance.foo
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", skipDestroy: true},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.Forget},
			},
		},

		{
			// State SkipDestroy=false,
			// Config: removed block with destroy=true
			// Expected: Delete
			name: "RemovedDestroyTrue_Delete_StateFalse",
			config: `
				removed {
					from = aws_instance.foo
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", skipDestroy: false},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.Delete},
			},
		},

		{
			// State SkipDestroy=false,			// Config: removed block with destroy=false
			// Expected: Forget
			name: "RemovedDestroyFalse_Forget_StateFalse",
			config: `
				removed {
					from = aws_instance.foo
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", skipDestroy: false},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", action: plans.Forget},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runSkipDestroyTestCase(t, tc)
		})
	}
}

// TestSkipDestroy_RemovedBlock_DeposedInstance tests removed block interactions with deposed instances.
// Deposed instances follow the same rules as current instances for removed blocks.
func TestSkipDestroy_RemovedBlock_DeposedInstance(t *testing.T) {
	tests := []skipDestroyTestCase{
		{
			// Config: removed block with destroy=true, State: Deposed with SkipDestroy=true
			// removed block takes precedence -> Delete
			name: "DeposedRemovedDestroyTrue_Delete",
			config: `
				removed {
					from = aws_instance.foo
					lifecycle {
						destroy = true
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", deposedKey: "00000001", skipDestroy: true},
				{addr: "aws_instance.foo", deposedKey: "00000002", skipDestroy: false},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", deposedKey: "00000001", action: plans.Delete},
				{addr: "aws_instance.foo", deposedKey: "00000002", action: plans.Delete},
			},
		},

		{
			// Config: removed block with destroy=false, State: Deposed with SkipDestroy=false
			// removed block takes precedence -> Forget
			name: "DeposedRemovedDestroyFalse_Forget",
			config: `
				removed {
					from = aws_instance.foo
					lifecycle {
						destroy = false
					}
				}
			`,
			stateInstances: []skipStateInstance{
				{addr: "aws_instance.foo", deposedKey: "00000001", skipDestroy: true},
				{addr: "aws_instance.foo", deposedKey: "00000002", skipDestroy: false},
			},
			expectedChanges: []skipExpectedChange{
				{addr: "aws_instance.foo", deposedKey: "00000001", action: plans.Forget},
				{addr: "aws_instance.foo", deposedKey: "00000002", action: plans.Forget},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runSkipDestroyTestCase(t, tc)
		})
	}
}

// TestSkipDestroy_ConfigChange_UpdatesState verifies the full cycle:
// Apply with destroy=false and verify attribute is set in state
// Another apply with destroy=true and verify that the attribute is removed from state
func TestSkipDestroy_ConfigChange_UpdatesState(t *testing.T) {
	// Apply with destroy=false
	m := testModule(t, "skip-destroy")
	ctx, _ := setupSkipTestDefaultContext(t)
	state := states.NewState()

	plan, _ := ctx.Plan(t.Context(), m, state, DefaultPlanOpts)
	appliedState, _ := ctx.Apply(t.Context(), plan, m, nil)

	if !appliedState.RootModule().Resources["aws_instance.foo"].Instance(addrs.NoKey).Current.SkipDestroy {
		t.Fatal("SkipDestroy attribute not set initially")
	}

	// Change config to destroy=true (using "simple" module which has default destroy=true)
	// and apply. This should remove the attribute.
	m2 := testModule(t, "simple")
	plan2, diags := ctx.Plan(t.Context(), m2, appliedState, DefaultPlanOpts)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	appliedState2, diags := ctx.Apply(t.Context(), plan2, m2, nil)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	if appliedState2.RootModule().Resources["aws_instance.foo"].Instance(addrs.NoKey).Current.SkipDestroy {
		t.Fatal("SkipDestroy attribute not removed after config change")
	}
}
