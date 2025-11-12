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
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/refactoring"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
)

func TestDiffTransformer_nilDiff(t *testing.T) {
	g := Graph{Path: addrs.RootModuleInstance}
	tf := &DiffTransformer{}
	if err := tf.Transform(t.Context(), &g); err != nil {
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
	if err := tf.Transform(t.Context(), &g); err != nil {
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
	if err := tf.Transform(t.Context(), &g); err != nil {
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

	err := (&ConfigTransformer{Config: module}).Transform(t.Context(), &g)
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
	}).Transform(t.Context(), &g)
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

// TestDiffTransformerEphemeralChanges is having some wierd and theoretically impossible setup steps, but it's done
// this way to verify that some checks are in place along the way. Check the inline comments for more details.
func TestDiffTransformerEphemeralChanges(t *testing.T) {
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
		Config: &configs.Config{
			Module: &configs.Module{},
		},
		Changes: &plans.Changes{
			Resources: []*plans.ResourceInstanceChangeSrc{
				{
					Addr: addrs.Resource{
						// This is the wierd/stupid setup: the list of changes is NEVER meant to contain an ephemeral resource
						// since those are computed from the state compared with the currently wanted configuration.
						// Since ephemeral resources are not meant to be stored in the state file, this should never happen.
						// This setup is done this way only to be sure that the code path for creating NodeDestroyResourceInstance
						// is working well and that the node resulted from that returns an error on v.Execute(...)
						Mode: addrs.EphemeralResourceMode,
						Type: "aws_secretmanager_secret",
						Name: "foo",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					ProviderAddr: addrs.AbsProviderConfig{
						Provider: addrs.NewDefaultProvider("aws"),
						Module:   addrs.RootModule,
					},
					ChangeSrc: plans.ChangeSrc{
						Action: plans.Delete,
						Before: beforeVal,
						After:  afterVal,
					},
				},
			},
		},
	}
	if err := tf.Transform(t.Context(), &g); err != nil {
		t.Fatalf("err: %s", err)
	}

	actual := strings.TrimSpace(g.String())
	expected := "ephemeral.aws_secretmanager_secret.foo (destroy)"
	if actual != expected {
		t.Fatalf("bad:\n\n%s", actual)
	}

	verts := g.Vertices()
	if got := len(verts); got != 1 {
		t.Fatalf("expected to have exactly one vertex. got: %d", got)
	}

	// Let's see how NodeDestroyResourceInstance.Execute is behaving when it's for an ephemeral resource
	v, ok := verts[0].(*NodeDestroyResourceInstance)
	if !ok {
		t.Fatalf("expected that the only created vertex to be NodeDestroyResourceInstance. got %s", reflect.TypeOf(verts[0]))
	}
	diags := v.Execute(t.Context(), nil, walkApply)
	got := diags.Err().Error()
	want := "Destroy invoked for an ephemeral resource: A destroy operation has been invoked for the ephemeral resource \"ephemeral.aws_secretmanager_secret.foo\". This is an OpenTofu error. Please report this."
	if got != want {
		t.Fatalf("unexpected error returned.\ngot: %s\nwant:%s", got, want)
	}
}
