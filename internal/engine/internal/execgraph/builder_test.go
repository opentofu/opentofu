// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/zclconf/go-cty/cty"
)

func TestBuilder_basics(t *testing.T) {
	builder := NewBuilder()

	// The following approximates might appear in the planning engine's code
	// for building the execution subgraph for a desired resource instance,
	// arranging for its changes to be planned and applied with whatever
	// provider instance was selected in the configuration.
	resourceInstAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "bar_thing",
		Name: "example",
	}.Absolute(addrs.RootModuleInstance).Instance(addrs.NoKey)
	initialPlannedValue := builder.ConstantValue(cty.ObjectVal(map[string]cty.Value{
		"name": cty.StringVal("thingy"),
	}))
	providerClient, addProviderUser := builder.ProviderInstance(addrs.AbsProviderInstanceCorrect{
		Config: addrs.AbsProviderConfigCorrect{
			Config: addrs.ProviderConfigCorrect{
				Provider: addrs.MustParseProviderSourceString("example.com/foo/bar"),
			},
		},
	}, nil)
	instAddrResult := builder.ConstantResourceInstAddr(resourceInstAddr)
	desiredInst := builder.ResourceInstanceDesired(instAddrResult, nil)
	priorState := builder.ResourceInstancePrior(instAddrResult)
	finalPlan := builder.ManagedFinalPlan(
		desiredInst,
		priorState,
		initialPlannedValue,
		providerClient,
	)
	newState := builder.ManagedApply(
		finalPlan, NilResultRef[*exec.ResourceInstanceObject](), providerClient,
	)
	addProviderUser(newState)
	builder.SetResourceInstanceFinalStateResult(resourceInstAddr, newState)

	graph := builder.Finish()
	got := graph.DebugRepr()
	want := strings.TrimLeft(`
v[0] = cty.ObjectVal(map[string]cty.Value{
    "name": cty.StringVal("thingy"),
});

r[0] = ProviderInstanceConfig(provider["example.com/foo/bar"], await());
r[1] = ProviderInstanceOpen(r[0]);
r[2] = ProviderInstanceClose(r[1], await(r[6]));
r[3] = ResourceInstanceDesired(bar_thing.example, await());
r[4] = ResourceInstancePrior(bar_thing.example);
r[5] = ManagedFinalPlan(r[3], r[4], v[0], r[1]);
r[6] = ManagedApply(r[5], nil, r[1]);

bar_thing.example = r[6];
`, "\n")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Error("wrong result\n" + diff)
	}
}
