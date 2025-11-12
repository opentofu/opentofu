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
	desiredInst := builder.DesiredResourceInstance(resourceInstAddr)
	priorState := builder.ResourceInstancePriorState(resourceInstAddr)
	finalPlan := builder.ManagedResourceObjectFinalPlan(
		desiredInst,
		priorState,
		initialPlannedValue,
		providerClient,
		nil,
	)
	newState := builder.ApplyManagedResourceObjectChanges(finalPlan, providerClient)
	addProviderUser(newState)
	builder.SetResourceInstanceFinalStateResult(resourceInstAddr, newState)

	graph := builder.Finish()
	got := graph.DebugRepr()
	want := strings.TrimLeft(`
v[0] = cty.ObjectVal(map[string]cty.Value{
    "name": cty.StringVal("thingy"),
});

r[0] = OpenProvider(provider("example.com/foo/bar"), providerInstConfig(provider["example.com/foo/bar"]), await());
r[1] = CloseProvider(r[0], await(r[3]));
r[2] = ManagedFinalPlan(desired(bar_thing.example), priorState(bar_thing.example), v[0], r[0], await());
r[3] = ManagedApplyChanges(r[2], r[0]);

bar_thing.example = r[3];
`, "\n")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Error("wrong result\n" + diff)
	}
}
