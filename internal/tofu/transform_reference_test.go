// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/dag"
)

func TestReferenceTransformer_simple(t *testing.T) {
	g := Graph{Path: addrs.RootModuleInstance}
	g.Add(&graphNodeRefParentTest{
		NameValue: "A",
		Names:     []string{"A"},
	})
	g.Add(&graphNodeRefChildTest{
		NameValue: "B",
		Refs:      []string{"A"},
	})

	tf := &ReferenceTransformer{}
	if err := tf.Transform(t.Context(), &g); err != nil {
		t.Fatalf("err: %s", err)
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformRefBasicStr)
	if actual != expected {
		t.Fatalf("wrong result\n\ngot:\n%s\n\nwant:\n%s", actual, expected)
	}
}

func TestReferenceTransformer_self(t *testing.T) {
	g := Graph{Path: addrs.RootModuleInstance}
	g.Add(&graphNodeRefParentTest{
		NameValue: "A",
		Names:     []string{"A"},
	})
	g.Add(&graphNodeRefChildTest{
		NameValue: "B",
		Refs:      []string{"A", "B"},
	})

	tf := &ReferenceTransformer{}
	if err := tf.Transform(t.Context(), &g); err != nil {
		t.Fatalf("err: %s", err)
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformRefBasicStr)
	if actual != expected {
		t.Fatalf("wrong result\n\ngot:\n%s\n\nwant:\n%s", actual, expected)
	}
}

func TestReferenceTransformer_path(t *testing.T) {
	g := Graph{Path: addrs.RootModuleInstance}
	g.Add(&graphNodeRefParentTest{
		NameValue: "A",
		Names:     []string{"A"},
	})
	g.Add(&graphNodeRefChildTest{
		NameValue: "B",
		Refs:      []string{"A"},
	})
	g.Add(&graphNodeRefParentTest{
		NameValue: "child.A",
		PathValue: addrs.ModuleInstance{addrs.ModuleInstanceStep{Name: "child"}},
		Names:     []string{"A"},
	})
	g.Add(&graphNodeRefChildTest{
		NameValue: "child.B",
		PathValue: addrs.ModuleInstance{addrs.ModuleInstanceStep{Name: "child"}},
		Refs:      []string{"A"},
	})

	tf := &ReferenceTransformer{}
	if err := tf.Transform(t.Context(), &g); err != nil {
		t.Fatalf("err: %s", err)
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformRefPathStr)
	if actual != expected {
		t.Fatalf("wrong result\n\ngot:\n%s\n\nwant:\n%s", actual, expected)
	}
}

func TestReferenceTransformer_resourceInstances(t *testing.T) {
	// Our reference analyses are all done based on unexpanded addresses
	// so that we can use this transformer both in the plan graph (where things
	// are not expanded yet) and the apply graph (where resource instances are
	// pre-expanded but nothing else is.)
	// However, that would make the result too conservative about instances
	// of the same resource in different instances of the same module, so we
	// make an exception for that situation in particular, keeping references
	// between resource instances segregated by their containing module
	// instance.
	g := Graph{Path: addrs.RootModuleInstance}
	moduleInsts := []addrs.ModuleInstance{
		{
			{
				Name: "foo", InstanceKey: addrs.IntKey(0),
			},
		},
		{
			{
				Name: "foo", InstanceKey: addrs.IntKey(1),
			},
		},
	}
	resourceAs := make([]addrs.AbsResourceInstance, len(moduleInsts))
	for i, moduleInst := range moduleInsts {
		resourceAs[i] = addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "thing",
			Name: "a",
		}.Instance(addrs.NoKey).Absolute(moduleInst)
	}
	resourceBs := make([]addrs.AbsResourceInstance, len(moduleInsts))
	for i, moduleInst := range moduleInsts {
		resourceBs[i] = addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "thing",
			Name: "b",
		}.Instance(addrs.NoKey).Absolute(moduleInst)
	}
	g.Add(&graphNodeFakeResourceInstance{
		Addr: resourceAs[0],
	})
	g.Add(&graphNodeFakeResourceInstance{
		Addr: resourceBs[0],
		Refs: []*addrs.Reference{
			{
				Subject: resourceAs[0].Resource,
			},
		},
	})
	g.Add(&graphNodeFakeResourceInstance{
		Addr: resourceAs[1],
	})
	g.Add(&graphNodeFakeResourceInstance{
		Addr: resourceBs[1],
		Refs: []*addrs.Reference{
			{
				Subject: resourceAs[1].Resource,
			},
		},
	})

	tf := &ReferenceTransformer{}
	if err := tf.Transform(t.Context(), &g); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Resource B should be connected to resource A in each module instance,
	// but there should be no connections between the two module instances.
	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(`
module.foo[0].thing.a
module.foo[0].thing.b
  module.foo[0].thing.a
module.foo[1].thing.a
module.foo[1].thing.b
  module.foo[1].thing.a
`)
	if actual != expected {
		t.Fatalf("wrong result\n\ngot:\n%s\n\nwant:\n%s", actual, expected)
	}
}

