// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonformat

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mitchellh/colorstring"

	"github.com/opentofu/opentofu/internal/command/jsonprovider"
	"github.com/opentofu/opentofu/internal/command/jsonstate"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/terminal"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tofu"
)

func TestState(t *testing.T) {
	color := &colorstring.Colorize{Colors: colorstring.DefaultColors, Disable: true}

	tests := []struct {
		State   *states.State
		Schemas *tofu.Schemas
		Want    string
	}{
		{
			State:   &states.State{},
			Schemas: &tofu.Schemas{},
			Want:    "The state file is empty. No resources are represented.\n",
		},
		{
			State:   basicState(t),
			Schemas: testSchemas(),
			Want:    basicStateOutput,
		},
		{
			State:   nestedState(t),
			Schemas: testSchemas(),
			Want:    nestedStateOutput,
		},
		{
			State:   deposedState(t),
			Schemas: testSchemas(),
			Want:    deposedNestedStateOutput,
		},
		{
			State:   onlyDeposedState(t),
			Schemas: testSchemas(),
			Want:    onlyDeposedOutput,
		},
		{
			State:   stateWithMoreOutputs(t),
			Schemas: testSchemas(),
			Want:    stateWithMoreOutputsOutput,
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {

			root, outputs, err := jsonstate.MarshalForRenderer(&statefile.File{
				State: tt.State,
			}, tt.Schemas)

			if err != nil {
				t.Errorf("found err: %v", err)
				return
			}

			streams, done := terminal.StreamsForTesting(t)
			renderer := Renderer{
				Colorize: color,
				Streams:  streams,
			}

			renderer.RenderHumanState(State{
				StateFormatVersion:    jsonstate.FormatVersion,
				RootModule:            root,
				RootModuleOutputs:     outputs,
				ProviderFormatVersion: jsonprovider.FormatVersion,
				ProviderSchemas:       jsonprovider.MarshalForRenderer(tt.Schemas),
			})

			result := done(t).All()
			if diff := cmp.Diff(result, tt.Want); diff != "" {
				t.Errorf("wrong output\nexpected:\n%s\nactual:\n%s\ndiff:\n%s\n", tt.Want, result, diff)
			}
		})
	}
}

func testProvider() *tofu.MockProvider {
	p := new(tofu.MockProvider)
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		return providers.ReadResourceResponse{NewState: req.PriorState}
	}

	p.GetProviderSchemaResponse = testProviderSchema()

	return p
}

func testProviderSchema() *providers.GetProviderSchemaResponse {
	return &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{
			Block: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"region": {Type: cty.String, Optional: true},
				},
			},
		},
		ResourceTypes: map[string]providers.Schema{
			"test_resource": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id":      {Type: cty.String, Computed: true},
						"foo":     {Type: cty.String, Optional: true},
						"woozles": {Type: cty.String, Optional: true},
					},
					BlockTypes: map[string]*configschema.NestedBlock{
						"nested": {
							Nesting: configschema.NestingList,
							Block: configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"compute": {Type: cty.String, Optional: true},
									"value":   {Type: cty.String, Optional: true},
								},
							},
						},
					},
				},
			},
		},
		DataSources: map[string]providers.Schema{
			"test_data_source": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"compute": {Type: cty.String, Optional: true},
						"value":   {Type: cty.String, Computed: true},
					},
				},
			},
		},
	}
}

func testSchemas() *tofu.Schemas {
	provider := testProvider()
	return &tofu.Schemas{
		Providers: map[addrs.Provider]providers.ProviderSchema{
			addrs.NewDefaultProvider("test"): provider.GetProviderSchema(context.TODO()),
		},
	}
}

const basicStateOutput = `# data.test_data_source.data:
data "test_data_source" "data" {
    compute = "sure"
}

# test_resource.baz[0]:
resource "test_resource" "baz" {
    woozles = "confuzles"
}


Outputs:

bar = "bar value"
`

const nestedStateOutput = `# test_resource.baz[0]:
resource "test_resource" "baz" {
    woozles = "confuzles"

    nested {
        value = "42"
    }
}
`

const deposedNestedStateOutput = `# test_resource.baz[0]:
resource "test_resource" "baz" {
    woozles = "confuzles"

    nested {
        value = "42"
    }
}

# test_resource.baz[0]: (deposed object 1234)
resource "test_resource" "baz" {
    woozles = "confuzles"

    nested {
        value = "42"
    }
}
`

const onlyDeposedOutput = `# test_resource.baz[0]: (deposed object 1234)
resource "test_resource" "baz" {
    woozles = "confuzles"

    nested {
        value = "42"
    }
}

# test_resource.baz[0]: (deposed object 5678)
resource "test_resource" "baz" {
    woozles = "confuzles"

    nested {
        value = "42"
    }
}
`

const stateWithMoreOutputsOutput = `# test_resource.baz[0]:
resource "test_resource" "baz" {
    woozles = "confuzles"
}


Outputs:

bool_var = true
int_var = 42
map_var = {
    "first"  = "foo"
    "second" = "bar"
}
sensitive_var = (sensitive value)
string_var = "string value"
`

