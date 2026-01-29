// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/getmodules"
)

func TestGet(t *testing.T) {
	wd := tempWorkingDirFixture(t, "get")
	t.Chdir(wd.RootModuleDir())

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
	t.Chdir(wd.RootModuleDir())

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
	t.Chdir(wd.RootModuleDir())

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
	// This test runs `tofu get` against a server that stalls indefinitely
	// instead of responding, and then requests shutdown in the same way
	// as package main would in response to SIGINT (or similar on other
	// platforms). This ensures that slow requests can be interrupted.

	wd := tempWorkingDirFixture(t, "init-module-early-eval")
	t.Chdir(wd.RootModuleDir())

	// One failure mode of this test is for the cancellation to fail and
	// so the command runs indefinitely, and so we'll impose a timeout
	// to allow us to eventually catch that and diagnose it as a test
	// failure message.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// This server intentionally stalls any incoming request by leaving
	// the connection open but not responding. "reqs" will become
	// readable each time a request arrives.
	server, reqs := testHangServer(t)

	// We'll close this channel once we've been notified that the server
	// received our request, which should then cause cancellation.
	shutdownCh := make(chan struct{})
	go func() {
		select {
		case <-reqs:
			// Request received, so time to interrupt.
			t.Log("server received request, but won't respond")
			close(shutdownCh)
		case <-ctx.Done():
			// Exit early if we reach our timeout.
			t.Log("timeout before server received request")
		}
		server.CloseClientConnections() // force any active client request to fail
	}()

	ui := cli.NewMockUi()
	c := &GetCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
			WorkingDir:       wd,
			ShutdownCh:       shutdownCh,

			// This test needs a real module package fetcher instance because
			// we want to attempt installing a module package from our server.
			ModulePackageFetcher: getmodules.NewPackageFetcher(t.Context(), nil),
		},
	}

	fakeModuleSourceAddr := server.URL + "/example.zip"
	t.Logf("attempting to install module package from %s", fakeModuleSourceAddr)
	args := []string{"-var=module_source=" + fakeModuleSourceAddr}
	code := c.Run(args)
	if err := ctx.Err(); err != nil {
		t.Errorf("context error: %s", err) // probably reporting a timeout
	}
	if code == 0 {
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
			t.Chdir(wd.RootModuleDir())

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
