// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"

	"github.com/opentofu/opentofu/internal/getmodules"
)

func TestGet(t *testing.T) {
	wd := tempWorkingDirFixture(t, "get")
	defer testChdir(t, wd.RootModuleDir())()

	ui := cli.NewMockUi()
	c := &GetCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
			WorkingDir:       wd,
		},
	}

	args := []string{}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: \n%s", ui.ErrorWriter.String())
	}

	output := ui.OutputWriter.String()
	if !strings.Contains(output, "- foo in") {
		t.Fatalf("doesn't look like get: %s", output)
	}
}

func TestGet_multipleArgs(t *testing.T) {
	wd := tempWorkingDir(t)
	defer testChdir(t, wd.RootModuleDir())()

	ui := cli.NewMockUi()
	c := &GetCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
			WorkingDir:       wd,
		},
	}

	args := []string{
		"bad",
		"bad",
	}
	if code := c.Run(args); code != 1 {
		t.Fatalf("bad: \n%s", ui.OutputWriter.String())
	}
}

func TestGet_update(t *testing.T) {
	wd := tempWorkingDirFixture(t, "get")
	defer testChdir(t, wd.RootModuleDir())()

	ui := cli.NewMockUi()
	c := &GetCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
			WorkingDir:       wd,
		},
	}

	args := []string{
		"-update",
	}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: \n%s", ui.ErrorWriter.String())
	}

	output := ui.OutputWriter.String()
	if !strings.Contains(output, `- foo in`) {
		t.Fatalf("doesn't look like get: %s", output)
	}
}

func TestGet_cancel(t *testing.T) {
	// This test runs `tofu get` as if SIGINT (or similar on other
	// platforms) were sent to it, testing that it is interruptible.

	wd := tempWorkingDirFixture(t, "init-registry-module")
	defer testChdir(t, wd.RootModuleDir())()

	// Our shutdown channel is pre-closed so init will exit as soon as it
	// starts a cancelable portion of the process.
	shutdownCh := make(chan struct{})
	close(shutdownCh)

	ui := cli.NewMockUi()
	c := &GetCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
			WorkingDir:       wd,
			ShutdownCh:       shutdownCh,

			// This test needs a real module package fetcher instance because
			// its configuration includes a reference to a module from a registry
			// that doesn't really exist. The shutdown signal prevents us from
			// actually making a request to this, but we still need to provide
			// the fetcher so that it will _attempt_ to make a network request
			// that can then fail with a cancellation error.
			ModulePackageFetcher: getmodules.NewPackageFetcher(nil),
		},
	}

	args := []string{}
	if code := c.Run(args); code == 0 {
		t.Fatalf("succeeded; wanted error\n%s", ui.OutputWriter.String())
	}

	if got, want := ui.ErrorWriter.String(), `Module installation was canceled by an interrupt signal`; !strings.Contains(got, want) {
		t.Fatalf("wrong error message\nshould contain: %s\ngot:\n%s", want, got)
	}
}

func TestGetCommand_InvalidArgs(t *testing.T) {
	testCases := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "TooManyArgs",
			args:     []string{"too", "many", "args"},
			expected: "Too many command line arguments",
		},
		{
			name:     "InvalidArgFormat",
			args:     []string{"--invalid-arg"},
			expected: "flag provided but not defined",
		},
		{
			name:     "MixedValidAndInvalidArgs",
			args:     []string{"--update", "--invalid-arg"},
			expected: "flag provided but not defined",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wd := tempWorkingDirFixture(t, "get")
			defer testChdir(t, wd.RootModuleDir())()

			ui := cli.NewMockUi()
			c := &GetCommand{
				Meta: Meta{
					testingOverrides: metaOverridesForProvider(testProvider()),
					Ui:               ui,
					WorkingDir:       wd,
				},
			}

			if code := c.Run(tc.args); code != 1 {
				t.Errorf("Expected error code 1 for invalid arguments, got %d", code)
			}

			if !strings.Contains(ui.ErrorWriter.String(), tc.expected) {
				t.Errorf("Expected error message to contain '%s', got: %s", tc.expected, ui.ErrorWriter.String())
			}
		})
	}
}
