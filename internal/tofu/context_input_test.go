// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
)

func TestContext2Input_provider(t *testing.T) {
	m := testModule(t, "input-provider")

	providerCfgSchema := configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"foo": {
				Type:        cty.String,
				Required:    true,
				Description: "something something",
			},
		},
	}
	resourceCfgSchema := configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"id": {
				Type:     cty.String,
				Computed: true,
			},
		},
	}
	// Create an aws provider with one resource
	awsp := testProvider("aws")
	awsp.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &providerCfgSchema,
		ResourceTypes: map[string]*configschema.Block{
			"aws_instance": &resourceCfgSchema,
		},
	})
	// Create a cloudflare provider with one data source
	cfp := testProvider("cloudflare")
	cfp.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &providerCfgSchema,
		DataSources: map[string]*configschema.Block{
			"cloudflare_account": &resourceCfgSchema,
		},
	})
	cfp.ReadDataSourceResponse = &providers.ReadDataSourceResponse{
		State: cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("data content")}),
	}
	// Create an azure provider with one ephemeral resource
	azp := testProvider("azurem")
	azp.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &providerCfgSchema,
		EphemeralTypes: map[string]*configschema.Block{
			"azurerm_key_vault_secret": &resourceCfgSchema,
		},
	})
	azp.OpenEphemeralResourceResponse = &providers.OpenEphemeralResourceResponse{Result: cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("ephemeral result")})}

	inp := &MockUIInput{
		InputReturnMap: map[string]string{
			"provider.aws.foo":        "bar",
			"provider.cloudflare.foo": "baz",
			"provider.azurerm.foo":    "qux",
		},
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"):        testProviderFuncFixed(awsp),
			addrs.NewDefaultProvider("cloudflare"): testProviderFuncFixed(cfp),
			addrs.NewDefaultProvider("azurerm"):    testProviderFuncFixed(azp),
		},
		UIInput: inp,
	})

	var (
		actual = map[addrs.Provider]interface{}{}
		mu     sync.Mutex
	)
	awsp.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) (resp providers.ConfigureProviderResponse) {
		mu.Lock()
		defer mu.Unlock()
		actual[addrs.NewDefaultProvider("aws")] = req.Config.GetAttr("foo").AsString()
		return
	}
	cfp.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) (resp providers.ConfigureProviderResponse) {
		mu.Lock()
		defer mu.Unlock()
		actual[addrs.NewDefaultProvider("cloudflare")] = req.Config.GetAttr("foo").AsString()
		return
	}
	azp.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) (resp providers.ConfigureProviderResponse) {
		mu.Lock()
		defer mu.Unlock()
		actual[addrs.NewDefaultProvider("azurerm")] = req.Config.GetAttr("foo").AsString()
		return
	}

	if diags := ctx.Input(context.Background(), m, InputModeStd); diags.HasErrors() {
		t.Fatalf("input errors: %s", diags.Err())
	}

	if !inp.InputCalled {
		t.Fatal("no input prompt; want prompt for argument \"foo\"")
	}
	if got, want := inp.InputOpts.Description, "something something"; got != want {
		t.Errorf("wrong description\ngot:  %q\nwant: %q", got, want)
	}

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
	assertNoErrors(t, diags)

	if _, diags := ctx.Apply(context.Background(), plan, m, nil); diags.HasErrors() {
		t.Fatalf("apply errors: %s", diags.Err())
	}

	want := map[addrs.Provider]interface{}{
		addrs.NewDefaultProvider("aws"):        "bar",
		addrs.NewDefaultProvider("cloudflare"): "baz",
		addrs.NewDefaultProvider("azurerm"):    "qux",
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("wrong result\ngot:  %#v\nwant: %#v", actual, want)
	}
}