// TestAttachResourceDependsOnTransformer performs a sanity check on the happy path
// of attachResourceDependsOnTransformer.
// This expects to find the dependency injected into the graph nodes that this
// transformer is working with (data sources and ephemeral resource).
func TestAttachResourceDependsOnTransformer(t *testing.T) {
	cfg := testModuleInline(t, map[string]string{
		"main.tf": `
resource "null_instance" "write" {
  foo = "attribute"
}

data "null_data_source" "read" {
  depends_on = ["null_instance.write"]
}

ephemeral "null_ephemeral" "open" {
  depends_on = ["null_instance.write"]
}
`,
	})
	g := Graph{Path: addrs.RootModuleInstance}
	resCfg := cfg.Module.ResourceByAddr(addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "null_instance",
		Name: "write",
	})
	resAddr := resCfg.Addr().InModule(addrs.RootModule)
	g.Add(&NodeAbstractResourceInstance{
		NodeAbstractResource: NodeAbstractResource{Addr: resAddr, Config: resCfg},
		Addr: addrs.AbsResourceInstance{
			Module: addrs.ModuleInstance{},
			Resource: addrs.ResourceInstance{
				Resource: resCfg.Addr(),
				Key:      addrs.NoKey,
			},
		}})

	dataCfg := cfg.Module.ResourceByAddr(addrs.Resource{
		Mode: addrs.DataResourceMode,
		Type: "null_data_source",
		Name: "read",
	})
	dataAddr := dataCfg.Addr().InModule(addrs.RootModule)
	dataNode := &NodeAbstractResourceInstance{
		NodeAbstractResource: NodeAbstractResource{Addr: dataAddr, Config: dataCfg,
			Schema: &configschema.Block{}, // Setting this strictly to force references processing (check GraphNodeReferencer)
		},
		Addr: addrs.AbsResourceInstance{
			Module: addrs.ModuleInstance{},
			Resource: addrs.ResourceInstance{
				Resource: dataCfg.Addr(),
				Key:      addrs.NoKey,
			},
		},
	}
	g.Add(dataNode)

	ephemeralCfg := cfg.Module.ResourceByAddr(addrs.Resource{
		Mode: addrs.EphemeralResourceMode,
		Type: "null_ephemeral",
		Name: "open",
	})
	ephemeralAddr := ephemeralCfg.Addr().InModule(addrs.RootModule)
	ephemeralNode := &NodeAbstractResourceInstance{
		NodeAbstractResource: NodeAbstractResource{Addr: ephemeralAddr, Config: ephemeralCfg,
			Schema: &configschema.Block{}, // Setting this strictly to force references processing (check GraphNodeReferencer)
		},
		Addr: addrs.AbsResourceInstance{
			Module: addrs.ModuleInstance{},
			Resource: addrs.ResourceInstance{
				Resource: ephemeralCfg.Addr(),
				Key:      addrs.NoKey,
			},
		},
	}
	g.Add(ephemeralNode)

	tr := &attachResourceDependsOnTransformer{}
	err := tr.Transform(t.Context(), &g)
	if err != nil {
		t.Fatalf("expected no error. got %s", err)
	}

	if got, want := len(dataNode.dependsOn), 1; got != want {
		t.Fatalf("wrong number of deps on the data source graph node. expected %d but got %d", want, got)
	}
	if got, want := dataNode.dependsOn[0], resAddr; !got.Equal(want) {
		t.Fatalf("wrong reference registered as dependency. expected %+v but got %+v", want, got)
	}

	if got, want := len(ephemeralNode.dependsOn), 1; got != want {
		t.Fatalf("wrong number of deps on the data source graph node. expected %d but got %d", want, got)
	}
	if got, want := ephemeralNode.dependsOn[0], resAddr; !got.Equal(want) {
		t.Fatalf("wrong reference registered as dependency. expected %+v but got %+v", want, got)
	}
}

