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
	"github.com/opentofu/opentofu/internal/states"
	"github.com/zclconf/go-cty/cty"
)

func TestGraphNodeImportStateExecute(t *testing.T) {
	state := states.NewState()
	provider := testProvider("aws")
	provider.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "aws_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("bar"),
				}),
			},
		},
	}
	provider.ConfigureProvider(t.Context(), providers.ConfigureProviderRequest{})

	evalCtx := &MockEvalContext{
		StateState:       state.SyncWrapper(),
		ProviderProvider: provider,
	}

	// Import a new aws_instance.foo, this time with ID=bar. The original
	// aws_instance.foo object should be removed from state and replaced with
	// the new.
	node := graphNodeImportState{
		Addr: addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "aws_instance",
			Name: "foo",
		}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
		ID: "bar",
		ResolvedProvider: ResolvedProvider{ProviderConfig: addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("aws"),
			Module:   addrs.RootModule,
		}},
	}

	diags := node.Execute(t.Context(), evalCtx, walkImport)
	if diags.HasErrors() {
		t.Fatalf("Unexpected error: %s", diags.Err())
	}

	if len(node.states) != 1 {
		t.Fatalf("Wrong result! Expected one imported resource, got %d", len(node.states))
	}
	// Verify the ID for good measure
	id := node.states[0].State.GetAttr("id")
	if !id.RawEquals(cty.StringVal("bar")) {
		t.Fatalf("Wrong result! Expected id \"bar\", got %q", id.AsString())
	}
}

func TestGraphNodeImportStateSubExecute(t *testing.T) {
	state := states.NewState()
	provider := testProvider("aws")
	provider.ConfigureProvider(t.Context(), providers.ConfigureProviderRequest{})
	evalCtx := &MockEvalContext{
		StateState:       state.SyncWrapper(),
		ProviderProvider: provider,
		ProviderSchemaSchema: providers.ProviderSchema{
			ResourceTypes: map[string]providers.Schema{
				"aws_instance": {
					Block: &configschema.Block{
						Attributes: map[string]*configschema.Attribute{
							"id": {
								Type:     cty.String,
								Computed: true,
							},
						},
					},
				},
			},
		},
	}

	importedResource := providers.ImportedResource{
		TypeName: "aws_instance",
		State:    cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("bar")}),
	}

	node := graphNodeImportStateSub{
		TargetAddr: addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "aws_instance",
			Name: "foo",
		}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
		State: importedResource,
		ResolvedProvider: ResolvedProvider{ProviderConfig: addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("aws"),
			Module:   addrs.RootModule,
		}},
	}
	diags := node.Execute(t.Context(), evalCtx, walkImport)
	if diags.HasErrors() {
		t.Fatalf("Unexpected error: %s", diags.Err())
	}

	// check for resource in state
	actual := strings.TrimSpace(state.String())
	expected := `aws_instance.foo:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/aws"]`
	if actual != expected {
		t.Fatalf("bad state after import: \n%s", actual)
	}
}

func TestGraphNodeImportStateSubExecuteNull(t *testing.T) {
	state := states.NewState()
	provider := testProvider("aws")
	provider.ReadResourceFn = func(req providers.ReadResourceRequest) (resp providers.ReadResourceResponse) {
		// return null indicating that the requested resource does not exist
		resp.NewState = cty.NullVal(cty.Object(map[string]cty.Type{
			"id": cty.String,
		}))
		return resp
	}

	evalCtx := &MockEvalContext{
		StateState:       state.SyncWrapper(),
		ProviderProvider: provider,
		ProviderSchemaSchema: providers.ProviderSchema{
			ResourceTypes: map[string]providers.Schema{
				"aws_instance": {
					Block: &configschema.Block{
						Attributes: map[string]*configschema.Attribute{
							"id": {
								Type:     cty.String,
								Computed: true,
							},
						},
					},
				},
			},
		},
	}

	importedResource := providers.ImportedResource{
		TypeName: "aws_instance",
		State:    cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("bar")}),
	}

	node := graphNodeImportStateSub{
		TargetAddr: addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "aws_instance",
			Name: "foo",
		}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
		State: importedResource,
		ResolvedProvider: ResolvedProvider{ProviderConfig: addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("aws"),
			Module:   addrs.RootModule,
		}},
	}
	diags := node.Execute(t.Context(), evalCtx, walkImport)
	if !diags.HasErrors() {
		t.Fatal("expected error for non-existent resource")
	}
}