func TestContext2Input_providerMulti(t *testing.T) {
	m := testModule(t, "input-provider-multi")

	getProviderSchemaResponse := getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"foo": {
					Type:        cty.String,
					Required:    true,
					Description: "something something",
				},
			},
		},
		ResourceTypes: map[string]*configschema.Block{
			"aws_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
		},
	})

	// In order to update the provider to check only the configure calls during
	// apply, we will need to inject a new factory function after plan. We must
	// use a closure around the factory, because in order for the inputs to
	// work during apply we need to maintain the same context value, preventing
	// us from assigning a new Providers map.
	providerFactory := func() (providers.Interface, error) {
		p := testProvider("aws")
		p.GetProviderSchemaResponse = getProviderSchemaResponse
		return p, nil
	}

	inp := &MockUIInput{
		InputReturnMap: map[string]string{
			"provider.aws.foo":      "bar",
			"provider.aws.east.foo": "bar",
		},
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): func() (providers.Interface, error) {
				return providerFactory()
			},
		},
		UIInput: inp,
	})

	var actual []interface{}
	var lock sync.Mutex

	if diags := ctx.Input(context.Background(), m, InputModeStd); diags.HasErrors() {
		t.Fatalf("input errors: %s", diags.Err())
	}

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), DefaultPlanOpts)
	assertNoErrors(t, diags)

	providerFactory = func() (providers.Interface, error) {
		p := testProvider("aws")
		p.GetProviderSchemaResponse = getProviderSchemaResponse
		p.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) (resp providers.ConfigureProviderResponse) {
			lock.Lock()
			defer lock.Unlock()
			actual = append(actual, req.Config.GetAttr("foo").AsString())
			return
		}
		return p, nil
	}

	if _, diags := ctx.Apply(context.Background(), plan, m, nil); diags.HasErrors() {
		t.Fatalf("apply errors: %s", diags.Err())
	}

	expected := []interface{}{"bar", "bar"}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("wrong result\ngot:  %#v\nwant: %#v", actual, expected)
	}
}

func TestContext2Input_providerOnce(t *testing.T) {
	m := testModule(t, "input-provider-once")
	p := testProvider("aws")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	if diags := ctx.Input(context.Background(), m, InputModeStd); diags.HasErrors() {
		t.Fatalf("input errors: %s", diags.Err())
	}
}

func TestContext2Input_providerOnly(t *testing.T) {
	input := new(MockUIInput)

	m := testModule(t, "input-provider-vars")
	p := testProvider("aws")
	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"foo": {
					Type:     cty.String,
					Required: true,
				},
			},
		},
		ResourceTypes: map[string]*configschema.Block{
			"aws_instance": {
				Attributes: map[string]*configschema.Attribute{
					"foo":  {Type: cty.String, Required: true},
					"id":   {Type: cty.String, Computed: true},
					"type": {Type: cty.String, Computed: true},
				},
			},
		},
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
		UIInput: input,
	})

	input.InputReturnMap = map[string]string{
		"provider.aws.foo": "bar",
	}

	var actual interface{}
	p.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) (resp providers.ConfigureProviderResponse) {
		actual = req.Config.GetAttr("foo").AsString()
		return
	}

	if err := ctx.Input(context.Background(), m, InputModeProvider); err != nil {
		t.Fatalf("err: %s", err)
	}

	// NOTE: This is a stale test case from an older version of Terraform
	// where Input was responsible for prompting for both input variables _and_
	// provider configuration arguments, where it was trying to test the case
	// where we were turning off the mode of prompting for input variables.
	// That's now always disabled, and so this is essentially the same as the
	// normal Input test, but we're preserving it until we have time to review
	// and make sure this isn't inadvertently providing unique test coverage
	// other than what it set out to test.
	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		SetVariables: InputValues{
			"foo": &InputValue{
				Value:      cty.StringVal("us-west-2"),
				SourceType: ValueFromCaller,
			},
		},
	})
	assertNoErrors(t, diags)

	state, err := ctx.Apply(context.Background(), plan, m, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if !reflect.DeepEqual(actual, "bar") {
		t.Fatalf("wrong result\ngot:  %#v\nwant: %#v", actual, "bar")
	}

	actualStr := strings.TrimSpace(state.String())
	expectedStr := strings.TrimSpace(testTofuInputProviderOnlyStr)
	if actualStr != expectedStr {
		t.Fatalf("wrong result\n\ngot:\n%s\n\nwant:\n%s", actualStr, expectedStr)
	}
}

