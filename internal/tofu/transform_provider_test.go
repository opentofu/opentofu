// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/dag"
)

func testProviderTransformerGraph(t *testing.T, cfg *configs.Config) *Graph {
	t.Helper()

	g := &Graph{Path: addrs.RootModuleInstance}
	ct := &ConfigTransformer{Config: cfg}
	if err := ct.Transform(g); err != nil {
		t.Fatal(err)
	}
	arct := &AttachResourceConfigTransformer{Config: cfg}
	if err := arct.Transform(g); err != nil {
		t.Fatal(err)
	}

	return g
}

// This variant exists purely for testing and can not currently include the ProviderFunctionTransformer
func testTransformProviders(concrete ConcreteProviderNodeFunc, config *configs.Config) GraphTransformer {
	return GraphTransformMulti(
		// Add providers from the config
		&ProviderConfigTransformer{
			Config:   config,
			Concrete: concrete,
		},
		// Add any remaining missing providers
		&MissingProviderTransformer{
			Config:   config,
			Concrete: concrete,
		},
		// Connect the providers
		&ProviderTransformer{
			Config: config,
		},
		// After schema transformer, we can add function references
		//  &ProviderFunctionTransformer{Config: config},
		// Remove unused providers and proxies
		&PruneProviderTransformer{},
	)
}

func TestProviderTransformer(t *testing.T) {
	mod := testModule(t, "transform-provider-basic")

	g := testProviderTransformerGraph(t, mod)
	{
		transform := &MissingProviderTransformer{}
		if err := transform.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	transform := &ProviderTransformer{}
	if err := transform.Transform(g); err != nil {
		t.Fatalf("err: %s", err)
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformProviderBasicStr)
	if actual != expected {
		t.Fatalf("bad:\n\n%s", actual)
	}
}

// Test providers with FQNs that do not match the typeName
func TestProviderTransformer_fqns(t *testing.T) {
	for _, mod := range []string{"fqns", "fqns-module"} {
		mod := testModule(t, fmt.Sprintf("transform-provider-%s", mod))

		g := testProviderTransformerGraph(t, mod)
		{
			transform := &MissingProviderTransformer{Config: mod}
			if err := transform.Transform(g); err != nil {
				t.Fatalf("err: %s", err)
			}
		}

		transform := &ProviderTransformer{Config: mod}
		if err := transform.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}

		actual := strings.TrimSpace(g.String())
		expected := strings.TrimSpace(testTransformProviderBasicStr)
		if actual != expected {
			t.Fatalf("bad:\n\n%s", actual)
		}
	}
}

