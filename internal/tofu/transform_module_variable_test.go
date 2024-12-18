// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/opentofu/opentofu/internal/addrs"
)

func TestModuleVariableTransformer(t *testing.T) {
	g := Graph{Path: addrs.RootModuleInstance}
	module := testModule(t, "transform-module-var-basic")

	{
		tf := &RootVariableTransformer{Config: module}
		if err := tf.Transform(&g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	{
		tf := &ModuleVariableTransformer{Config: module}
		if err := tf.Transform(&g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	got := strings.TrimSpace(g.String())
	want := strings.TrimSpace(testTransformModuleVarBasicStr)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal("wrong graph\n" + diff)
	}
}

func TestModuleVariableTransformer_nested(t *testing.T) {
	g := Graph{Path: addrs.RootModuleInstance}
	module := testModule(t, "transform-module-var-nested")

	{
		tf := &RootVariableTransformer{Config: module}
		if err := tf.Transform(&g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	{
		tf := &ModuleVariableTransformer{Config: module}
		if err := tf.Transform(&g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	got := strings.TrimSpace(g.String())
	want := strings.TrimSpace(testTransformModuleVarNestedStr)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal("wrong graph\n" + diff)
	}
}

const testTransformModuleVarBasicStr = `
module.child.var.value (input)
module.child.var.value (reference)
  module.child.var.value (input)
`

const testTransformModuleVarNestedStr = `
module.child.module.child.var.value (input)
module.child.module.child.var.value (reference)
  module.child.module.child.var.value (input)
module.child.var.value (input)
module.child.var.value (reference)
  module.child.var.value (input)
`
