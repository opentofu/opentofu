// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"
)

func TestProviders(t *testing.T) {
	t.Chdir(testFixturePath("providers/basic"))

	ui := new(cli.MockUi)
	c := &ProvidersCommand{
		Meta: Meta{
			Ui: ui,
		},
	}

	args := []string{}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, ui.ErrorWriter.String())
	}

	wantOutput := []string{
		"provider[registry.opentofu.org/hashicorp/foo]",
		"provider[registry.opentofu.org/hashicorp/bar]",
		"provider[registry.opentofu.org/hashicorp/baz]",
	}

	output := ui.OutputWriter.String()
	for _, want := range wantOutput {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %s:\n%s", want, output)
		}
	}
}

func TestProviders_noConfigs(t *testing.T) {
	t.Chdir(testFixturePath(""))

	ui := new(cli.MockUi)
	c := &ProvidersCommand{
		Meta: Meta{
			Ui: ui,
		},
	}

	args := []string{}
	if code := c.Run(args); code == 0 {
		t.Fatal("expected command to return non-zero exit code" +
			" when no configs are available")
	}

	output := ui.ErrorWriter.String()
	expectedErrMsg := "No configuration files"
	if !strings.Contains(output, expectedErrMsg) {
		t.Errorf("Expected error message: %s\nGiven output: %s", expectedErrMsg, output)
	}
}

func TestProviders_modules(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("providers/modules"), td)
	t.Chdir(td)

	// first run init with mock provider sources to install the module
	initUi := new(cli.MockUi)
	providerSource, close := newMockProviderSource(t, map[string][]string{
		"foo": {"1.0.0"},
		"bar": {"2.0.0"},
		"baz": {"1.2.2"},
	})
	defer close()
	m := Meta{
		testingOverrides: metaOverridesForProvider(testProvider()),
		Ui:               initUi,
		ProviderSource:   providerSource,
	}
	ic := &InitCommand{
		Meta: m,
	}
	if code := ic.Run([]string{}); code != 0 {
		t.Fatalf("init failed\n%s", initUi.ErrorWriter)
	}

	// Providers command
	ui := new(cli.MockUi)
	c := &ProvidersCommand{
		Meta: Meta{
			Ui: ui,
		},
	}

	args := []string{}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, ui.ErrorWriter.String())
	}

	wantOutput := []string{
		"provider[registry.opentofu.org/hashicorp/foo] 1.0.0", // from required_providers
		"provider[registry.opentofu.org/hashicorp/bar] 2.0.0", // from provider config
		"── module.kiddo",                               // tree node for child module
		"provider[registry.opentofu.org/hashicorp/baz]", // implied by a resource in the child module
	}

	output := ui.OutputWriter.String()
	for _, want := range wantOutput {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %s:\n%s", want, output)
		}
	}
}

func TestProviders_state(t *testing.T) {
	t.Chdir(testFixturePath("providers/state"))

	ui := new(cli.MockUi)
	c := &ProvidersCommand{
		Meta: Meta{
			Ui: ui,
		},
	}

	args := []string{}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, ui.ErrorWriter.String())
	}

	wantOutput := []string{
		"provider[registry.opentofu.org/hashicorp/foo] 1.0.0", // from required_providers
		"provider[registry.opentofu.org/hashicorp/bar] 2.0.0", // from a provider config block
		"Providers required by state",                         // header for state providers
		"provider[registry.opentofu.org/hashicorp/baz]",       // from a resource in state (only)
	}

	output := ui.OutputWriter.String()
	for _, want := range wantOutput {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %s:\n%s", want, output)
		}
	}
}

func TestProviders_tests(t *testing.T) {
	t.Chdir(testFixturePath("providers/tests"))

	ui := new(cli.MockUi)
	c := &ProvidersCommand{
		Meta: Meta{
			Ui: ui,
		},
	}

	args := []string{}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, ui.ErrorWriter.String())
	}

	wantOutput := []string{
		"provider[registry.opentofu.org/hashicorp/foo]",
		"test.main",
		"provider[registry.opentofu.org/hashicorp/bar]",
	}

	output := ui.OutputWriter.String()
	for _, want := range wantOutput {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %s:\n%s", want, output)
		}
	}
}
