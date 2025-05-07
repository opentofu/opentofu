// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
)

func TestPlanGraphBuilder_impl(t *testing.T) {
	var _ GraphBuilder = new(PlanGraphBuilder)
}

func TestPlanGraphBuilder(t *testing.T) {
	awsProvider := &MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			Provider: providers.Schema{Block: simpleTestSchema()},
			ResourceTypes: map[string]providers.Schema{
				"aws_security_group": {Block: simpleTestSchema()},
				"aws_instance":       {Block: simpleTestSchema()},
				"aws_load_balancer":  {Block: simpleTestSchema()},
			},
		},
	}
	openstackProvider := mockProviderWithResourceTypeSchema("openstack_floating_ip", simpleTestSchema())
	plugins := newContextPlugins(map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("aws"):       providers.FactoryFixed(awsProvider),
		addrs.NewDefaultProvider("openstack"): providers.FactoryFixed(openstackProvider),
	}, nil)

	b := &PlanGraphBuilder{
		Config:    testModule(t, "graph-builder-plan-basic"),
		Plugins:   plugins,
		Operation: walkPlan,
	}

	g, err := b.Build(t.Context(), addrs.RootModuleInstance)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if g.Path.String() != addrs.RootModuleInstance.String() {
		t.Fatalf("wrong module path %q", g.Path)
	}

	got := strings.TrimSpace(g.String())
	want := strings.TrimSpace(testPlanGraphBuilderStr)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("wrong result\n%s", diff)
	}
}

func TestPlanGraphBuilder_dynamicBlock(t *testing.T) {
	provider := mockProviderWithResourceTypeSchema("test_thing", &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"id":   {Type: cty.String, Computed: true},
			"list": {Type: cty.List(cty.String), Computed: true},
		},
		BlockTypes: map[string]*configschema.NestedBlock{
			"nested": {
				Nesting: configschema.NestingList,
				Block: configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"foo": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	})
	plugins := newContextPlugins(map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("test"): providers.FactoryFixed(provider),
	}, nil)

	b := &PlanGraphBuilder{
		Config:    testModule(t, "graph-builder-plan-dynblock"),
		Plugins:   plugins,
		Operation: walkPlan,
	}

	g, err := b.Build(t.Context(), addrs.RootModuleInstance)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if g.Path.String() != addrs.RootModuleInstance.String() {
		t.Fatalf("wrong module path %q", g.Path)
	}

	// This test is here to make sure we properly detect references inside
	// the special "dynamic" block construct. The most important thing here
	// is that at the end test_thing.c depends on both test_thing.a and
	// test_thing.b. Other details might shift over time as other logic in
	// the graph builders changes.
	got := strings.TrimSpace(g.String())
	want := strings.TrimSpace(`
provider["registry.opentofu.org/hashicorp/test"]
provider["registry.opentofu.org/hashicorp/test"] (close)
  test_thing.c (expand)
root
  provider["registry.opentofu.org/hashicorp/test"] (close)
test_thing.a (expand)
  provider["registry.opentofu.org/hashicorp/test"]
test_thing.b (expand)
  provider["registry.opentofu.org/hashicorp/test"]
test_thing.c (expand)
  test_thing.a (expand)
  test_thing.b (expand)
`)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("wrong result\n%s", diff)
	}
}

func TestPlanGraphBuilder_attrAsBlocks(t *testing.T) {
	provider := mockProviderWithResourceTypeSchema("test_thing", &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"id": {Type: cty.String, Computed: true},
			"nested": {
				Type: cty.List(cty.Object(map[string]cty.Type{
					"foo": cty.String,
				})),
				Optional: true,
			},
		},
	})
	plugins := newContextPlugins(map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("test"): providers.FactoryFixed(provider),
	}, nil)

	b := &PlanGraphBuilder{
		Config:    testModule(t, "graph-builder-plan-attr-as-blocks"),
		Plugins:   plugins,
		Operation: walkPlan,
	}

	g, err := b.Build(t.Context(), addrs.RootModuleInstance)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if g.Path.String() != addrs.RootModuleInstance.String() {
		t.Fatalf("wrong module path %q", g.Path)
	}

	// This test is here to make sure we properly detect references inside
	// the "nested" block that is actually defined in the schema as a
	// list-of-objects attribute. This requires some special effort
	// inside lang.ReferencesInBlock to make sure it searches blocks of
	// type "nested" along with an attribute named "nested".
	got := strings.TrimSpace(g.String())
	want := strings.TrimSpace(`
provider["registry.opentofu.org/hashicorp/test"]
provider["registry.opentofu.org/hashicorp/test"] (close)
  test_thing.b (expand)
root
  provider["registry.opentofu.org/hashicorp/test"] (close)
test_thing.a (expand)
  provider["registry.opentofu.org/hashicorp/test"]
test_thing.b (expand)
  test_thing.a (expand)
`)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("wrong result\n%s", diff)
	}
}

