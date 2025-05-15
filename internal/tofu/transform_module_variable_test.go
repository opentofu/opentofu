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

func TestModuleVariableTransformer(t *testing.T) {
	g := Graph{Path: addrs.RootModuleInstance}
	module := testModule(t, "transform-module-var-basic")

	{
		tf := &RootVariableTransformer{Config: module}
		if err := tf.Transform(t.Context(), &g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	{
		tf := &ModuleVariableTransformer{Config: module}
		if err := tf.Transform(t.Context(), &g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformModuleVarBasicStr)
	if actual != expected {
		t.Fatalf("bad:\n\n%s", actual)
	}
}

func TestModuleVariableTransformer_nested(t *testing.T) {
	g := Graph{Path: addrs.RootModuleInstance}
	module := testModule(t, "transform-module-var-nested")

	{
		tf := &RootVariableTransformer{Config: module}
		if err := tf.Transform(t.Context(), &g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	{
		tf := &ModuleVariableTransformer{Config: module}
		if err := tf.Transform(t.Context(), &g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformModuleVarNestedStr)
	if actual != expected {
		t.Fatalf("bad:\n\n%s", actual)
	}
}

const testTransformModuleVarBasicStr = `
module.child.var.value (expand, input)
module.child.var.value (expand, reference)
  module.child.var.value (expand, input)
`

const testTransformModuleVarNestedStr = `
module.child.module.child.var.value (expand, input)
module.child.module.child.var.value (expand, reference)
  module.child.module.child.var.value (expand, input)
module.child.var.value (expand, input)
module.child.var.value (expand, reference)
  module.child.var.value (expand, input)
`
