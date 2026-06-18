// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
	"github.com/opentofu/opentofu/internal/plans"
)

// TestExecGraphBuilder_ManagedResourceInstanceSubgraph is a unit test for
// the ManagedResourceInstanceSubgraph method in particular, focused only on
// the items and relationships that function produces.
//
// Interactions between this method and others should be tested elsewhere.
func TestExecGraphBuilder_ManagedResourceInstanceSubgraph(t *testing.T) {
	// instAddr is the resource instance address that each test should use
	// for the resource instance object whose result is returned from the
	// "Build" function. We set the return value as the result for this
	// resource instance so that it'll appear in the graph DebugRepr for
	// comparison.
	instAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test",
		Name: "placeholder",
	}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)

	tests := map[string]struct {
		// Build has an awkward signature because this test was written for
		// an older prototype design of ManagedResourceInstanceSubgraph that
		// didn't return as many results.
		// TODO: Find a different way to structure this test so that we can
		// confine this complexity only to the testing loop below and not
		// to each individual test case.
		Build func(
			b *execGraphBuilder,
		) (
			execgraph.ResourceInstanceResultRef,
			execgraph.ResourceInstanceResultRef,
			func(execgraph.AnyResultRef),
			func(execgraph.AnyResultRef),
		)
		WantRepr string
	}{
		"create": {
			func(b *execGraphBuilder) (execgraph.ResourceInstanceResultRef, execgraph.ResourceInstanceResultRef, func(execgraph.AnyResultRef), func(execgraph.AnyResultRef)) {
				return b.ManagedResourceInstanceSubgraph(
					&plans.ResourceInstanceChange{
						Addr:        instAddr,
						PrevRunAddr: instAddr,
						Change: plans.Change{
							Action: plans.Create,
							Before: cty.NullVal(cty.EmptyObject),
							After:  cty.EmptyObjectVal,
						},
					},
					replaceDestroyThenCreate,
				)
			},
			`
				v[0] = cty.EmptyObjectVal;
				
				r[0] = ResourceInstanceDesired(test.placeholder, await());
				r[1] = ManagedFinalPlan(r[0], nil, v[0]);
				r[2] = ManagedApply(r[1], nil, await());

				test.placeholder = r[2];
			`,
		},
		"update": {
			func(b *execGraphBuilder) (execgraph.ResourceInstanceResultRef, execgraph.ResourceInstanceResultRef, func(execgraph.AnyResultRef), func(execgraph.AnyResultRef)) {
				return b.ManagedResourceInstanceSubgraph(
					&plans.ResourceInstanceChange{
						Addr:        instAddr,
						PrevRunAddr: instAddr,
						Change: plans.Change{
							Action: plans.Update,
							Before: cty.StringVal("before"),
							After:  cty.StringVal("after"),
						},
					},
					replaceDestroyThenCreate,
				)
			},
			`
				v[0] = cty.StringVal("after");
				
				r[0] = ResourceInstancePrior(test.placeholder, await());
				r[1] = ResourceInstanceDesired(test.placeholder, await());
				r[2] = ManagedFinalPlan(r[1], r[0], v[0]);
				r[3] = ManagedApply(r[2], nil, await());

				test.placeholder = r[3];
			`,
		},
		"update with move": {
			func(b *execGraphBuilder) (execgraph.ResourceInstanceResultRef, execgraph.ResourceInstanceResultRef, func(execgraph.AnyResultRef), func(execgraph.AnyResultRef)) {
				oldInstAddr := addrs.Resource{
					Mode: addrs.ManagedResourceMode,
					Type: "test",
					Name: "old",
				}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)
				return b.ManagedResourceInstanceSubgraph(
					&plans.ResourceInstanceChange{
						Addr:        instAddr,
						PrevRunAddr: oldInstAddr,
						Change: plans.Change{
							Action: plans.Update,
							Before: cty.StringVal("before"),
							After:  cty.StringVal("after"),
						},
					},
					replaceDestroyThenCreate,
				)
			},
			`
				v[0] = cty.StringVal("after");

				r[0] = ResourceInstancePrior(test.old, await());
				r[1] = ManagedChangeAddr(r[0], test.placeholder);
				r[2] = ResourceInstanceDesired(test.placeholder, await());
				r[3] = ManagedFinalPlan(r[2], r[1], v[0]);
				r[4] = ManagedApply(r[3], nil, await());

				test.placeholder = r[4];
			`,
		},
		"delete": {
			func(b *execGraphBuilder) (execgraph.ResourceInstanceResultRef, execgraph.ResourceInstanceResultRef, func(execgraph.AnyResultRef), func(execgraph.AnyResultRef)) {
				return b.ManagedResourceInstanceSubgraph(
					&plans.ResourceInstanceChange{
						Addr:        instAddr,
						PrevRunAddr: instAddr,
						Change: plans.Change{
							Action: plans.Delete,
							Before: cty.EmptyObjectVal,
							After:  cty.NullVal(cty.EmptyObject),
						},
					},
					replaceDestroyThenCreate,
				)
			},
			`
				v[0] = cty.NullVal(cty.EmptyObject);
				
				r[0] = ResourceInstancePrior(test.placeholder, await());
				r[1] = ManagedFinalPlan(nil, r[0], v[0]);
				r[2] = ManagedApply(r[1], nil, await());

				test.placeholder = r[0];
			`,
			// NOTE: The result for an object being deleted is set to the
			// prior state, which is counter-intuitive but correct because:
			// - In normal planning mode there can't be configuration references
			//   to something that is being deleted anyway, and so it doesn't
			//   really matter which value is treated as the result.
			// - In destroy planning mode ephemeral objects like provider
			//   instances are expected to be able to rely on the prior state
			//   of resource instances that are being planned for deletion,
			//   and so this wiring creates that effect during the apply phase.
		},
		"delete then create": {
			func(b *execGraphBuilder) (execgraph.ResourceInstanceResultRef, execgraph.ResourceInstanceResultRef, func(execgraph.AnyResultRef), func(execgraph.AnyResultRef)) {
				return b.ManagedResourceInstanceSubgraph(
					&plans.ResourceInstanceChange{
						Addr:        instAddr,
						PrevRunAddr: instAddr,
						Change: plans.Change{
							Action: plans.DeleteThenCreate,
							Before: cty.StringVal("before"),
							After:  cty.StringVal("after"),
						},
					},
					replaceDestroyThenCreate,
				)
			},
			`
				v[0] = cty.StringVal("after");
				v[1] = cty.NullVal(cty.String);
				
				r[0] = ResourceInstancePrior(test.placeholder, await());
				r[1] = ResourceInstanceDesired(test.placeholder, await());
				r[2] = ManagedFinalPlan(r[1], nil, v[0]);
				r[3] = ManagedFinalPlan(nil, r[0], v[1]);
				r[4] = ManagedApply(r[3], nil, await());
				r[5] = ManagedApply(r[2], nil, await(r[4]));

				test.placeholder = r[5];
			`,
		},
		"delete then create with move": {
			func(b *execGraphBuilder) (execgraph.ResourceInstanceResultRef, execgraph.ResourceInstanceResultRef, func(execgraph.AnyResultRef), func(execgraph.AnyResultRef)) {
				oldInstAddr := addrs.Resource{
					Mode: addrs.ManagedResourceMode,
					Type: "test",
					Name: "old",
				}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)
				return b.ManagedResourceInstanceSubgraph(
					&plans.ResourceInstanceChange{
						Addr:        instAddr,
						PrevRunAddr: oldInstAddr,
						Change: plans.Change{
							Action: plans.DeleteThenCreate,
							Before: cty.StringVal("before"),
							After:  cty.StringVal("after"),
						},
					},
					replaceDestroyThenCreate,
				)
			},
			`
				v[0] = cty.StringVal("after");
				v[1] = cty.NullVal(cty.String);

				r[0] = ResourceInstancePrior(test.old, await());
				r[1] = ManagedChangeAddr(r[0], test.placeholder);
				r[2] = ResourceInstanceDesired(test.placeholder, await());
				r[3] = ManagedFinalPlan(r[2], nil, v[0]);
				r[4] = ManagedFinalPlan(nil, r[1], v[1]);
				r[5] = ManagedApply(r[4], nil, await());
				r[6] = ManagedApply(r[3], nil, await(r[5]));

				test.placeholder = r[6];
			`,
		},
		"create then delete": {
			func(b *execGraphBuilder) (execgraph.ResourceInstanceResultRef, execgraph.ResourceInstanceResultRef, func(execgraph.AnyResultRef), func(execgraph.AnyResultRef)) {
				return b.ManagedResourceInstanceSubgraph(
					&plans.ResourceInstanceChange{
						Addr:        instAddr,
						PrevRunAddr: instAddr,
						Change: plans.Change{
							Action: plans.CreateThenDelete,
							Before: cty.StringVal("before"),
							After:  cty.StringVal("after"),
						},
					},
					replaceCreateThenDestroy,
				)
			},
			`
				v[0] = cty.StringVal("after");
				v[1] = cty.NullVal(cty.String);

				r[0] = ResourceInstancePrior(test.placeholder, await());
				r[1] = ResourceInstanceDesired(test.placeholder, await());
				r[2] = ManagedFinalPlan(r[1], nil, v[0]);
				r[3] = ManagedFinalPlan(nil, r[0], v[1]);
				r[4] = ManagedPrepareDepose(r[3], "00000001");
				r[5] = ManagedPerformDepose(r[0], r[4], await(r[2]));
				r[6] = ManagedApply(r[2], r[5], await());
				r[7] = ManagedApply(r[4], nil, await(r[6]));

				test.placeholder = r[6];
			`,
		},
		"create then delete with move": {
			func(b *execGraphBuilder) (execgraph.ResourceInstanceResultRef, execgraph.ResourceInstanceResultRef, func(execgraph.AnyResultRef), func(execgraph.AnyResultRef)) {
				oldInstAddr := addrs.Resource{
					Mode: addrs.ManagedResourceMode,
					Type: "test",
					Name: "old",
				}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)
				return b.ManagedResourceInstanceSubgraph(
					&plans.ResourceInstanceChange{
						Addr:        instAddr,
						PrevRunAddr: oldInstAddr,
						Change: plans.Change{
							Action: plans.CreateThenDelete,
							Before: cty.StringVal("before"),
							After:  cty.StringVal("after"),
						},
					},
					replaceCreateThenDestroy,
				)
			},
			`
				v[0] = cty.StringVal("after");
				v[1] = cty.NullVal(cty.String);

				r[0] = ResourceInstancePrior(test.old, await());
				r[1] = ManagedChangeAddr(r[0], test.placeholder);
				r[2] = ResourceInstanceDesired(test.placeholder, await());
				r[3] = ManagedFinalPlan(r[2], nil, v[0]);
				r[4] = ManagedFinalPlan(nil, r[1], v[1]);
				r[5] = ManagedPrepareDepose(r[4], "00000001");
				r[6] = ManagedPerformDepose(r[1], r[5], await(r[3]));
				r[7] = ManagedApply(r[3], r[6], await());
				r[8] = ManagedApply(r[5], nil, await(r[7]));

				test.placeholder = r[7];
			`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var placeholderDeposedKey uint32
			builder := newExecGraphBuilder(func(_ addrs.AbsResourceInstance) addrs.DeposedKey {
				// For testing purposes we just allocate sequential integers
				// so that we have predictable keys to include in the expected
				// output of each test.
				placeholderDeposedKey++
				return addrs.DeposedKey(fmt.Sprintf("%08x", placeholderDeposedKey))
			})
			// FIXME: We're currently ignoring all but the first result
			// because this test was originally written for an older variant
			// of this function which only had one result. We should find a
			// nice way to restructure this test so that it can check whether
			// _all_ of the return values are correct.
			resultRef, _, _, _ := test.Build(builder)
			builder.lower.SetResourceInstanceFinalStateResult(instAddr, resultRef)

			graph := builder.Finish()
			gotGraphRepr := strings.TrimSpace(graph.DebugRepr())
			wantGraphRepr := strings.TrimSpace(stripCommonLeadingTabs(test.WantRepr))
			if diff := cmp.Diff(wantGraphRepr, gotGraphRepr); diff != "" {
				t.Error("wrong result\n" + diff)
			}
		})
	}
}
