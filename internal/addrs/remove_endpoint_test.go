// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

func TestParseRemoveEndpoint(t *testing.T) {
	tests := []struct {
		Input   string
		WantRel ConfigRemovable
		WantErr string
	}{
		{
			`foo.bar`,
			ConfigResource{
				Module: RootModule,
				Resource: Resource{

					Mode: ManagedResourceMode,
					Type: "foo",
					Name: "bar",
				},
			},
			``,
		},
		{
			`module.boop`,
			Module{"boop"},
			``,
		},
		{
			`module.boop.foo.bar`,
			ConfigResource{
				Module: Module{"boop"},
				Resource: Resource{
					Mode: ManagedResourceMode,
					Type: "foo",
					Name: "bar",
				},
			},
			``,
		},
		{
			`module.foo.module.bar`,
			Module{"foo", "bar"},
			``,
		},
		{
			`module.boop.module.bip.foo.bar`,
			ConfigResource{
				Module: Module{"boop", "bip"},
				Resource: Resource{
					Mode: ManagedResourceMode,
					Type: "foo",
					Name: "bar",
				},
			},
			``,
		},
		{
			`foo.bar[0]`,
			nil,
			`Resource instance address with keys is not allowed: Resource address cannot be a resource instance (e.g. "null_resource.a[0]"), it must be a resource instead (e.g. "null_resource.a").`,
		},
		{
			`foo.bar["a"]`,
			nil,
			`Resource instance address with keys is not allowed: Resource address cannot be a resource instance (e.g. "null_resource.a[0]"), it must be a resource instead (e.g. "null_resource.a").`,
		},
		{
			`module.boop.foo.bar[0]`,
			nil,
			`Resource instance address with keys is not allowed: Resource address cannot be a resource instance (e.g. "null_resource.a[0]"), it must be a resource instead (e.g. "null_resource.a").`,
		},
		{
			`module.boop.foo.bar["a"]`,
			nil,

			`Resource instance address with keys is not allowed: Resource address cannot be a resource instance (e.g. "null_resource.a[0]"), it must be a resource instead (e.g. "null_resource.a").`,
		},
		{
			`data.foo.bar`,
			nil,
			`Data source address is not allowed: Data sources cannot be destroyed, and therefore, 'removed' blocks are not allowed to target them. To remove data sources from the state, you should remove the data source block from the configuration.`,
		},
		{
			`data.foo.bar[0]`,
			nil,
			`Resource instance address with keys is not allowed: Resource address cannot be a resource instance (e.g. "null_resource.a[0]"), it must be a resource instead (e.g. "null_resource.a").`,
		},
		{
			`data.foo.bar["a"]`,
			nil,
			`Resource instance address with keys is not allowed: Resource address cannot be a resource instance (e.g. "null_resource.a[0]"), it must be a resource instead (e.g. "null_resource.a").`,
		},
		{
			`module.boop.data.foo.bar[0]`,
			nil,
			`Resource instance address with keys is not allowed: Resource address cannot be a resource instance (e.g. "null_resource.a[0]"), it must be a resource instead (e.g. "null_resource.a").`,
		},
		{
			`module.boop.data.foo.bar["a"]`,
			nil,
			`Resource instance address with keys is not allowed: Resource address cannot be a resource instance (e.g. "null_resource.a[0]"), it must be a resource instead (e.g. "null_resource.a").`,
		},
		{
			`module.foo[0]`,
			nil,
			`Module instance address with keys is not allowed: Module address cannot be a module instance (e.g. "module.a[0]"), it must be a module instead (e.g. "module.a").`,
		},
		{
			`module.foo["a"]`,
			nil,
			`Module instance address with keys is not allowed: Module address cannot be a module instance (e.g. "module.a[0]"), it must be a module instead (e.g. "module.a").`,
		},
		{
			`module.foo[1].module.bar`,
			nil,
			`Module instance address with keys is not allowed: Module address cannot be a module instance (e.g. "module.a[0]"), it must be a module instead (e.g. "module.a").`,
		},
		{
			`module.foo.module.bar[1]`,
			nil,
			`Module instance address with keys is not allowed: Module address cannot be a module instance (e.g. "module.a[0]"), it must be a module instead (e.g. "module.a").`,
		},
		{
			`module.foo[0].module.bar[1]`,
			nil,
			`Module instance address with keys is not allowed: Module address cannot be a module instance (e.g. "module.a[0]"), it must be a module instead (e.g. "module.a").`,
		},
		{
			`module`,
			nil,
			`Invalid address operator: Prefix "module." must be followed by a module name.`,
		},
		{
			`module[0]`,
			nil,
			`Invalid address operator: Prefix "module." must be followed by a module name.`,
		},
		{
			`module.foo.data`,
			nil,
			`Invalid address: Resource specification must include a resource type and name.`,
		},
		{
			`module.foo.data.bar`,
			nil,
			`Invalid address: Resource specification must include a resource type and name.`,
		},
		{
			`module.foo.data[0]`,
			nil,
			`Invalid address: Resource specification must include a resource type and name.`,
		},
		{
			`module.foo.data.bar[0]`,
			nil,
			`Invalid address: A resource name is required.`,
		},
		{
			`module.foo.bar`,
			nil,
			`Invalid address: Resource specification must include a resource type and name.`,
		},
		{
			`module.foo.bar[0]`,
			nil,
			`Invalid address: A resource name is required.`,
		},
		// ephemeral
		{
			`ephemeral.foo.bar`,
			nil,
			`Ephemeral resource address is not allowed: Ephemeral resources cannot be destroyed, and therefore, 'removed' blocks are not allowed to target them. To remove ephemeral resources from the state, you should remove the ephemeral resource block from the configuration.`,
		},
		{
			`ephemeral.foo.bar[0]`,
			nil,
			`Resource instance address with keys is not allowed: Resource address cannot be a resource instance (e.g. "null_resource.a[0]"), it must be a resource instead (e.g. "null_resource.a").`,
		},
		{
			`ephemeral.foo.bar["a"]`,
			nil,
			`Resource instance address with keys is not allowed: Resource address cannot be a resource instance (e.g. "null_resource.a[0]"), it must be a resource instead (e.g. "null_resource.a").`,
		},
		{
			`module.boop.ephemeral.foo.bar[0]`,
			nil,
			`Resource instance address with keys is not allowed: Resource address cannot be a resource instance (e.g. "null_resource.a[0]"), it must be a resource instead (e.g. "null_resource.a").`,
		},
		{
			`module.boop.ephemeral.foo.bar["a"]`,
			nil,
			`Resource instance address with keys is not allowed: Resource address cannot be a resource instance (e.g. "null_resource.a[0]"), it must be a resource instead (e.g. "null_resource.a").`,
		},
		{
			`module.foo.ephemeral`,
			nil,
			`Invalid address: Resource specification must include a resource type and name.`,
		},
		{
			`module.foo.ephemeral.bar`,
			nil,
			`Invalid address: Resource specification must include a resource type and name.`,
		},
		{
			`module.foo.ephemeral[0]`,
			nil,
			`Invalid address: Resource specification must include a resource type and name.`,
		},
		{
			`module.foo.ephemeral.bar[0]`,
			nil,
			`Invalid address: A resource name is required.`,
		},
	}

	for _, test := range tests {
		t.Run(test.Input, func(t *testing.T) {
			traversal, hclDiags := hclsyntax.ParseTraversalAbs([]byte(test.Input), "", hcl.InitialPos)
			if hclDiags.HasErrors() {
				// We're not trying to test the HCL parser here, so any
				// failures at this point are likely to be bugs in the
				// test case itself.
				t.Fatalf("syntax error: %s", hclDiags.Error())
			}

			moveEp, diags := ParseRemoveEndpoint(traversal)

			switch {
			case test.WantErr != "":
				if !diags.HasErrors() {
					t.Fatalf("unexpected success\nwant error: %s", test.WantErr)
				}
				gotErr := diags.Err().Error()
				if gotErr != test.WantErr {
					t.Fatalf("wrong error\ngot:  %s\nwant: %s", gotErr, test.WantErr)
				}
			default:
				if diags.HasErrors() {
					t.Fatalf("unexpected error: %s", diags.Err().Error())
				}
				if diff := cmp.Diff(test.WantRel, moveEp.RelSubject); diff != "" {
					t.Errorf("wrong result\n%s", diff)
				}
			}
		})
	}
}