func TestReferenceMapReferences(t *testing.T) {
	cases := map[string]struct {
		Nodes  []dag.Vertex
		Check  dag.Vertex
		Result []string
	}{
		"simple": {
			Nodes: []dag.Vertex{
				&graphNodeRefParentTest{
					NameValue: "A",
					Names:     []string{"A"},
				},
			},
			Check: &graphNodeRefChildTest{
				NameValue: "foo",
				Refs:      []string{"A"},
			},
			Result: []string{"A"},
		},
	}

	for tn, tc := range cases {
		t.Run(tn, func(t *testing.T) {
			rm := NewReferenceMap(tc.Nodes)
			result := rm.References(tc.Check)

			var resultStr []string
			for _, v := range result {
				resultStr = append(resultStr, dag.VertexName(v))
			}

			sort.Strings(resultStr)
			sort.Strings(tc.Result)
			if !reflect.DeepEqual(resultStr, tc.Result) {
				t.Fatalf("bad: %#v", resultStr)
			}
		})
	}
}

type graphNodeRefParentTest struct {
	NameValue string
	PathValue addrs.ModuleInstance
	Names     []string
}

var _ GraphNodeReferenceable = (*graphNodeRefParentTest)(nil)

func (n *graphNodeRefParentTest) Name() string {
	return n.NameValue
}

func (n *graphNodeRefParentTest) ReferenceableAddrs() []addrs.Referenceable {
	ret := make([]addrs.Referenceable, len(n.Names))
	for i, name := range n.Names {
		ret[i] = addrs.LocalValue{Name: name}
	}
	return ret
}

func (n *graphNodeRefParentTest) Path() addrs.ModuleInstance {
	return n.PathValue
}

func (n *graphNodeRefParentTest) ModulePath() addrs.Module {
	return n.PathValue.Module()
}

type graphNodeRefChildTest struct {
	NameValue string
	PathValue addrs.ModuleInstance
	Refs      []string
}

var _ GraphNodeReferencer = (*graphNodeRefChildTest)(nil)

func (n *graphNodeRefChildTest) Name() string {
	return n.NameValue
}

func (n *graphNodeRefChildTest) References() []*addrs.Reference {
	ret := make([]*addrs.Reference, len(n.Refs))
	for i, name := range n.Refs {
		ret[i] = &addrs.Reference{
			Subject: addrs.LocalValue{Name: name},
		}
	}
	return ret
}

func (n *graphNodeRefChildTest) Path() addrs.ModuleInstance {
	return n.PathValue
}

func (n *graphNodeRefChildTest) ModulePath() addrs.Module {
	return n.PathValue.Module()
}

type graphNodeFakeResourceInstance struct {
	Addr addrs.AbsResourceInstance
	Refs []*addrs.Reference
}

var _ GraphNodeResourceInstance = (*graphNodeFakeResourceInstance)(nil)
var _ GraphNodeReferenceable = (*graphNodeFakeResourceInstance)(nil)
var _ GraphNodeReferencer = (*graphNodeFakeResourceInstance)(nil)

func (n *graphNodeFakeResourceInstance) ResourceInstanceAddr() addrs.AbsResourceInstance {
	return n.Addr
}

func (n *graphNodeFakeResourceInstance) ModulePath() addrs.Module {
	return n.Addr.Module.Module()
}

func (n *graphNodeFakeResourceInstance) ReferenceableAddrs() []addrs.Referenceable {
	return []addrs.Referenceable{n.Addr.Resource}
}

func (n *graphNodeFakeResourceInstance) References() []*addrs.Reference {
	return n.Refs
}

func (n *graphNodeFakeResourceInstance) StateDependencies() []addrs.ConfigResource {
	return nil
}

func (n *graphNodeFakeResourceInstance) String() string {
	return n.Addr.String()
}

const testTransformRefBasicStr = `
A
B
  A
`

const testTransformRefPathStr = `
A
B
  A
child.A
child.B
  child.A
`