func basicState(t *testing.T) *states.State {
	state := states.NewState()

	rootModule := state.RootModule()
	if rootModule == nil {
		t.Errorf("root module is nil; want valid object")
	}

	rootModule.SetLocalValue("foo", cty.StringVal("foo value"))
	rootModule.SetOutputValue("bar", cty.StringVal("bar value"), false, "")
	rootModule.SetResourceInstanceCurrent(
		addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "test_resource",
			Name: "baz",
		}.Instance(addrs.IntKey(0)),
		&states.ResourceInstanceObjectSrc{
			Status:        states.ObjectReady,
			SchemaVersion: 0,
			AttrsJSON:     []byte(`{"woozles":"confuzles"}`),
		},
		addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("test"),
			Module:   addrs.RootModule,
		},
		addrs.NoKey,
	)
	rootModule.SetResourceInstanceCurrent(
		addrs.Resource{
			Mode: addrs.DataResourceMode,
			Type: "test_data_source",
			Name: "data",
		}.Instance(addrs.NoKey),
		&states.ResourceInstanceObjectSrc{
			Status:        states.ObjectReady,
			SchemaVersion: 0,
			AttrsJSON:     []byte(`{"compute":"sure"}`),
		},
		addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("test"),
			Module:   addrs.RootModule,
		},
		addrs.NoKey,
	)
	return state
}

func stateWithMoreOutputs(t *testing.T) *states.State {
	state := states.NewState()

	rootModule := state.RootModule()
	if rootModule == nil {
		t.Errorf("root module is nil; want valid object")
	}

	rootModule.SetOutputValue("string_var", cty.StringVal("string value"), false, "")
	rootModule.SetOutputValue("int_var", cty.NumberIntVal(42), false, "")
	rootModule.SetOutputValue("bool_var", cty.BoolVal(true), false, "")
	rootModule.SetOutputValue("sensitive_var", cty.StringVal("secret!!!"), true, "")
	rootModule.SetOutputValue("map_var", cty.MapVal(map[string]cty.Value{
		"first":  cty.StringVal("foo"),
		"second": cty.StringVal("bar"),
	}), false, "")

	rootModule.SetResourceInstanceCurrent(
		addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "test_resource",
			Name: "baz",
		}.Instance(addrs.IntKey(0)),
		&states.ResourceInstanceObjectSrc{
			Status:        states.ObjectReady,
			SchemaVersion: 0,
			AttrsJSON:     []byte(`{"woozles":"confuzles"}`),
		},
		addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("test"),
			Module:   addrs.RootModule,
		},
		addrs.NoKey,
	)
	return state
}

func nestedState(t *testing.T) *states.State {
	state := states.NewState()

	rootModule := state.RootModule()
	if rootModule == nil {
		t.Errorf("root module is nil; want valid object")
	}

	rootModule.SetResourceInstanceCurrent(
		addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "test_resource",
			Name: "baz",
		}.Instance(addrs.IntKey(0)),
		&states.ResourceInstanceObjectSrc{
			Status:        states.ObjectReady,
			SchemaVersion: 0,
			AttrsJSON:     []byte(`{"woozles":"confuzles","nested": [{"value": "42"}]}`),
		},
		addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("test"),
			Module:   addrs.RootModule,
		},
		addrs.NoKey,
	)
	return state
}

func deposedState(t *testing.T) *states.State {
	state := nestedState(t)
	rootModule := state.RootModule()
	rootModule.SetResourceInstanceDeposed(
		addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "test_resource",
			Name: "baz",
		}.Instance(addrs.IntKey(0)),
		states.DeposedKey("1234"),
		&states.ResourceInstanceObjectSrc{
			Status:        states.ObjectReady,
			SchemaVersion: 0,
			AttrsJSON:     []byte(`{"woozles":"confuzles","nested": [{"value": "42"}]}`),
		},
		addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("test"),
			Module:   addrs.RootModule,
		},
		addrs.NoKey,
	)
	return state
}

// replicate a corrupt resource where only a deposed exists
func onlyDeposedState(t *testing.T) *states.State {
	state := states.NewState()

	rootModule := state.RootModule()
	if rootModule == nil {
		t.Errorf("root module is nil; want valid object")
	}

	rootModule.SetResourceInstanceDeposed(
		addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "test_resource",
			Name: "baz",
		}.Instance(addrs.IntKey(0)),
		states.DeposedKey("1234"),
		&states.ResourceInstanceObjectSrc{
			Status:        states.ObjectReady,
			SchemaVersion: 0,
			AttrsJSON:     []byte(`{"woozles":"confuzles","nested": [{"value": "42"}]}`),
		},
		addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("test"),
			Module:   addrs.RootModule,
		},
		addrs.NoKey,
	)
	rootModule.SetResourceInstanceDeposed(
		addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "test_resource",
			Name: "baz",
		}.Instance(addrs.IntKey(0)),
		states.DeposedKey("5678"),
		&states.ResourceInstanceObjectSrc{
			Status:        states.ObjectReady,
			SchemaVersion: 0,
			AttrsJSON:     []byte(`{"woozles":"confuzles","nested": [{"value": "42"}]}`),
		},
		addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("test"),
			Module:   addrs.RootModule,
		},
		addrs.NoKey,
	)
	return state
}
