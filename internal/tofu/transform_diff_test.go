// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/refactoring"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
)

func TestDiffTransformer_nilDiff(t *testing.T) {
	g := Graph{Path: addrs.RootModuleInstance}
	tf := &DiffTransformer{}
	if err := tf.Transform(&g); err != nil {
		t.Fatalf("err: %s", err)
	}

	if len(g.Vertices()) > 0 {
		t.Fatal("graph should be empty")
	}
}

func TestDiffTransformer(t *testing.T) {
	g := Graph{Path: addrs.RootModuleInstance}

	beforeVal, err := plans.NewDynamicValue(cty.StringVal(""), cty.String)
	if err != nil {
		t.Fatal(err)
	}
	afterVal, err := plans.NewDynamicValue(cty.StringVal(""), cty.String)
	if err != nil {
		t.Fatal(err)
	}

	tf := &DiffTransformer{
		Changes: &plans.Changes{
			Resources: []*plans.ResourceInstanceChangeSrc{
				{
					Addr: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "aws_instance",
						Name: "foo",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					ProviderAddr: addrs.AbsProviderConfig{
						Provider: addrs.NewDefaultProvider("aws"),
						Module:   addrs.RootModule,
					},
					ChangeSrc: plans.ChangeSrc{
						Action: plans.Update,
						Before: beforeVal,
						After:  afterVal,
					},
				},
			},
		},
	}
	if err := tf.Transform(&g); err != nil {
		t.Fatalf("err: %s", err)
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformDiffBasicStr)
	if actual != expected {
		t.Fatalf("bad:\n\n%s", actual)
	}
}

func TestDiffTransformer_noOpChange(t *testing.T) {
	// "No-op" changes are how we record explicitly in a plan that we did
	// indeed visit a particular resource instance during the planning phase
	// and concluded that no changes were needed, as opposed to the resource
	// instance not existing at all or having been excluded from planning
	// entirely.
	//
	// We must include nodes for resource instances with no-op changes in the
	// apply graph, even though they won't take any external actions, because
	// there are some secondary effects such as precondition/postcondition
	// checks that can refer to objects elsewhere and so might have their
	// results changed even if the resource instance they are attached to
	// didn't actually change directly itself.

	// aws_instance.foo has a precondition, so should be included in the final
	// graph. aws_instance.bar has no conditions, so there is nothing to
	// execute during apply and it should not be included in the graph.
	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "aws_instance" "bar" {
}

resource "aws_instance" "foo" {
  test_string = "ok"

  lifecycle {
	precondition {
		condition     = self.test_string != ""
		error_message = "resource error"
	}
  }
}
`})

	g := Graph{Path: addrs.RootModuleInstance}

	beforeVal, err := plans.NewDynamicValue(cty.StringVal(""), cty.String)
	if err != nil {
		t.Fatal(err)
	}

	tf := &DiffTransformer{
		Config: m,
		Changes: &plans.Changes{
			Resources: []*plans.ResourceInstanceChangeSrc{
				{
					Addr: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "aws_instance",
						Name: "foo",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					ProviderAddr: addrs.AbsProviderConfig{
						Provider: addrs.NewDefaultProvider("aws"),
						Module:   addrs.RootModule,
					},
					ChangeSrc: plans.ChangeSrc{
						// A "no-op" change has the no-op action and has the
						// same object as both Before and After.
						Action: plans.NoOp,
						Before: beforeVal,
						After:  beforeVal,
					},
				},
				{
					Addr: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "aws_instance",
						Name: "bar",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					ProviderAddr: addrs.AbsProviderConfig{
						Provider: addrs.NewDefaultProvider("aws"),
						Module:   addrs.RootModule,
					},
					ChangeSrc: plans.ChangeSrc{
						// A "no-op" change has the no-op action and has the
						// same object as both Before and After.
						Action: plans.NoOp,
						Before: beforeVal,
						After:  beforeVal,
					},
				},
			},
		},
	}
	if err := tf.Transform(&g); err != nil {
		t.Fatalf("err: %s", err)
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformDiffBasicStr)
	if actual != expected {
		t.Fatalf("bad:\n\n%s", actual)
	}
}

const testTransformDiffBasicStr = `
aws_instance.foo
`

func TestTransformRemovedProvisioners(t *testing.T) {
	g := Graph{Path: addrs.RootModuleInstance}
	module := testModule(t, "transform-diff-creates-destroy-node")

	err := (&ConfigTransformer{Config: module}).Transform(&g)
	if err != nil {
		t.Fatal(err)
	}

	// instead of creating nodes manually in the graph, just use the DiffTransformer for doing it
	resAddr := mustResourceInstanceAddr("module.child.tfcoremock_simple_resource.example")
	err = (&DiffTransformer{
		Config: module,
		Changes: &plans.Changes{
			Resources: []*plans.ResourceInstanceChangeSrc{
				{
					Addr: resAddr,
					ChangeSrc: plans.ChangeSrc{
						Action: plans.Delete,
					},
				},
			},
		},
	}).Transform(&g)
	if err != nil {
		t.Fatal(err)
	}

	verts := g.Vertices()
	if len(verts) != 1 {
		t.Fatalf("Expected 1 vertices, got %v", len(verts))
	}
	concrete, ok := verts[0].(*NodeDestroyResourceInstance)
	if !ok {
		t.Fatalf("expected the only vertex to be a NodeDestroyResourceInstance. got instead %s", reflect.TypeOf(verts[0]).String())
	}
	wantProvisioners := refactoring.FindResourceRemovedBlockProvisioners(module, resAddr.ConfigResource())
	if len(wantProvisioners) == 0 { // just a sanity check to ensure that the call generating the wanted provisioners is actually returning 1 provisioner
		t.Fatalf("expected to have 1 provisioner in the config. this is an indication that the functionality for searching provisioners might be broken or that the test is wrongly configured")
	}
	if diff := cmp.Diff(wantProvisioners, concrete.removedBlockProvisioners, cmpopts.IgnoreUnexported(cty.Value{}, hcl.TraverseRoot{}, hcl.TraverseAttr{}, hclsyntax.Body{})); diff != "" {
		t.Fatalf("expected no diff between the expected provisioners and the ones configured in NodeDestroyResourceInstance. got:\n %s", diff)
	}
}
