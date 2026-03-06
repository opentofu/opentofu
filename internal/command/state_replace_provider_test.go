// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/command/workdir"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
)

func TestStateReplaceProvider(t *testing.T) {
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "aws_instance",
				Name: "alpha",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"alpha","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("aws"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "aws_instance",
				Name: "beta",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"beta","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("aws"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "azurerm_virtual_machine",
				Name: "gamma",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"gamma","baz":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewLegacyProvider("azurerm"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})

	t.Run("happy path", func(t *testing.T) {
		statePath := testStateFile(t, state)

		view, done := testView(t)
		c := &StateReplaceProviderCommand{
			StateMeta{
				Meta: Meta{
					WorkingDir: workdir.NewDir("."),
					View:       view,
				},
			},
		}

		defer testInputMap(t, map[string]string{
			"confirm": "yes",
		})()

		args := []string{
			"-state", statePath,
			"hashicorp/aws",
			"acmecorp/aws",
		}
		code := c.Run(args)
		output := done(t)
		if code != 0 {
			t.Fatalf("return code: %d\n\n%s", code, output.Stderr())
		}

		testStateOutput(t, statePath, testStateReplaceProviderOutput)

		backups := testStateBackups(t, filepath.Dir(statePath))
		if len(backups) != 1 {
			t.Fatalf("unexpected backups: %#v", backups)
		}
		testStateOutput(t, backups[0], testStateReplaceProviderOutputOriginal)
	})

	t.Run("auto approve", func(t *testing.T) {
		statePath := testStateFile(t, state)

		view, done := testView(t)
		c := &StateReplaceProviderCommand{
			StateMeta{
				Meta: Meta{
					WorkingDir: workdir.NewDir("."),
					View:       view,
				},
			},
		}
		defer testInputMap(t, map[string]string{})()

		args := []string{
			"-state", statePath,
			"-auto-approve",
			"hashicorp/aws",
			"acmecorp/aws",
		}
		code := c.Run(args)
		output := done(t)
		if code != 0 {
			t.Fatalf("return code: %d\n\n%s", code, output.Stderr())
		}

		testStateOutput(t, statePath, testStateReplaceProviderOutput)

		backups := testStateBackups(t, filepath.Dir(statePath))
		if len(backups) != 1 {
			t.Fatalf("unexpected backups: %#v", backups)
		}
		testStateOutput(t, backups[0], testStateReplaceProviderOutputOriginal)
	})

	t.Run("cancel at approval step", func(t *testing.T) {
		statePath := testStateFile(t, state)

		view, done := testView(t)
		c := &StateReplaceProviderCommand{
			StateMeta{
				Meta: Meta{
					WorkingDir: workdir.NewDir("."),
					View:       view,
				},
			},
		}
		defer testInputMap(t, map[string]string{
			"confirm": "no",
		})()

		args := []string{
			"-state", statePath,
			"hashicorp/aws",
			"acmecorp/aws",
		}
		code := c.Run(args)
		output := done(t)
		if code != 0 {
			t.Fatalf("return code: %d\n\n%s", code, output.Stderr())
		}

		testStateOutput(t, statePath, testStateReplaceProviderOutputOriginal)

		backups := testStateBackups(t, filepath.Dir(statePath))
		if len(backups) != 0 {
			t.Fatalf("unexpected backups: %#v", backups)
		}
	})

	t.Run("no matching provider found", func(t *testing.T) {
		statePath := testStateFile(t, state)

		view, done := testView(t)
		c := &StateReplaceProviderCommand{
			StateMeta{
				Meta: Meta{
					WorkingDir: workdir.NewDir("."),
					View:       view,
				},
			},
		}

		args := []string{
			"-state", statePath,
			"hashicorp/google",
			"acmecorp/google",
		}
		code := c.Run(args)
		output := done(t)
		if code != 0 {
			t.Fatalf("return code: %d\n\n%s", code, output.Stderr())
		}

		testStateOutput(t, statePath, testStateReplaceProviderOutputOriginal)

		backups := testStateBackups(t, filepath.Dir(statePath))
		if len(backups) != 0 {
			t.Fatalf("unexpected backups: %#v", backups)
		}
	})

	t.Run("invalid flags", func(t *testing.T) {
		view, done := testView(t)
		c := &StateReplaceProviderCommand{
			StateMeta{
				Meta: Meta{
					WorkingDir: workdir.NewDir("."),
					View:       view,
				},
			},
		}

		args := []string{
			"-no-color",
			"-invalid",
			"hashicorp/google",
			"acmecorp/google",
		}
		code := c.Run(args)
		output := done(t)
		if code == 0 {
			t.Fatalf("successful exit; want error")
		}

		if got, want := output.Stderr(), "Error parsing command-line flags"; !strings.Contains(got, want) {
			t.Fatalf("missing expected error message\nwant: %s\nfull output:\n%s", want, got)
		}
	})

	t.Run("wrong number of arguments", func(t *testing.T) {
		view, done := testView(t)
		c := &StateReplaceProviderCommand{
			StateMeta{
				Meta: Meta{
					WorkingDir: workdir.NewDir("."),
					View:       view,
				},
			},
		}

		args := []string{"a", "b", "c", "d"}
		code := c.Run(args)
		output := done(t)
		if code == 0 {
			t.Fatalf("successful exit; want error")
		}

		if got, want := output.Stderr(), "Exactly two arguments expected"; !strings.Contains(got, want) {
			t.Fatalf("missing expected error message\nwant: %s\nfull output:\n%s", want, got)
		}
	})

	t.Run("invalid provider strings", func(t *testing.T) {
		view, done := testView(t)
		c := &StateReplaceProviderCommand{
			StateMeta{
				Meta: Meta{
					WorkingDir: workdir.NewDir("."),
					View:       view,
				},
			},
		}

		args := []string{
			"hashicorp/google_cloud",
			"-/-/google",
		}
		code := c.Run(args)
		output := done(t)
		if code == 0 {
			t.Fatalf("successful exit; want error")
		}

		got := output.Stderr()
		msgs := []string{
			`Invalid "from" provider "hashicorp/google_cloud"`,
			"Invalid provider type",
			`Invalid "to" provider "-/-/google"`,
			"Invalid provider source hostname",
		}
		for _, msg := range msgs {
			if !strings.Contains(got, msg) {
				t.Errorf("missing expected error message\nwant: %s\nfull output:\n%s", msg, got)
			}
		}
	})
}