func TestContext2Input_providerVars(t *testing.T) {
	input := new(MockUIInput)
	m := testModule(t, "input-provider-with-vars")
	p := testProvider("aws")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
		UIInput: input,
	})

	input.InputReturnMap = map[string]string{
		"var.foo": "bar",
	}

	var actual interface{}
	p.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) (resp providers.ConfigureProviderResponse) {
		actual = req.Config.GetAttr("foo").AsString()
		return
	}
	if diags := ctx.Input(context.Background(), m, InputModeStd); diags.HasErrors() {
		t.Fatalf("input errors: %s", diags.Err())
	}

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
		SetVariables: InputValues{
			"foo": &InputValue{
				Value:      cty.StringVal("bar"),
				SourceType: ValueFromCaller,
			},
		},
	})
	assertNoErrors(t, diags)

	if _, diags := ctx.Apply(context.Background(), plan, m, nil); diags.HasErrors() {
		t.Fatalf("apply errors: %s", diags.Err())
	}

	if !reflect.DeepEqual(actual, "bar") {
		t.Fatalf("bad: %#v", actual)
	}
}

func TestContext2Input_providerVarsModuleInherit(t *testing.T) {
	input := new(MockUIInput)
	m := testModule(t, "input-provider-with-vars-and-module")
	p := testProvider("aws")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
		UIInput: input,
	})

	if diags := ctx.Input(context.Background(), m, InputModeStd); diags.HasErrors() {
		t.Fatalf("input errors: %s", diags.Err())
	}
}

// adding a list interpolation in fails to interpolate the count variable
func TestContext2Input_submoduleTriggersInvalidCount(t *testing.T) {
	input := new(MockUIInput)
	m := testModule(t, "input-submodule-count")
	p := testProvider("aws")
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
		UIInput: input,
	})

	if diags := ctx.Input(context.Background(), m, InputModeStd); diags.HasErrors() {
		t.Fatalf("input errors: %s", diags.Err())
	}
}

// In this case, a module variable can't be resolved from a data source until
// it's refreshed, but it can't be refreshed during Input.
func TestContext2Input_dataSourceRequiresRefresh(t *testing.T) {
	input := new(MockUIInput)
	p := testProvider("null")
	m := testModule(t, "input-module-data-vars")

	p.GetProviderSchemaResponse = getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		DataSources: map[string]*configschema.Block{
			"null_data_source": {
				Attributes: map[string]*configschema.Attribute{
					"foo": {Type: cty.List(cty.String), Optional: true},
				},
			},
		},
	})
	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
		return providers.ReadDataSourceResponse{
			State: req.Config,
		}
	}

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.DataResourceMode,
				Type: "null_data_source",
				Name: "bar",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsFlat: map[string]string{
					"id":    "-",
					"foo.#": "1",
					"foo.0": "a",
					// foo.1 exists in the data source, but needs to be refreshed.
				},
				Status: states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("null"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("null"): testProviderFuncFixed(p),
		},
		UIInput: input,
	})

	if diags := ctx.Input(context.Background(), m, InputModeStd); diags.HasErrors() {
		t.Fatalf("input errors: %s", diags.Err())
	}

	// ensure that plan works after Refresh. This is a legacy test that
	// doesn't really make sense anymore, because Refresh is really just
	// a wrapper around plan anyway, but we're keeping it until we get a
	// chance to review and check whether it's giving us any additional
	// test coverage aside from what it's specifically intending to test.
	if _, diags := ctx.Refresh(context.Background(), m, state, DefaultPlanOpts); diags.HasErrors() {
		t.Fatalf("refresh errors: %s", diags.Err())
	}
	if _, diags := ctx.Plan(context.Background(), m, state, DefaultPlanOpts); diags.HasErrors() {
		t.Fatalf("plan errors: %s", diags.Err())
	}
}
