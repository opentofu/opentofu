// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/states"
)

func TestNodeAbstractResourceInstanceProvider(t *testing.T) {
	tests := []struct {
		Addr                 addrs.AbsResourceInstance
		Config               *configs.Resource
		StoredProviderConfig addrs.AbsProviderConfig
		Want                 addrs.Provider
	}{
		{
			Addr: addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "null_resource",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "hashicorp",
				Type:      "null",
			},
		},
		{
			Addr: addrs.Resource{
				Mode: addrs.DataResourceMode,
				Type: "terraform_remote_state",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Want: addrs.Provider{
				// As a special case, the type prefix "terraform_" maps to
				// the builtin provider, not the default one.
				Hostname:  addrs.BuiltInProviderHost,
				Namespace: addrs.BuiltInProviderNamespace,
				Type:      "terraform",
			},
		},
		{
			Addr: addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "null_resource",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Config: &configs.Resource{
				// Just enough configs.Resource for the Provider method. Not
				// actually valid for general use.
				Provider: addrs.Provider{
					Hostname:  addrs.DefaultProviderRegistryHost,
					Namespace: "awesomecorp",
					Type:      "happycloud",
				},
			},
			// The config overrides the default behavior.
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "awesomecorp",
				Type:      "happycloud",
			},
		},
		{
			Addr: addrs.Resource{
				Mode: addrs.DataResourceMode,
				Type: "terraform_remote_state",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Config: &configs.Resource{
				// Just enough configs.Resource for the Provider method. Not
				// actually valid for general use.
				Provider: addrs.Provider{
					Hostname:  addrs.DefaultProviderRegistryHost,
					Namespace: "awesomecorp",
					Type:      "happycloud",
				},
			},
			// The config overrides the default behavior.
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "awesomecorp",
				Type:      "happycloud",
			},
		},
		{
			Addr: addrs.Resource{
				Mode: addrs.DataResourceMode,
				Type: "null_resource",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Config: nil,
			StoredProviderConfig: addrs.AbsProviderConfig{
				Module: addrs.RootModule,
				Provider: addrs.Provider{
					Hostname:  addrs.DefaultProviderRegistryHost,
					Namespace: "awesomecorp",
					Type:      "null",
				},
			},
			// The stored provider config overrides the default behavior.
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "awesomecorp",
				Type:      "null",
			},
		},
	}

	for _, test := range tests {
		var name string
		if test.Config != nil {
			name = fmt.Sprintf("%s with configured %s", test.Addr, test.Config.Provider)
		} else {
			name = fmt.Sprintf("%s with no configuration", test.Addr)
		}
		t.Run(name, func(t *testing.T) {
			node := &NodeAbstractResourceInstance{
				// Just enough NodeAbstractResourceInstance for the Provider
				// function. (This would not be valid for some other functions.)
				Addr: test.Addr,
				NodeAbstractResource: NodeAbstractResource{
					Addr:                 test.Addr.ConfigResource(),
					Config:               test.Config,
					storedProviderConfig: test.StoredProviderConfig,
				},
			}
			got := node.Provider()
			if got != test.Want {
				t.Errorf("wrong result\naddr:  %s\nconfig: %#v\ngot:   %s\nwant:  %s", test.Addr, test.Config, got, test.Want)
			}
		})
	}
}

func TestNodeAbstractResourceInstance_WriteResourceInstanceState(t *testing.T) {
	state := states.NewState()
	ctx := new(MockEvalContext)
	ctx.StateState = state.SyncWrapper()
	ctx.PathPath = addrs.RootModuleInstance

	mockProvider := mockProviderWithResourceTypeSchema("aws_instance", &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"id": {
				Type:     cty.String,
				Optional: true,
			},
		},
	})

	obj := &states.ResourceInstanceObject{
		Value: cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("i-abc123"),
		}),
		Status: states.ObjectReady,
	}

	node := &NodeAbstractResourceInstance{
		Addr: mustResourceInstanceAddr("aws_instance.foo"),
		// instanceState:        obj,
		NodeAbstractResource: NodeAbstractResource{
			ResolvedProvider: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		},
	}
	ctx.ProviderProvider = mockProvider
	ctx.ProviderSchemaSchema = mockProvider.GetProviderSchema()

	err := node.writeResourceInstanceState(ctx, obj, workingState)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	checkStateString(t, state, `
aws_instance.foo:
  ID = i-abc123
  provider = provider["registry.opentofu.org/hashicorp/aws"]
	`)
}

func TestCombinePathValueMarks(t *testing.T) {
	paths := map[string]cty.PathValueMarks{
		"a.b": {
			Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "b"}},
			Marks: cty.NewValueMarks(marks.Sensitive),
		},
		"a.c": {
			Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "c"}},
			Marks: cty.NewValueMarks(marks.Sensitive),
		},
		"[0]": {
			Path:  cty.Path{cty.IndexStep{Key: cty.NumberIntVal(0)}},
			Marks: cty.NewValueMarks("a"),
		},
		"a.b<alt>": {
			Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "b"}},
			Marks: cty.NewValueMarks("a"),
		},
	}

	tests := []struct {
		name string
		LHS  []cty.PathValueMarks
		RHS  []cty.PathValueMarks
		Want []cty.PathValueMarks
	}{
		{
			name: "no marks",
			LHS:  []cty.PathValueMarks{},
			RHS:  []cty.PathValueMarks{},
			Want: []cty.PathValueMarks{},
		},
		{
			name: "one mark",
			LHS:  []cty.PathValueMarks{paths["a.b"]},
			RHS:  []cty.PathValueMarks{},
			Want: []cty.PathValueMarks{paths["a.b"]},
		},
		{
			name: "one overlapping mark",
			LHS:  []cty.PathValueMarks{paths["a.b"]},
			RHS:  []cty.PathValueMarks{paths["a.b"]},
			Want: []cty.PathValueMarks{paths["a.b"]},
		},
		{
			name: "one non-overlapping mark",
			LHS:  []cty.PathValueMarks{paths["a.b"]},
			RHS:  []cty.PathValueMarks{paths["a.c"]},
			Want: []cty.PathValueMarks{paths["a.b"], paths["a.c"]},
		},
		{
			name: "one overlapping and two non-overlapping marks",
			LHS:  []cty.PathValueMarks{paths["a.b"], paths["a.c"], paths["[0]"]},
			RHS:  []cty.PathValueMarks{paths["a.c"]},
			Want: []cty.PathValueMarks{paths["a.b"], paths["a.c"], paths["[0]"]},
		},
		{
			name: "one overlapping mark with different values",
			LHS: []cty.PathValueMarks{
				{
					Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "b"}},
					Marks: cty.NewValueMarks(marks.Sensitive),
				},
			},
			RHS: []cty.PathValueMarks{
				{
					Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "b"}},
					Marks: cty.NewValueMarks("OTHERMARK"),
				},
			},
			Want: []cty.PathValueMarks{
				{
					Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "b"}},
					Marks: cty.NewValueMarks(marks.Sensitive, "OTHERMARK"),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := combinePathValueMarks(test.LHS, test.RHS)
			if len(got) != len(test.Want) {
				t.Fatalf("incorrect result length\ngot:  %#v\nwant: %#v", got, test.Want)
			}

			for i, want := range test.Want {
				if !got[i].Equal(want) {
					t.Errorf("incorrect result\nindex: %d\ngot:  %#v\nwant: %#v", i, got[i], want)
				}
			}
		})
	}
}
