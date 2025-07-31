// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
)

func TestTransitiveReductionTransformer(t *testing.T) {
	mod := testModule(t, "transform-trans-reduce-basic")

	g := Graph{Path: addrs.RootModuleInstance}
	{
		tf := &ConfigTransformer{Config: mod}
		if err := tf.Transform(t.Context(), &g); err != nil {
			t.Fatalf("err: %s", err)
		}
		t.Logf("graph after ConfigTransformer:\n%s", g.String())
	}

	{
		transform := &AttachResourceConfigTransformer{Config: mod}
		if err := transform.Transform(t.Context(), &g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	{
		transform := &AttachSchemaTransformer{
			Plugins: schemaOnlyProvidersForTesting(map[addrs.Provider]providers.ProviderSchema{
				addrs.NewDefaultProvider("aws"): {
					ResourceTypes: map[string]providers.Schema{
						"aws_instance": {
							Block: &configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"A": {
										Type:     cty.String,
										Optional: true,
									},
									"B": {
										Type:     cty.String,
										Optional: true,
									},
								},
							},
						},
					},
				},
			}, t),
		}
		if err := transform.Transform(t.Context(), &g); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	{
		transform := &ReferenceTransformer{}
		if err := transform.Transform(t.Context(), &g); err != nil {
			t.Fatalf("err: %s", err)
		}
		t.Logf("graph after ReferenceTransformer:\n%s", g.String())
	}

	{
		transform := &TransitiveReductionTransformer{}
		if err := transform.Transform(t.Context(), &g); err != nil {
			t.Fatalf("err: %s", err)
		}
		t.Logf("graph after TransitiveReductionTransformer:\n%s", g.String())
	}

	actual := strings.TrimSpace(g.String())
	expected := strings.TrimSpace(testTransformTransReduceBasicStr)
	if actual != expected {
		t.Errorf("wrong result\ngot:\n%s\n\nwant:\n%s", actual, expected)
	}
}

const testTransformTransReduceBasicStr = `
aws_instance.A
aws_instance.B
  aws_instance.A
aws_instance.C
  aws_instance.B
`
