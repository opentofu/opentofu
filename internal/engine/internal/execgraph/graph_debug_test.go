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

func TestGraphWriteGraphvizRepr(t *testing.T) {
	dataInstAddr := addrs.Resource{
		Mode: addrs.DataResourceMode,
		Type: "test_thingy",
		Name: "example",
	}.Absolute(addrs.RootModuleInstance).Instance(addrs.NoKey)
	managedInstAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test_thingy",
		Name: "example",
	}.Absolute(addrs.RootModuleInstance).Instance(addrs.NoKey)

	b := NewBuilder()
	providerAddr := addrs.NewBuiltInProvider("test")
	providerDeps := b.Waiter()
	providerClient, registerCloseBlocker := b.ProviderInstance(
		addrs.AbsProviderInstanceCorrect{
			Config: addrs.AbsProviderConfigCorrect{
				Module: addrs.RootModuleInstance,
				Config: addrs.ProviderConfigCorrect{
					Provider: providerAddr,
				},
			},
		},
		providerDeps,
	)

	dataInstDeps := b.Waiter()
	dataDesired := b.DesiredResourceInstance(dataInstAddr)
	dataInstResult := b.DataRead(dataDesired, providerClient, dataInstDeps)
	b.SetResourceInstanceFinalStateResult(dataInstAddr, dataInstResult)
	registerCloseBlocker(dataInstResult)

	managedInstDeps := b.Waiter(dataInstResult)
	managedDesired := b.DesiredResourceInstance(managedInstAddr)
	managedPrior := b.ResourceInstancePriorState(managedInstAddr)
	managedPlanned := b.ConstantValue(cty.ObjectVal(map[string]cty.Value{
		"environment": cty.StringVal("production"),
		"id":          cty.UnknownVal(cty.String),
	}))
	managedInstPlan := b.ManagedResourceObjectFinalPlan(managedDesired, managedPrior, managedPlanned, providerClient, managedInstDeps)
	managedInstResult := b.ApplyManagedResourceObjectChanges(managedInstPlan, providerClient)
	b.SetResourceInstanceFinalStateResult(managedInstAddr, managedInstResult)
	registerCloseBlocker(managedInstResult)

	graph := b.Finish()
	var buf strings.Builder
	err := graph.WriteGraphvizRepr(&buf)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	want := strings.TrimSpace(`
digraph {
  rankdir=LR;
  node [fontname="Helvetica",color="#000000",bgcolor = "#ffffff"];
  r0 [shape=none,label=<<table border="0" cellborder="1" cellspacing="0"><tr><td bgcolor="#90c0e0" align="center"><b>  OpenProvider  </b></td></tr><tr><td align="left" port="a0">terraform.io/builtin/test</td></tr><tr><td align="left" port="a1">provider[&#34;terraform.io/builtin/test&#34;]</td></tr><tr><td align="left" port="a2"><i>(no dependencies)</i></td></tr><tr><td align="right" bgcolor="#eeeeee" port="r">r[0]</td></tr></table>>];
  r1 [shape=none,label=<<table border="0" cellborder="1" cellspacing="0"><tr><td bgcolor="#90c0e0" align="center"><b>  CloseProvider  </b></td></tr><tr><td align="left" port="a0">r[0]</td></tr><tr><td align="left" port="a1"><i>(2 dependencies)</i></td></tr><tr><td align="right" bgcolor="#eeeeee" port="r">r[1]</td></tr></table>>];
  r2 [shape=none,label=<<table border="0" cellborder="1" cellspacing="0"><tr><td bgcolor="#90c0e0" align="center"><b>  DataRead  </b></td></tr><tr><td align="left" port="a0">data.test_thingy.example<br align="left" />desired state<br align="left" /></td></tr><tr><td align="left" port="a1">r[0]</td></tr><tr><td align="left" port="a2"><i>(no dependencies)</i></td></tr><tr><td align="right" bgcolor="#eeeeee" port="r">data.test_thingy.example as r[2]</td></tr></table>>];
  r3 [shape=none,label=<<table border="0" cellborder="1" cellspacing="0"><tr><td bgcolor="#90c0e0" align="center"><b>  ManagedFinalPlan  </b></td></tr><tr><td align="left" port="a0">test_thingy.example<br align="left" />desired state<br align="left" /></td></tr><tr><td align="left" port="a1">test_thingy.example<br align="left" />prior state<br align="left" /></td></tr><tr><td align="left" port="a2"><font face="Courier">{<br align="left" />  &#34;environment&#34;: &#34;production&#34;,<br align="left" />  &#34;id&#34;: null<br align="left" />}<br align="left" /></font></td></tr><tr><td align="left" port="a3">r[0]</td></tr><tr><td align="left" port="a4"><i>(1 dependency)</i></td></tr><tr><td align="right" bgcolor="#eeeeee" port="r">r[3]</td></tr></table>>];
  r4 [shape=none,label=<<table border="0" cellborder="1" cellspacing="0"><tr><td bgcolor="#90c0e0" align="center"><b>  ManagedApplyChanges  </b></td></tr><tr><td align="left" port="a0">r[3]</td></tr><tr><td align="left" port="a1">r[0]</td></tr><tr><td align="right" bgcolor="#eeeeee" port="r">test_thingy.example as r[4]</td></tr></table>>];
  r0:r:e -> r1:a0:w;
  r2:r:e -> r1:a1:w;
  r4:r:e -> r1:a1:w;
  r0:r:e -> r2:a1:w;
  r0:r:e -> r3:a3:w;
  r2:r:e -> r3:a4:w;
  r3:r:e -> r4:a0:w;
  r0:r:e -> r4:a1:w;
}
`)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Error("wrong result\n" + diff)
	}
}
