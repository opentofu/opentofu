// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package local

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/clistate"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/depsfile"
	"github.com/opentofu/opentofu/internal/initwd"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/opentofu/opentofu/internal/tofu"

	"github.com/zclconf/go-cty/cty"
)

func TestLocal_refresh(t *testing.T) {
	b := TestLocal(t)

	p := TestLocalProvider(t, b, "test", refreshFixtureSchema())
	testStateFile(t, b.StatePath, testRefreshState())

	p.ReadResourceFn = nil
	p.ReadResourceResponse = &providers.ReadResourceResponse{NewState: cty.ObjectVal(map[string]cty.Value{
		"id": cty.StringVal("yes"),
	})}

	op, done := testOperationRefresh(t, "./testdata/refresh")
	defer done(t)

	run, err := b.Operation(context.Background(), op)
	if err != nil {
		t.Fatalf("bad: %s", err)
	}
	<-run.Done()

	if !p.ReadResourceCalled {
		t.Fatal("ReadResource should be called")
	}

	checkState(t, b.StateOutPath, `
test_instance.foo:
  ID = yes
  provider = provider["registry.opentofu.org/hashicorp/test"]
	`)

	// the backend should be unlocked after a run
	assertBackendStateUnlocked(t, b)
}

func TestLocal_refreshInput(t *testing.T) {
	b := TestLocal(t)

	schema := providers.ProviderSchema{
		Provider: providers.Schema{
			Block: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"value": {Type: cty.String, Optional: true},
				},
			},
		},
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id":  {Type: cty.String, Computed: true},
						"foo": {Type: cty.String, Optional: true},
						"ami": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}

	p := TestLocalProvider(t, b, "test", schema)
	testStateFile(t, b.StatePath, testRefreshState())

	p.ReadResourceFn = nil
	p.ReadResourceResponse = &providers.ReadResourceResponse{NewState: cty.ObjectVal(map[string]cty.Value{
		"id": cty.StringVal("yes"),
	})}
	p.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) (resp providers.ConfigureProviderResponse) {
		val := req.Config.GetAttr("value")
		if val.IsNull() || val.AsString() != "bar" {
			resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf("incorrect value %#v", val))
		}

		return
	}

	// Enable input asking since it is normally disabled by default
	b.OpInput = true
	b.ContextOpts.UIInput = &tofu.MockUIInput{InputReturnString: "bar"}

	op, done := testOperationRefresh(t, "./testdata/refresh-var-unset")
	defer done(t)
	op.UIIn = b.ContextOpts.UIInput

	run, err := b.Operation(context.Background(), op)
	if err != nil {
		t.Fatalf("bad: %s", err)
	}
	<-run.Done()

	if !p.ReadResourceCalled {
		t.Fatal("ReadResource should be called")
	}

	checkState(t, b.StateOutPath, `
test_instance.foo:
  ID = yes
  provider = provider["registry.opentofu.org/hashicorp/test"]
	`)
}

func TestLocal_refreshValidate(t *testing.T) {
	b := TestLocal(t)
	p := TestLocalProvider(t, b, "test", refreshFixtureSchema())
	testStateFile(t, b.StatePath, testRefreshState())
	p.ReadResourceFn = nil
	p.ReadResourceResponse = &providers.ReadResourceResponse{NewState: cty.ObjectVal(map[string]cty.Value{
		"id": cty.StringVal("yes"),
	})}

	// Enable validation
	b.OpValidation = true

	op, done := testOperationRefresh(t, "./testdata/refresh")
	defer done(t)

	run, err := b.Operation(context.Background(), op)
	if err != nil {
		t.Fatalf("bad: %s", err)
	}
	<-run.Done()

	checkState(t, b.StateOutPath, `
test_instance.foo:
  ID = yes
  provider = provider["registry.opentofu.org/hashicorp/test"]
	`)
}

