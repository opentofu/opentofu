// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
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
		Build    func(b *execGraphBuilder, providerClientRef execgraph.ResultRef[*exec.ProviderClient]) execgraph.ResourceInstanceResultRef
		WantRepr string
	}{
		"create": {
			func(b *execGraphBuilder, providerClientRef execgraph.ResultRef[*exec.ProviderClient]) execgraph.ResourceInstanceResultRef {
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
					providerClientRef,
					addrs.MakeSet[addrs.AbsResourceInstance](),
				)
			},
			`
				v[0] = cty.EmptyObjectVal;
				
				r[0] = ResourceInstanceDesired(test.placeholder, await());
				r[1] = ManagedFinalPlan(r[0], nil, v[0], nil);
				r[2] = ManagedApply(r[1], nil, nil, await());

				test.placeholder = r[2];
			`,
		},
		"update": {
			func(b *execGraphBuilder, providerClientRef execgraph.ResultRef[*exec.ProviderClient]) execgraph.ResourceInstanceResultRef {
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
					providerClientRef,
					addrs.MakeSet[addrs.AbsResourceInstance](),
				)
			},
			`
				v[0] = cty.StringVal("after");
				
				r[0] = ResourceInstancePrior(test.placeholder);
				r[1] = ResourceInstanceDesired(test.placeholder, await());
				r[2] = ManagedFinalPlan(r[1], r[0], v[0], nil);
				r[3] = ManagedApply(r[2], nil, nil, await());

				test.placeholder = r[3];
			`,
		},
		"update with move": {
			func(b *execGraphBuilder, providerClientRef execgraph.ResultRef[*exec.ProviderClient]) execgraph.ResourceInstanceResultRef {
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
					providerClientRef,
					addrs.MakeSet[addrs.AbsResourceInstance](),
				)
			},
			`
				v[0] = cty.StringVal("after");

				r[0] = ResourceInstancePrior(test.old);
				r[1] = ManagedChangeAddr(r[0], test.placeholder);
				r[2] = ResourceInstanceDesired(test.placeholder, await());
				r[3] = ManagedFinalPlan(r[2], r[1], v[0], nil);
				r[4] = ManagedApply(r[3], nil, nil, await());

				test.placeholder = r[4];
			`,
		},
		"delete": {
			func(b *execGraphBuilder, providerClientRef execgraph.ResultRef[*exec.ProviderClient]) execgraph.ResourceInstanceResultRef {
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
					providerClientRef,
					addrs.MakeSet[addrs.AbsResourceInstance](),
				)
			},
			`
				v[0] = cty.NullVal(cty.EmptyObject);
				
				r[0] = ResourceInstancePrior(test.placeholder);
				r[1] = ManagedFinalPlan(nil, r[0], v[0], nil);
				r[2] = ManagedApply(r[1], nil, nil, await());

				test.placeholder = r[2];
			`,
		},
		"delete then create": {
			func(b *execGraphBuilder, providerClientRef execgraph.ResultRef[*exec.ProviderClient]) execgraph.ResourceInstanceResultRef {
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
					providerClientRef,
					addrs.MakeSet[addrs.AbsResourceInstance](),
				)
			},
			`
				v[0] = cty.StringVal("after");
				v[1] = cty.NullVal(cty.String);
				
				r[0] = ResourceInstancePrior(test.placeholder);
				r[1] = ResourceInstanceDesired(test.placeholder, await());
				r[2] = ManagedFinalPlan(r[1], nil, v[0], nil);
				r[3] = ManagedFinalPlan(nil, r[0], v[1], nil);
				r[4] = ManagedApply(r[3], nil, nil, await(r[2]));
				r[5] = ManagedApply(r[2], nil, nil, await(r[4]));

				test.placeholder = r[5];
			`,
		},
		"delete then create with move": {
			func(b *execGraphBuilder, providerClientRef execgraph.ResultRef[*exec.ProviderClient]) execgraph.ResourceInstanceResultRef {
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
					providerClientRef,
					addrs.MakeSet[addrs.AbsResourceInstance](),
				)
			},
			`
				v[0] = cty.StringVal("after");
				v[1] = cty.NullVal(cty.String);

				r[0] = ResourceInstancePrior(test.old);
				r[1] = ManagedChangeAddr(r[0], test.placeholder);
				r[2] = ResourceInstanceDesired(test.placeholder, await());
				r[3] = ManagedFinalPlan(r[2], nil, v[0], nil);
				r[4] = ManagedFinalPlan(nil, r[1], v[1], nil);
				r[5] = ManagedApply(r[4], nil, nil, await(r[3]));
				r[6] = ManagedApply(r[3], nil, nil, await(r[5]));

				test.placeholder = r[6];
			`,
		},
		"create then delete": {
			func(b *execGraphBuilder, providerClientRef execgraph.ResultRef[*exec.ProviderClient]) execgraph.ResourceInstanceResultRef {
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
					providerClientRef,
					addrs.MakeSet[addrs.AbsResourceInstance](),
				)
			},
			`
				v[0] = cty.StringVal("after");
				v[1] = cty.NullVal(cty.String);

				r[0] = ResourceInstancePrior(test.placeholder);
				r[1] = ResourceInstanceDesired(test.placeholder, await());
				r[2] = ManagedFinalPlan(r[1], nil, v[0], nil);
				r[3] = ManagedFinalPlan(nil, r[0], v[1], nil);
				r[4] = ManagedDepose(r[0], await(r[2], r[3]));
				r[5] = ManagedApply(r[2], r[4], nil, await());
				r[6] = ManagedApply(r[3], nil, nil, await(r[5]));

				test.placeholder = r[5];
			`,
		},
		"create then delete with move": {
			func(b *execGraphBuilder, providerClientRef execgraph.ResultRef[*exec.ProviderClient]) execgraph.ResourceInstanceResultRef {
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
					providerClientRef,
					addrs.MakeSet[addrs.AbsResourceInstance](),
				)
			},
			`
				v[0] = cty.StringVal("after");
				v[1] = cty.NullVal(cty.String);

				r[0] = ResourceInstancePrior(test.old);
				r[1] = ManagedChangeAddr(r[0], test.placeholder);
				r[2] = ResourceInstanceDesired(test.placeholder, await());
				r[3] = ManagedFinalPlan(r[2], nil, v[0], nil);
				r[4] = ManagedFinalPlan(nil, r[1], v[1], nil);
				r[5] = ManagedDepose(r[1], await(r[3], r[4]));
				r[6] = ManagedApply(r[3], r[5], nil, await());
				r[7] = ManagedApply(r[4], nil, nil, await(r[6]));

				test.placeholder = r[6];
			`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			builder := newExecGraphBuilder()
			// This test is focused only on the resource instance subgraphs,
			// so we just use a placeholder nil result for the provider client.
			providerClientRef := execgraph.NilResultRef[*exec.ProviderClient]()
			resultRef := test.Build(builder, providerClientRef)
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
