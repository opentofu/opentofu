// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
)

func TestStatePull(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("state-pull-backend"), td)
	t.Chdir(td)

	expected, err := os.ReadFile("local-state.tfstate")
	if err != nil {
		t.Fatalf("error reading state: %v", err)
	}

	p := testProvider()
	ui := new(cli.MockUi)
	c := &StatePullCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	args := []string{}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, ui.ErrorWriter.String())
	}

	actual := ui.OutputWriter.Bytes()
	if bytes.Equal(actual, expected) {
		t.Fatalf("expected:\n%s\n\nto include: %q", actual, expected)
	}
}

func TestStatePull_noState(t *testing.T) {
	testCwdTemp(t)

	p := testProvider()
	ui := cli.NewMockUi()
	c := &StatePullCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	args := []string{}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, ui.ErrorWriter.String())
	}

	actual := ui.OutputWriter.String()
	if actual != "" {
		t.Fatalf("bad: %s", actual)
	}
}

func TestStatePull_checkRequiredVersion(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("command-check-required-version"), td)
	t.Chdir(td)

	p := testProvider()
	ui := cli.NewMockUi()
	c := &StatePullCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	args := []string{}
	if code := c.Run(args); code != 1 {
		t.Fatalf("got exit status %d; want 1\nstderr:\n%s\n\nstdout:\n%s", code, ui.ErrorWriter.String(), ui.OutputWriter.String())
	}

	// Required version diags are correct
	errStr := ui.ErrorWriter.String()
	if !strings.Contains(errStr, `required_version = "~> 0.9.0"`) {
		t.Fatalf("output should point to unmet version constraint, but is:\n\n%s", errStr)
	}
	if strings.Contains(errStr, `required_version = ">= 0.13.0"`) {
		t.Fatalf("output should not point to met version constraint, but is:\n\n%s", errStr)
	}
}
