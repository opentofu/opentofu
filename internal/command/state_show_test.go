// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/zclconf/go-cty/cty"
)

func TestStateShow(t *testing.T) {
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})
	statePath := testStateFile(t, state)

	p := testProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id":  {Type: cty.String, Optional: true, Computed: true},
						"foo": {Type: cty.String, Optional: true},
						"bar": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}

	streams, done := terminal.StreamsForTesting(t)
	c := &StateShowCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Streams:          streams,
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	// Test that outputs were displayed
	expected := strings.TrimSpace(testStateShowOutput) + "\n"
	actual := output.Stdout()
	if actual != expected {
		t.Fatalf("Expected:\n%q\n\nTo equal:\n%q", actual, expected)
	}
}

func TestStateShow_multi(t *testing.T) {
	submod, _ := addrs.ParseModuleInstanceStr("module.sub")
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(submod),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"foo","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   submod.Module(),
			},
			addrs.NoKey,
		)
	})
	statePath := testStateFile(t, state)

	p := testProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id":  {Type: cty.String, Optional: true, Computed: true},
						"foo": {Type: cty.String, Optional: true},
						"bar": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}

	streams, done := terminal.StreamsForTesting(t)
	c := &StateShowCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Streams:          streams,
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	// Test that outputs were displayed
	expected := strings.TrimSpace(testStateShowOutput) + "\n"
	actual := output.Stdout()
	if actual != expected {
		t.Fatalf("Expected:\n%q\n\nTo equal:\n%q", actual, expected)
	}
}

func TestStateShow_noState(t *testing.T) {
	testCwdTemp(t)

	p := testProvider()
	streams, done := terminal.StreamsForTesting(t)
	c := &StateShowCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Streams:          streams,
		},
	}

	args := []string{
		"test_instance.foo",
	}
	if code := c.Run(args); code != 1 {
		t.Fatalf("bad: %d", code)
	}
	output := done(t)
	if !strings.Contains(output.Stderr(), "No state file was found!") {
		t.Fatalf("expected a no state file error, got: %s", output.Stderr())
	}
}

func TestStateShow_emptyState(t *testing.T) {
	state := states.NewState()
	statePath := testStateFile(t, state)

	p := testProvider()
	streams, done := terminal.StreamsForTesting(t)
	c := &StateShowCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Streams:          streams,
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
	}
	if code := c.Run(args); code != 1 {
		t.Fatalf("bad: %d", code)
	}
	output := done(t)
	if !strings.Contains(output.Stderr(), "No instance found for the given address!") {
		t.Fatalf("expected a no instance found error, got: %s", output.Stderr())
	}
}

func TestStateShow_configured_provider(t *testing.T) {
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test-beta"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})
	statePath := testStateFile(t, state)

	p := testProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id":  {Type: cty.String, Optional: true, Computed: true},
						"foo": {Type: cty.String, Optional: true},
						"bar": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}

	streams, done := terminal.StreamsForTesting(t)
	c := &StateShowCommand{
		Meta: Meta{
			testingOverrides: &testingOverrides{
				Providers: map[addrs.Provider]providers.Factory{
					addrs.NewDefaultProvider("test-beta"): providers.FactoryFixed(p),
				},
			},
			Streams: streams,
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	// Test that outputs were displayed
	expected := strings.TrimSpace(testStateShowOutput) + "\n"
	actual := output.Stdout()
	if actual != expected {
		t.Fatalf("Expected:\n%q\n\nTo equal:\n%q", actual, expected)
	}
}

func TestStateShow_withoutShowSensitiveArg(t *testing.T) {
	state := stateWithSensitiveValueForStateShow()
	statePath := testStateFile(t, state)

	p := testProvider()
	p.GetProviderSchemaResponse = providerWithSensitiveValueForStateShow()

	streams, done := terminal.StreamsForTesting(t)
	c := &StateShowCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Streams:          streams,
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: \n%s", output.Stderr())
	}

	expected := `# test_instance.foo:
resource "test_instance" "foo" {
    bar = "value"
    foo = "value"
    id  = (sensitive value)
}`
	actual := strings.TrimSpace(output.Stdout())
	if diff := cmp.Diff(actual, expected); len(diff) > 0 {
		t.Fatalf("got incorrect output\n %v", diff)
	}
}

func TestStateShow_showSensitiveArg(t *testing.T) {
	state := stateWithSensitiveValueForStateShow()
	statePath := testStateFile(t, state)

	p := testProvider()
	p.GetProviderSchemaResponse = providerWithSensitiveValueForStateShow()

	streams, done := terminal.StreamsForTesting(t)
	c := &StateShowCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Streams:          streams,
		},
	}

	args := []string{
		"-show-sensitive",
		"-state", statePath,
		"test_instance.foo",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: \n%s", output.Stderr())
	}

	expected := `# test_instance.foo:
resource "test_instance" "foo" {
    bar = "value"
    foo = "value"
    id  = "bar"
}`
	actual := strings.TrimSpace(output.Stdout())
	if diff := cmp.Diff(actual, expected); len(diff) > 0 {
		t.Fatalf("got incorrect output\n %v", diff)
	}
}

// stateWithSensitiveValueForStateShow returns a state with a resource
// instance.
func stateWithSensitiveValueForStateShow() *states.State {
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})

	return state
}

// providerWithSensitiveValueForStateShow returns a provider schema response
// with the "id" attribute flagged as sensitive.
func providerWithSensitiveValueForStateShow() *providers.GetProviderSchemaResponse {
	return &providers.GetProviderSchemaResponse{
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id":  {Type: cty.String, Optional: true, Computed: true, Sensitive: true},
						"foo": {Type: cty.String, Optional: true},
						"bar": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}
}

const testStateShowOutput = `
# test_instance.foo:
resource "test_instance" "foo" {
    bar = "value"
    foo = "value"
    id  = "bar"
}
`
