// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/zclconf/go-cty/cty/gocty"
)

func TestModuleTransformAttachConfigTransformer(t *testing.T) {
	g := Graph{Path: addrs.RootModuleInstance}
	module := testModule(t, "transform-attach-config")

	err := (&ConfigTransformer{Config: module}).Transform(&g)
	if err != nil {
		t.Fatal(err)
	}

	err = (&AttachResourceConfigTransformer{Config: module}).Transform(&g)
	if err != nil {
		t.Fatal(err)
	}

	verts := g.Vertices()

	if len(verts) != 4 {
		t.Fatalf("Expected 4 vertices, got %v", len(verts))
	}

	expected := map[string]map[string]int{
		"data.aws_instance.data_instance": map[string]int{
			"data_config": 10,
		},
		"aws_instance.resource_instance": map[string]int{
			"resource_config": 20,
		},
		"module.child.data.aws_instance.child_data_instance": map[string]int{
			"data_config": 30,
		},
		"module.child.aws_instance.child_resource_instance": map[string]int{
			"data_config": 40,
		},
	}

	got := make(map[string]map[string]int)
	for _, v := range verts {
		ar := v.(*NodeAbstractResource)
		attrs, _ := ar.Config.Config.JustAttributes()

		values := make(map[string]int)
		for _, attr := range attrs {
			val, _ := attr.Expr.Value(nil)
			var target int
			gocty.FromCtyValue(val, &target)
			values[attr.Name] = target
		}

		got[ar.ResourceAddr().String()] = values
	}

	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("Expected %s, got %s", spew.Sdump(expected), spew.Sdump(got))
	}
}
