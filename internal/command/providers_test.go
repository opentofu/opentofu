// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/command/workdir"
)

func TestProviders(t *testing.T) {
	t.Chdir(testFixturePath("providers/basic"))

	view, done := testView(t)
	c := &ProvidersCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	code := c.Run(nil)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	wantOutput := []string{
		"provider[registry.opentofu.org/hashicorp/foo]",
		"provider[registry.opentofu.org/hashicorp/bar]",
		"provider[registry.opentofu.org/hashicorp/baz]",
	}

	stdout := output.Stdout()
	for _, want := range wantOutput {
		if !strings.Contains(stdout, want) {
			t.Errorf("output missing %s:\n%s", want, stdout)
		}
	}
}

func TestProviders_noConfigs(t *testing.T) {
	t.Chdir(testFixturePath(""))

	view, done := testView(t)
	c := &ProvidersCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	code := c.Run(nil)
	output := done(t)
	if code == 0 {
		t.Fatal("expected command to return non-zero exit code" +
			" when no configs are available")
	}

	stderr := output.Stderr()
	expectedErrMsg := "No configuration files"
	if !strings.Contains(stderr, expectedErrMsg) {
		t.Errorf("Expected error message: %s\nGiven output: %s", expectedErrMsg, stderr)
	}
}

func TestProviders_modules(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("providers/modules"), td)
	t.Chdir(td)

	// first run init with mock provider sources to install the module
	providerSource, closer := newMockProviderSource(t, map[string][]string{
		"foo": {"1.0.0"},
		"bar": {"2.0.0"},
		"baz": {"1.2.2"},
	})
	defer closer()
	initView, initDone := testView(t)
	initMeta := Meta{
		WorkingDir:       workdir.NewDir("."),
		testingOverrides: metaOverridesForProvider(testProvider()),
		View:             initView,
		ProviderSource:   providerSource,
	}
	ic := &InitCommand{
		Meta: initMeta,
	}
	code := ic.Run(nil)
	initOutput := initDone(t)
	if code != 0 {
		t.Fatalf("init failed\n%s", initOutput.Stderr())
	}

	// Providers command
	view, done := testView(t)
	c := &ProvidersCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	code = c.Run(nil)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	wantOutput := []string{
		"provider[registry.opentofu.org/hashicorp/foo] 1.0.0", // from required_providers
		"provider[registry.opentofu.org/hashicorp/bar] 2.0.0", // from provider config
		"── module.kiddo", // tree node for child module
		"provider[registry.opentofu.org/hashicorp/baz]", // implied by a resource in the child module
	}

	stdout := output.Stdout()
	for _, want := range wantOutput {
		if !strings.Contains(stdout, want) {
			t.Errorf("output missing %s:\n%s", want, stdout)
		}
	}
}

func TestProviders_state(t *testing.T) {
	t.Chdir(testFixturePath("providers/state"))

	view, done := testView(t)
	c := &ProvidersCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	code := c.Run(nil)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	wantOutput := []string{
		"provider[registry.opentofu.org/hashicorp/foo] 1.0.0", // from required_providers
		"provider[registry.opentofu.org/hashicorp/bar] 2.0.0", // from a provider config block
		"Providers required by state",                         // header for state providers
		"provider[registry.opentofu.org/hashicorp/baz]",       // from a resource in state (only)
	}

	stdout := output.Stdout()
	for _, want := range wantOutput {
		if !strings.Contains(stdout, want) {
			t.Errorf("output missing %s:\n%s", want, stdout)
		}
	}
}

func TestProviders_tests(t *testing.T) {
	t.Chdir(testFixturePath("providers/tests"))

	view, done := testView(t)
	c := &ProvidersCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	code := c.Run(nil)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	wantOutput := []string{
		"provider[registry.opentofu.org/hashicorp/foo]",
		"test.main",
		"provider[registry.opentofu.org/hashicorp/bar]",
	}

	stdout := output.Stdout()
	for _, want := range wantOutput {
		if !strings.Contains(stdout, want) {
			t.Errorf("output missing %s:\n%s", want, stdout)
		}
	}
}