func TestCloseProviderTransformer(t *testing.T) {
	mod := testModule(t, "transform-provider-basic")
	g := testProviderTransformerGraph(t, mod)

	{
		transform := &MissingProviderTransformer{}
		if err := transform.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	{
		transform := &ProviderTransformer{}
		if err := transform.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	{
		transform := &CloseProviderTransformer{}
		if err := transform.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformCloseProviderBasicStr)
	if actual != expected {
		t.Fatalf("bad:\n\n%s", actual)
	}
}

func TestCloseProviderTransformer_withTargets(t *testing.T) {
	mod := testModule(t, "transform-provider-basic")

	g := testProviderTransformerGraph(t, mod)
	transforms := []GraphTransformer{
		&MissingProviderTransformer{},
		&ProviderTransformer{},
		&CloseProviderTransformer{},
		&TargetsTransformer{
			Targets: []addrs.Targetable{
				addrs.RootModuleInstance.Resource(
					addrs.ManagedResourceMode, "something", "else",
				),
			},
		},
	}

	for _, tr := range transforms {
		if err := tr.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(``)
	if actual != expected {
		t.Fatalf("expected:%s\n\ngot:\n\n%s", expected, actual)
	}
}

func TestMissingProviderTransformer(t *testing.T) {
	mod := testModule(t, "transform-provider-missing")

	g := testProviderTransformerGraph(t, mod)
	{
		transform := &MissingProviderTransformer{}
		if err := transform.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	{
		transform := &ProviderTransformer{}
		if err := transform.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	{
		transform := &CloseProviderTransformer{}
		if err := transform.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformMissingProviderBasicStr)
	if actual != expected {
		t.Fatalf("expected:\n%s\n\ngot:\n%s", expected, actual)
	}
}

func TestMissingProviderTransformer_grandchildMissing(t *testing.T) {
	mod := testModule(t, "transform-provider-missing-grandchild")

	concrete := func(a *NodeAbstractProvider) dag.Vertex { return a }

	g := testProviderTransformerGraph(t, mod)
	{
		transform := testTransformProviders(concrete, mod)
		if err := transform.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}
	{
		transform := &TransitiveReductionTransformer{}
		if err := transform.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformMissingGrandchildProviderStr)
	if actual != expected {
		t.Fatalf("expected:\n%s\n\ngot:\n%s", expected, actual)
	}
}

func TestPruneProviderTransformer(t *testing.T) {
	mod := testModule(t, "transform-provider-prune")

	g := testProviderTransformerGraph(t, mod)
	{
		transform := &MissingProviderTransformer{}
		if err := transform.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	{
		transform := &ProviderTransformer{}
		if err := transform.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	{
		transform := &CloseProviderTransformer{}
		if err := transform.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	{
		transform := &PruneProviderTransformer{}
		if err := transform.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformPruneProviderBasicStr)
	if actual != expected {
		t.Fatalf("bad:\n\n%s", actual)
	}
}

// the child module resource is attached to the configured parent provider
func TestProviderConfigTransformer_parentProviders(t *testing.T) {
	mod := testModule(t, "transform-provider-inherit")
	concrete := func(a *NodeAbstractProvider) dag.Vertex { return a }

	g := testProviderTransformerGraph(t, mod)
	{
		tf := testTransformProviders(concrete, mod)
		if err := tf.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformModuleProviderConfigStr)
	if actual != expected {
		t.Fatalf("expected:\n%s\n\ngot:\n%s", expected, actual)
	}
}

// the child module resource is attached to the configured grand-parent provider
func TestProviderConfigTransformer_grandparentProviders(t *testing.T) {
	mod := testModule(t, "transform-provider-grandchild-inherit")
	concrete := func(a *NodeAbstractProvider) dag.Vertex { return a }

	g := testProviderTransformerGraph(t, mod)
	{
		tf := testTransformProviders(concrete, mod)
		if err := tf.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformModuleProviderGrandparentStr)
	if actual != expected {
		t.Fatalf("expected:\n%s\n\ngot:\n%s", expected, actual)
	}
}

func TestProviderConfigTransformer_inheritOldSkool(t *testing.T) {
	mod := testModuleInline(t, map[string]string{
		"main.tf": `
provider "test" {
  test_string = "config"
}

module "moda" {
  source = "./moda"
}
`,

		"moda/main.tf": `
resource "test_object" "a" {
}
`,
	})
	concrete := func(a *NodeAbstractProvider) dag.Vertex { return a }

	g := testProviderTransformerGraph(t, mod)
	{
		tf := testTransformProviders(concrete, mod)
		if err := tf.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	expected := `module.moda.test_object.a
  provider["registry.opentofu.org/hashicorp/test"]
provider["registry.opentofu.org/hashicorp/test"]`

	actual := strings.TrimSpace(g.String())
	if actual != expected {
		t.Fatalf("expected:\n%s\n\ngot:\n%s", expected, actual)
	}
}

// Verify that configurations which are not recommended yet supported still work
func TestProviderConfigTransformer_nestedModuleProviders(t *testing.T) {
	mod := testModuleInline(t, map[string]string{
		"main.tf": `
terraform {
  required_providers {
    test = {
      source = "registry.opentofu.org/hashicorp/test"
	}
  }
}

provider "test" {
  alias = "z"
  test_string = "config"
}

module "moda" {
  source = "./moda"
  providers = {
    test.x = test.z
  }
}
`,

		"moda/main.tf": `
terraform {
  required_providers {
    test = {
      source = "registry.opentofu.org/hashicorp/test"
      configuration_aliases = [ test.x ]
	}
  }
}

provider "test" {
  test_string = "config"
}

// this should connect to this module's provider
resource "test_object" "a" {
}

resource "test_object" "x" {
  provider = test.x
}

module "modb" {
  source = "./modb"
}
`,

		"moda/modb/main.tf": `
# this should end up with the provider from the parent module
resource "test_object" "a" {
}
`,
	})
	concrete := func(a *NodeAbstractProvider) dag.Vertex { return a }

	g := testProviderTransformerGraph(t, mod)
	{
		tf := testTransformProviders(concrete, mod)
		if err := tf.Transform(g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	expected := `module.moda.module.modb.test_object.a
  module.moda.provider["registry.opentofu.org/hashicorp/test"]
module.moda.provider["registry.opentofu.org/hashicorp/test"]
module.moda.test_object.a
  module.moda.provider["registry.opentofu.org/hashicorp/test"]
module.moda.test_object.x
  provider["registry.opentofu.org/hashicorp/test"].z
provider["registry.opentofu.org/hashicorp/test"].z`

	actual := strings.TrimSpace(g.String())
	if actual != expected {
		t.Fatalf("expected:\n%s\n\ngot:\n%s", expected, actual)
	}
}

func TestProviderConfigTransformer_duplicateLocalName(t *testing.T) {
	mod := testModuleInline(t, map[string]string{
		"main.tf": `
terraform {
  required_providers {
	# We have to allow this since it wasn't previously prevented. If the
	# default config is equivalent to the provider config, the user may never
	# see an error.
    dupe = {
      source = "registry.opentofu.org/hashicorp/test"
    }
  }
}

provider "test" {
}
`})
	concrete := func(a *NodeAbstractProvider) dag.Vertex { return a }

	g := testProviderTransformerGraph(t, mod)
	tf := ProviderConfigTransformer{
		Config:   mod,
		Concrete: concrete,
	}
	if err := tf.Transform(g); err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := `provider["registry.opentofu.org/hashicorp/test"]`

	actual := strings.TrimSpace(g.String())
	if actual != expected {
		t.Fatalf("expected:\n%s\n\ngot:\n%s", expected, actual)
	}
}

const testTransformProviderBasicStr = `
aws_instance.web
  provider["registry.opentofu.org/hashicorp/aws"]
provider["registry.opentofu.org/hashicorp/aws"]
`

const testTransformCloseProviderBasicStr = `
aws_instance.web
  provider["registry.opentofu.org/hashicorp/aws"]
provider["registry.opentofu.org/hashicorp/aws"]
provider["registry.opentofu.org/hashicorp/aws"] (close)
  aws_instance.web
  provider["registry.opentofu.org/hashicorp/aws"]
`

const testTransformMissingProviderBasicStr = `
aws_instance.web
  provider["registry.opentofu.org/hashicorp/aws"]
foo_instance.web
  provider["registry.opentofu.org/hashicorp/foo"]
provider["registry.opentofu.org/hashicorp/aws"]
provider["registry.opentofu.org/hashicorp/aws"] (close)
  aws_instance.web
  provider["registry.opentofu.org/hashicorp/aws"]
provider["registry.opentofu.org/hashicorp/foo"]
provider["registry.opentofu.org/hashicorp/foo"] (close)
  foo_instance.web
  provider["registry.opentofu.org/hashicorp/foo"]
`

const testTransformMissingGrandchildProviderStr = `
module.sub.module.subsub.bar_instance.two
  provider["registry.opentofu.org/hashicorp/bar"]
module.sub.module.subsub.foo_instance.one
  module.sub.provider["registry.opentofu.org/hashicorp/foo"]
module.sub.provider["registry.opentofu.org/hashicorp/foo"]
provider["registry.opentofu.org/hashicorp/bar"]
`

const testTransformPruneProviderBasicStr = `
foo_instance.web
  provider["registry.opentofu.org/hashicorp/foo"]
provider["registry.opentofu.org/hashicorp/foo"]
provider["registry.opentofu.org/hashicorp/foo"] (close)
  foo_instance.web
  provider["registry.opentofu.org/hashicorp/foo"]
`

const testTransformModuleProviderConfigStr = `
module.child.aws_instance.thing
  provider["registry.opentofu.org/hashicorp/aws"].foo
provider["registry.opentofu.org/hashicorp/aws"].foo
`

const testTransformModuleProviderGrandparentStr = `
module.child.module.grandchild.aws_instance.baz
  provider["registry.opentofu.org/hashicorp/aws"].foo
provider["registry.opentofu.org/hashicorp/aws"].foo
`

/* TODO
func Test_graphNodeProxyProvider_Expanded_noForEachOnModuleIs(t *testing.T) {
	// This test describes the simplest scenario, where we don't have for_each on the providers in none of the modules:
	// root provider -> second provider -> third provider
	rootProvider := NodeApplyableProvider{
		NodeAbstractProvider: &NodeAbstractProvider{
			Addr: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/null"]`),
		},
	}

	SecondLevelProxy := graphNodeProxyProvider{
		addr:    mustProviderConfig(`module.second.provider["registry.opentofu.org/hashicorp/null"]`),
		targets: map[addrs.InstanceKey]GraphNodeProvider{addrs.NoKey: &rootProvider},
	}

	thirdLevelProxy := graphNodeProxyProvider{
		addr:    mustProviderConfig(`module.second.module.third.provider["registry.opentofu.org/hashicorp/null"]`),
		targets: map[addrs.InstanceKey]GraphNodeProvider{addrs.NoKey: &SecondLevelProxy},
	}

	want := []ModuleInstancePotentialProvider{
		{
			moduleIdentifier: []addrs.ModuleInstanceStep{
				{
					Name: "second",
				},
				{
					Name: "third",
				},
			},
			resourceIdentifier: addrs.NoKey,
			concreteProvider:   rootProvider,
		},
	}

	t.Run("graphNodeProxyProvider_Expanded - no for_each on providers at all", func(t *testing.T) {
		n := thirdLevelProxy
		got := n.Expanded()

		if len(got) != 1 {
			t.Errorf("expected to get a single provider as a result")
		}

		if isDistinguishableProviderEqual(got[0], want[0]) {
			t.Errorf("Got DistinguishableProvider %v, want %v", got[0], want[0])
		}
	})
}

func Test_graphNodeProxyProvider_Expanded_ForEachInSecondModule(t *testing.T) {
	// This test describes the scenario where we have a for_each on the providers in the second module:
	// root provider.first, provider.second -> second provider.first, provider.second (for_each) -> third.first / third.second
	rootProviderFirst := NodeApplyableProvider{
		NodeAbstractProvider: &NodeAbstractProvider{
			Addr: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/null"].first`),
		},
	}

	rootProviderSecond := NodeApplyableProvider{
		NodeAbstractProvider: &NodeAbstractProvider{
			Addr: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/null"].second`),
		},
	}

	SecondLevelProxyFirst := graphNodeProxyProvider{
		addr:    mustProviderConfig(`module.second.provider["registry.opentofu.org/hashicorp/null"].first`),
		targets: map[addrs.InstanceKey]GraphNodeProvider{addrs.NoKey: &rootProviderFirst},
	}

	SecondLevelProxySecond := graphNodeProxyProvider{
		addr:    mustProviderConfig(`module.second.provider["registry.opentofu.org/hashicorp/null"].second`),
		targets: map[addrs.InstanceKey]GraphNodeProvider{addrs.NoKey: &rootProviderSecond},
	}

	thirdLevelProxy := graphNodeProxyProvider{
		addr:    mustProviderConfig(`module.second.module.third.provider["registry.opentofu.org/hashicorp/null"]`),
		targets: map[addrs.InstanceKey]GraphNodeProvider{addrs.StringKey("first"): &SecondLevelProxyFirst, addrs.StringKey("second"): &SecondLevelProxySecond},
	}

	want := []ModuleInstancePotentialProvider{
		{
			moduleIdentifier: []addrs.ModuleInstanceStep{
				{
					Name: "second",
				},
				{
					Name:        "third",
					InstanceKey: addrs.StringKey("first"),
				},
			},
			resourceIdentifier: addrs.NoKey,
			concreteProvider:   rootProviderFirst,
		},
		{
			moduleIdentifier: []addrs.ModuleInstanceStep{
				{
					Name: "second",
				},
				{
					Name:        "third",
					InstanceKey: addrs.StringKey("second"),
				},
			},
			resourceIdentifier: addrs.NoKey,
			concreteProvider:   rootProviderSecond,
		},
	}

	t.Run("graphNodeProxyProvider_Expanded - for_each on providers in the second module", func(t *testing.T) {
		n := thirdLevelProxy
		got := n.Expanded()

		if len(got) != 2 {
			t.Errorf("expected to get a single provider as a result")
		}

		matchingCounter := 0
		for _, resultProvider := range got {
			for _, wantProvider := range want {
				if isDistinguishableProviderEqual(resultProvider, wantProvider) {
					matchingCounter = matchingCounter + 1
				}
			}
		}

		if matchingCounter != 2 {
			t.Errorf("recieved providers are not matching to the expected providers")
		}
	})
}

func Test_graphNodeProxyProvider_Expanded_ForEachInRootModule(t *testing.T) {
	// This test describes the scenario where we have a for_each on the providers in the third module:
	// root provider.first, provider.second (for_each) -> second provider.first / provider.second  -> third.first / third.second
	rootProviderFirst := NodeApplyableProvider{
		NodeAbstractProvider: &NodeAbstractProvider{
			Addr: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/null"].first`),
		},
	}

	rootProviderSecond := NodeApplyableProvider{
		NodeAbstractProvider: &NodeAbstractProvider{
			Addr: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/null"].second`),
		},
	}

	SecondLevelProxy := graphNodeProxyProvider{
		addr:    mustProviderConfig(`module.second.provider["registry.opentofu.org/hashicorp/null"].first`),
		targets: map[addrs.InstanceKey]GraphNodeProvider{addrs.StringKey("first"): &rootProviderFirst, addrs.StringKey("second"): &rootProviderSecond},
	}

	thirdLevelProxy := graphNodeProxyProvider{
		addr:    mustProviderConfig(`module.second.module.third.provider["registry.opentofu.org/hashicorp/null"]`),
		targets: map[addrs.InstanceKey]GraphNodeProvider{addrs.NoKey: &SecondLevelProxy},
	}

	want := []ModuleInstancePotentialProvider{
		{
			moduleIdentifier: []addrs.ModuleInstanceStep{
				{
					Name:        "second",
					InstanceKey: addrs.StringKey("first"),
				},
				{
					Name: "third",
				},
			},
			resourceIdentifier: addrs.NoKey,
			concreteProvider:   rootProviderFirst,
		},
		{
			moduleIdentifier: []addrs.ModuleInstanceStep{
				{
					Name:        "second",
					InstanceKey: addrs.StringKey("second"),
				},
				{
					Name: "third",
				},
			},
			resourceIdentifier: addrs.NoKey,
			concreteProvider:   rootProviderSecond,
		},
	}

	t.Run("graphNodeProxyProvider_Expanded - for_each on providers in the root module", func(t *testing.T) {
		n := thirdLevelProxy
		got := n.Expanded()

		if len(got) != 2 {
			t.Errorf("expected to get a single provider as a result")
		}

		matchingCounter := 0
		for _, resultProvider := range got {
			for _, wantProvider := range want {
				if isDistinguishableProviderEqual(resultProvider, wantProvider) {
					matchingCounter = matchingCounter + 1
				}
			}
		}

		if matchingCounter != 2 {
			t.Errorf("recieved providers are not matching to the expected providers")
		}
	})
}

func isDistinguishableProviderEqual(got ModuleInstancePotentialProvider, want ModuleInstancePotentialProvider) bool {
	return !reflect.DeepEqual(got.moduleIdentifier, want.moduleIdentifier) ||
		!reflect.DeepEqual(got.resourceIdentifier, want.resourceIdentifier) ||
		got.concreteProvider.Name() != want.concreteProvider.Name()
}*/