func TestLocal_refreshValidateProviderConfigured(t *testing.T) {
	b := TestLocal(t)

	schema := providers.ProviderSchema{
		Provider: providers.Schema{
			Block: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"value": {Type: cty.String, Optional: true},
				},
			},
		},
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id":  {Type: cty.String, Computed: true},
						"ami": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}

	p := TestLocalProvider(t, b, "test", schema)
	testStateFile(t, b.StatePath, testRefreshState())
	p.ReadResourceFn = nil
	p.ReadResourceResponse = &providers.ReadResourceResponse{NewState: cty.ObjectVal(map[string]cty.Value{
		"id": cty.StringVal("yes"),
	})}

	// Enable validation
	b.OpValidation = true

	op, done := testOperationRefresh(t, "./testdata/refresh-provider-config")
	defer done(t)

	run, err := b.Operation(context.Background(), op)
	if err != nil {
		t.Fatalf("bad: %s", err)
	}
	<-run.Done()

	if !p.ValidateProviderConfigCalled {
		t.Fatal("Validate provider config should be called")
	}

	checkState(t, b.StateOutPath, `
test_instance.foo:
  ID = yes
  provider = provider["registry.opentofu.org/hashicorp/test"]
	`)
}

// This test validates the state lacking behavior when the inner call to
// Context() fails
func TestLocal_refresh_context_error(t *testing.T) {
	b := TestLocal(t)
	testStateFile(t, b.StatePath, testRefreshState())
	op, done := testOperationRefresh(t, "./testdata/apply")
	defer done(t)

	// we coerce a failure in Context() by omitting the provider schema

	run, err := b.Operation(context.Background(), op)
	if err != nil {
		t.Fatalf("bad: %s", err)
	}
	<-run.Done()
	if run.Result == backend.OperationSuccess {
		t.Fatal("operation succeeded; want failure")
	}
	assertBackendStateUnlocked(t, b)
}

func TestLocal_refreshEmptyState(t *testing.T) {
	b := TestLocal(t)

	p := TestLocalProvider(t, b, "test", refreshFixtureSchema())
	testStateFile(t, b.StatePath, states.NewState())

	p.ReadResourceFn = nil
	p.ReadResourceResponse = &providers.ReadResourceResponse{NewState: cty.ObjectVal(map[string]cty.Value{
		"id": cty.StringVal("yes"),
	})}

	op, done := testOperationRefresh(t, "./testdata/refresh")

	run, err := b.Operation(context.Background(), op)
	if err != nil {
		t.Fatalf("bad: %s", err)
	}
	<-run.Done()

	output := done(t)

	if stderr := output.Stderr(); stderr != "" {
		t.Fatalf("expected only warning diags, got errors: %s", stderr)
	}
	if got, want := output.Stdout(), "Warning: Empty or non-existent state"; !strings.Contains(got, want) {
		t.Errorf("wrong diags\n got: %s\nwant: %s", got, want)
	}

	// the backend should be unlocked after a run
	assertBackendStateUnlocked(t, b)
}

func testOperationRefresh(t *testing.T, configDir string) (*backend.Operation, func(*testing.T) *terminal.TestOutput) {
	t.Helper()

	_, configLoader := initwd.MustLoadConfigForTests(t, configDir, "tests")

	streams, done := terminal.StreamsForTesting(t)
	view := views.NewOperation(arguments.ViewHuman, false, views.NewView(streams))

	// Many of our tests use an overridden "test" provider that's just in-memory
	// inside the test process, not a separate plugin on disk.
	depLocks := depsfile.NewLocks()
	depLocks.SetProviderOverridden(addrs.MustParseProviderSourceString("registry.opentofu.org/hashicorp/test"))

	return &backend.Operation{
		Type:            backend.OperationTypeRefresh,
		ConfigDir:       configDir,
		ConfigLoader:    configLoader,
		StateLocker:     clistate.NewNoopLocker(),
		View:            view,
		DependencyLocks: depLocks,
	}, done
}

// testRefreshState is just a common state that we use for testing refresh.
func testRefreshState() *states.State {
	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_instance.foo").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"bar"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	return state
}

// refreshFixtureSchema returns a schema suitable for processing the
// configuration in testdata/refresh . This schema should be
// assigned to a mock provider named "test".
func refreshFixtureSchema() providers.ProviderSchema {
	return providers.ProviderSchema{
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"ami": {Type: cty.String, Optional: true},
						"id":  {Type: cty.String, Computed: true},
					},
				},
			},
		},
	}
}
