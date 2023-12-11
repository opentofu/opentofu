// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
)

func TestRootTransformer(t *testing.T) {
	t.Run("many nodes", func(t *testing.T) {
		mod := testModule(t, "transform-root-basic")

		g := Graph{Path: addrs.RootModuleInstance}
		{
			tf := &ConfigTransformer{Config: mod}
			if err := tf.Transform(&g); err != nil {
				t.Fatalf("err: %s", err)
			}
		}

		{
			transform := &MissingProviderTransformer{}
			if err := transform.Transform(&g); err != nil {
				t.Fatalf("err: %s", err)
			}
		}

		{
			transform := &ProviderTransformer{}
			if err := transform.Transform(&g); err != nil {
				t.Fatalf("err: %s", err)
			}
		}

		{
			transform := &RootTransformer{}
			if err := transform.Transform(&g); err != nil {
				t.Fatalf("err: %s", err)
			}
		}

		actual := strings.TrimSpace(g.String())
		expected := strings.TrimSpace(testTransformRootBasicStr)
		if actual != expected {
			t.Fatalf("wrong result\n\ngot:\n%s\n\nwant:\n%s", actual, expected)
		}

		root, err := g.Root()
		if err != nil {
			t.Fatalf("err: %s", err)
		}
		if _, ok := root.(graphNodeRoot); !ok {
			t.Fatalf("bad: %#v", root)
		}
	})

	t.Run("only one initial node", func(t *testing.T) {
		g := Graph{Path: addrs.RootModuleInstance}
		g.Add("foo")
		addRootNodeToGraph(&g)
		got := strings.TrimSpace(g.String())
		want := strings.TrimSpace(`
foo
root
  foo
`)
		if got != want {
			t.Errorf("wrong final graph\ngot:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("graph initially empty", func(t *testing.T) {
		g := Graph{Path: addrs.RootModuleInstance}
		addRootNodeToGraph(&g)
		got := strings.TrimSpace(g.String())
		want := `root`
		if got != want {
			t.Errorf("wrong final graph\ngot:\n%s\nwant:\n%s", got, want)
		}
	})

}

const testTransformRootBasicStr = `
aws_instance.foo
  provider["registry.opentofu.org/hashicorp/aws"]
do_droplet.bar
  provider["registry.opentofu.org/hashicorp/do"]
provider["registry.opentofu.org/hashicorp/aws"]
provider["registry.opentofu.org/hashicorp/do"]
root
  aws_instance.foo
  do_droplet.bar
`