func TestPlanGraphBuilder_targetModule(t *testing.T) {
	b := &PlanGraphBuilder{
		Config:  testModule(t, "graph-builder-plan-target-module-provider"),
		Plugins: simpleMockPluginLibrary(),
		Targets: []addrs.Targetable{
			addrs.RootModuleInstance.Child("child2", addrs.NoKey),
		},
		Operation: walkPlan,
	}

	g, err := b.Build(t.Context(), addrs.RootModuleInstance)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	t.Logf("Graph: %s", g.String())

	testGraphNotContains(t, g, `module.child1.provider["registry.opentofu.org/hashicorp/test"]`)
	testGraphNotContains(t, g, "module.child1.test_object.foo")
}

func TestPlanGraphBuilder_excludeModule(t *testing.T) {
	b := &PlanGraphBuilder{
		Config:  testModule(t, "graph-builder-plan-target-module-provider"),
		Plugins: simpleMockPluginLibrary(),
		Excludes: []addrs.Targetable{
			addrs.RootModuleInstance.Child("child1", addrs.NoKey),
		},
		Operation: walkPlan,
	}

	g, err := b.Build(t.Context(), addrs.RootModuleInstance)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	t.Logf("Graph: %s", g.String())

	testGraphNotContains(t, g, `module.child1.provider["registry.opentofu.org/hashicorp/test"]`)
	testGraphNotContains(t, g, "module.child1.test_object.foo")
}

func TestPlanGraphBuilder_forEach(t *testing.T) {
	awsProvider := mockProviderWithResourceTypeSchema("aws_instance", simpleTestSchema())

	plugins := newContextPlugins(map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("aws"): providers.FactoryFixed(awsProvider),
	}, nil)

	b := &PlanGraphBuilder{
		Config:    testModule(t, "plan-for-each"),
		Plugins:   plugins,
		Operation: walkPlan,
	}

	g, err := b.Build(t.Context(), addrs.RootModuleInstance)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if g.Path.String() != addrs.RootModuleInstance.String() {
		t.Fatalf("wrong module path %q", g.Path)
	}

	got := strings.TrimSpace(g.String())
	// We're especially looking for the edge here, where aws_instance.bat
	// has a dependency on aws_instance.boo
	want := strings.TrimSpace(testPlanGraphBuilderForEachStr)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("wrong result\n%s", diff)
	}
}

const testPlanGraphBuilderStr = `
aws_instance.web (expand)
  aws_security_group.firewall (expand)
  var.foo (expand, reference)
aws_load_balancer.weblb (expand)
  aws_instance.web (expand)
aws_security_group.firewall (expand)
  provider["registry.opentofu.org/hashicorp/aws"]
local.instance_id (expand)
  aws_instance.web (expand)
openstack_floating_ip.random (expand)
  provider["registry.opentofu.org/hashicorp/openstack"]
output.instance_id (expand)
  local.instance_id (expand)
provider["registry.opentofu.org/hashicorp/aws"]
  openstack_floating_ip.random (expand)
provider["registry.opentofu.org/hashicorp/aws"] (close)
  aws_load_balancer.weblb (expand)
provider["registry.opentofu.org/hashicorp/openstack"]
provider["registry.opentofu.org/hashicorp/openstack"] (close)
  openstack_floating_ip.random (expand)
root
  output.instance_id (expand)
  provider["registry.opentofu.org/hashicorp/aws"] (close)
  provider["registry.opentofu.org/hashicorp/openstack"] (close)
var.foo
var.foo (expand, reference)
  var.foo
`
const testPlanGraphBuilderForEachStr = `
aws_instance.bar (expand)
  provider["registry.opentofu.org/hashicorp/aws"]
aws_instance.bar2 (expand)
  provider["registry.opentofu.org/hashicorp/aws"]
aws_instance.bat (expand)
  aws_instance.boo (expand)
aws_instance.baz (expand)
  provider["registry.opentofu.org/hashicorp/aws"]
aws_instance.boo (expand)
  provider["registry.opentofu.org/hashicorp/aws"]
aws_instance.foo (expand)
  provider["registry.opentofu.org/hashicorp/aws"]
provider["registry.opentofu.org/hashicorp/aws"]
provider["registry.opentofu.org/hashicorp/aws"] (close)
  aws_instance.bar (expand)
  aws_instance.bar2 (expand)
  aws_instance.bat (expand)
  aws_instance.baz (expand)
  aws_instance.foo (expand)
root
  provider["registry.opentofu.org/hashicorp/aws"] (close)
`