func TestStateReplaceProvider_docs(t *testing.T) {
	c := &StateReplaceProviderCommand{}

	if got, want := c.Help(), "Usage: tofu [global options] state replace-provider"; !strings.Contains(got, want) {
		t.Fatalf("unexpected help text\nwant: %s\nfull output:\n%s", want, got)
	}

	if got, want := c.Synopsis(), "Replace provider in the state"; got != want {
		t.Fatalf("unexpected synopsis\nwant: %s\nfull output:\n%s", want, got)
	}
}

func TestStateReplaceProvider_checkRequiredVersion(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("command-check-required-version"), td)
	t.Chdir(td)

	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "aws_instance",
				Name: "alpha",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"alpha","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("aws"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "aws_instance",
				Name: "beta",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"beta","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("aws"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "azurerm_virtual_machine",
				Name: "gamma",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"gamma","baz":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewLegacyProvider("azurerm"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})

	statePath := testStateFile(t, state)

	view, done := testView(t)
	c := &StateReplaceProviderCommand{
		StateMeta{
			Meta: Meta{
				WorkingDir: workdir.NewDir("."),
				View:       view,
			},
		},
	}

	defer testInputMap(t, map[string]string{})()

	args := []string{
		"-state", statePath,
		"hashicorp/aws",
		"acmecorp/aws",
	}
	code := c.Run(args)
	output := done(t)
	if code != 1 {
		t.Fatalf("got exit status %d; want 1\nstderr:\n%s\n\nstdout:\n%s", code, output.Stderr(), output.Stdout())
	}

	// State is unchanged
	testStateOutput(t, statePath, testStateReplaceProviderOutputOriginal)

	// Required version diags are correct
	errStr := output.Stderr()
	if !strings.Contains(errStr, `required_version = "~> 0.9.0"`) {
		t.Fatalf("output should point to unmet version constraint, but is:\n\n%s", errStr)
	}
	if strings.Contains(errStr, `required_version = ">= 0.13.0"`) {
		t.Fatalf("output should not point to met version constraint, but is:\n\n%s", errStr)
	}
}

const testStateReplaceProviderOutputOriginal = `
aws_instance.alpha:
  ID = alpha
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  bar = value
  foo = value
aws_instance.beta:
  ID = beta
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  bar = value
  foo = value
azurerm_virtual_machine.gamma:
  ID = gamma
  provider = provider["registry.opentofu.org/-/azurerm"]
  baz = value
`

const testStateReplaceProviderOutput = `
aws_instance.alpha:
  ID = alpha
  provider = provider["registry.opentofu.org/acmecorp/aws"]
  bar = value
  foo = value
aws_instance.beta:
  ID = beta
  provider = provider["registry.opentofu.org/acmecorp/aws"]
  bar = value
  foo = value
azurerm_virtual_machine.gamma:
  ID = gamma
  provider = provider["registry.opentofu.org/-/azurerm"]
  baz = value
`
