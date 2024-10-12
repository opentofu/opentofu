// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
)

func testProviderInstanceTransformerGraph(t *testing.T, cfg *configs.Config) *Graph {
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
func testTransformProviders(concrete concreteProviderInstanceNodeFunc, config *configs.Config) GraphTransformer {
	return GraphTransformMulti(
		// Add providers from the config
		&providerConfigTransformer{
			config:           config,
			concreteProvider: concrete,
		},
	)
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
