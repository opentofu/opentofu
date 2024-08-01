// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
)

func TestModuleExpansionTransformer(t *testing.T) {
	g := Graph{Path: addrs.RootModuleInstance}
	module := testModule(t, "transform-module-var-basic")

	{
		tf := &ModuleExpansionTransformer{Config: module}
		if err := tf.Transform(&g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformModuleExpBasicStr)
	if actual != expected {
		t.Fatalf("want:\n\n%s\n\ngot:\n\n%s", expected, actual)
	}
}

func TestModuleExpansionTransformer_nested(t *testing.T) {
	g := Graph{Path: addrs.RootModuleInstance}
	module := testModule(t, "transform-module-var-nested")

	{
		tf := &ModuleExpansionTransformer{Config: module}
		if err := tf.Transform(&g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformModuleExpNestedStr)
	if actual != expected {
		t.Fatalf("want:\n\n%s\n\ngot:\n\n%s", expected, actual)
	}
}

const testTransformModuleExpBasicStr = `
module.child (close)
  module.child (expand)
module.child (expand)
`

const testTransformModuleExpNestedStr = `
module.child (close)
  module.child (expand)
  module.child.module.child (close)
module.child (expand)
module.child.module.child (close)
  module.child.module.child (expand)
module.child.module.child (expand)
  module.child (expand)
`
