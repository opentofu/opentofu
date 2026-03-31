// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/workdir"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/backend/local"
	"github.com/opentofu/opentofu/internal/tofu"
)

func TestMetaColorize(t *testing.T) {
	t.Run("with color enabled", func(t *testing.T) {
		view, done := testView(t)
		defer done(t)

		m := &Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		}
		args := []string{"foo", "bar"}
		wantArgs := []string{"foo", "bar"}
		viewArgs, args := arguments.ParseView(args)

		view.Configure(viewArgs)
		m.configureUiFromView(arguments.ViewOptions{ViewType: arguments.ViewHuman})

		if !reflect.DeepEqual(args, wantArgs) {
			t.Fatalf("bad: %#v", args)
		}
		if m.View.Colorize().Disable {
			t.Fatal("should not be disabled")
		}
	})

	// NOTE: the case that was removed from here was testing that without marking Meta.Color=true manually,
	// and having no -no-color flag, it was **disabled** by default.
	// When Meta was created in regular flow, the Meta.Color was set to "true".
	// During tests, Meta.Color is false, so it needs to be configured manually as "true".
	// This is what the test case above did before, but with the migration to the new view, the
	// logic is flipped, where the view.colorise.disabled is by default false and when -no-color
	// is given, it disables it.
	// Therefore, the case above, tests exactly what it was testing before the refactor, but
	// the test here, makes no more sense now.

	t.Run("one occurrence of -no-color flag", func(t *testing.T) {
		view, done := testView(t)
		defer done(t)

		m := &Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		}
		args := []string{"foo", "-no-color", "bar"}
		args2 := []string{"foo", "bar"}
		viewArgs, args := arguments.ParseView(args)

		view.Configure(viewArgs)
		m.configureUiFromView(arguments.ViewOptions{ViewType: arguments.ViewHuman})

		if !reflect.DeepEqual(args, args2) {
			t.Fatalf("bad: %#v", args)
		}
		if !m.View.Colorize().Disable {
			t.Fatal("should be disabled")
		}
	})

	t.Run("one occurrences of -no-color flag", func(t *testing.T) {
		view, done := testView(t)
		defer done(t)
		// Test disable #2
		// Verify multiple -no-color options are removed from args slice.
		// E.g. an additional -no-color arg could be added by TF_CLI_ARGS.
		m := &Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		}
		args := []string{"foo", "-no-color", "bar", "-no-color"}
		args2 := []string{"foo", "bar"}
		viewArgs, args := arguments.ParseView(args)

		view.Configure(viewArgs)
		m.configureUiFromView(arguments.ViewOptions{ViewType: arguments.ViewHuman})

		if !reflect.DeepEqual(args, args2) {
			t.Fatalf("bad: %#v", args)
		}
		if !m.View.Colorize().Disable {
			t.Fatal("should be disabled")
		}
	})
}

func TestMetaInputMode(t *testing.T) {
	test = false
	defer func() { test = true }()

	m := &Meta{
		WorkingDir: workdir.NewDir("."),
	}
	// TODO meta-refactor: these assignments are needed because the extendedFlagSet was used here before,
	//   which had these with defaults as "true". In a future iteration, once these are not needed, we need to remove them.
	m.input = true
	m.stateLock = true

	args := []string{}

	fs := flag.NewFlagSet("foo", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("err: %s", err)
	}

	if m.InputMode() != tofu.InputModeStd {
		t.Fatalf("bad: %#v", m.InputMode())
	}
}

func TestMetaInputMode_envVar(t *testing.T) {
	test = false
	defer func() { test = true }()

	m := &Meta{
		WorkingDir: workdir.NewDir("."),
	}
	// TODO meta-refactor: these assignments are needed because the extendedFlagSet was used here before,
	//   which had these with defaults as "true". In a future iteration, once these are not needed, we need to remove them.
	m.input = true
	m.stateLock = true

	args := []string{}

	fs := flag.NewFlagSet("foo", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("err: %s", err)
	}

	off := tofu.InputMode(0)
	on := tofu.InputModeStd
	cases := []struct {
		EnvVar   string
		Expected tofu.InputMode
	}{
		{"false", off},
		{"0", off},
		{"true", on},
		{"1", on},
	}

	for _, tc := range cases {
		t.Setenv(InputModeEnvVar, tc.EnvVar)
		if m.InputMode() != tc.Expected {
			t.Fatalf("expected InputMode: %#v, got: %#v", tc.Expected, m.InputMode())
		}
	}
}

func TestMetaInputMode_disable(t *testing.T) {
	test = false
	defer func() { test = true }()

	m := &Meta{
		WorkingDir: workdir.NewDir("."),
	}
	// TODO meta-refactor: these assignments are needed because the extendedFlagSet was used here before,
	//   which had these with defaults as "true". In a future iteration, once these are not needed, we need to remove them.
	m.input = true
	m.stateLock = true
	args := []string{"-input=false"}

	fs := flag.NewFlagSet("foo", flag.ContinueOnError)
	var viewOpts arguments.ViewOptions
	viewOpts.AddFlags(fs, true)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("err: %s", err)
	}
	if _, diags := viewOpts.Parse(); len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %s", diags)
	}
	m.input = viewOpts.InputEnabled

	if m.InputMode() > 0 {
		t.Fatalf("bad: %#v", m.InputMode())
	}
}

func TestMeta_initStatePaths(t *testing.T) {
	m := &Meta{
		WorkingDir: workdir.NewDir("."),
	}
	m.initStatePaths()

	if m.statePath != arguments.DefaultStateFilename {
		t.Fatalf("bad: %#v", m)
	}
	if m.stateOutPath != arguments.DefaultStateFilename {
		t.Fatalf("bad: %#v", m)
	}
	if m.backupPath != arguments.DefaultStateFilename+DefaultBackupExtension {
		t.Fatalf("bad: %#v", m)
	}

	m = &Meta{
		WorkingDir: workdir.NewDir("."),
	}
	m.statePath = "foo"
	m.initStatePaths()

	if m.stateOutPath != "foo" {
		t.Fatalf("bad: %#v", m)
	}
	if m.backupPath != "foo"+DefaultBackupExtension {
		t.Fatalf("bad: %#v", m)
	}

	m = &Meta{
		WorkingDir: workdir.NewDir("."),
	}
	m.stateOutPath = "foo"
	m.initStatePaths()

	if m.statePath != arguments.DefaultStateFilename {
		t.Fatalf("bad: %#v", m)
	}
	if m.backupPath != "foo"+DefaultBackupExtension {
		t.Fatalf("bad: %#v", m)
	}
}

func TestMeta_Env(t *testing.T) {
	td := t.TempDir()
	t.Chdir(td)

	m := &Meta{
		WorkingDir: workdir.NewDir("."),
	}

	env, err := m.Workspace(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if env != backend.DefaultStateName {
		t.Fatalf("expected env %q, got env %q", backend.DefaultStateName, env)
	}

	testEnv := "test_env"
	if err := m.SetWorkspace(testEnv); err != nil {
		t.Fatal("error setting env:", err)
	}

	env, _ = m.Workspace(t.Context())
	if env != testEnv {
		t.Fatalf("expected env %q, got env %q", testEnv, env)
	}

	if err := m.SetWorkspace(backend.DefaultStateName); err != nil {
		t.Fatal("error setting env:", err)
	}

	env, _ = m.Workspace(t.Context())
	if env != backend.DefaultStateName {
		t.Fatalf("expected env %q, got env %q", backend.DefaultStateName, env)
	}
}

func TestMeta_Workspace_override(t *testing.T) {
	m := &Meta{
		WorkingDir: workdir.NewDir("."),
	}

	testCases := map[string]struct {
		workspace string
		err       error
	}{
		"": {
			"default",
			nil,
		},
		"development": {
			"development",
			nil,
		},
		"invalid name": {
			"",
			errInvalidWorkspaceNameEnvVar,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Setenv(WorkspaceNameEnvVar, name)
			workspace, err := m.Workspace(t.Context())
			if workspace != tc.workspace {
				t.Errorf("Unexpected workspace\n got: %s\nwant: %s\n", workspace, tc.workspace)
			}
			if err != tc.err {
				t.Errorf("Unexpected error\n got: %s\nwant: %s\n", err, tc.err)
			}
		})
	}
}

func TestMeta_Workspace_invalidSelected(t *testing.T) {
	td := t.TempDir()
	t.Chdir(td)

	// this is an invalid workspace name
	workspace := "test workspace"

	// create the workspace directories
	if err := os.MkdirAll(filepath.Join(local.DefaultWorkspaceDir, workspace), 0755); err != nil {
		t.Fatal(err)
	}

	// create the workspace file to select it
	if err := os.MkdirAll(workdir.DefaultDataDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workdir.DefaultDataDir, local.DefaultWorkspaceFile), []byte(workspace), 0644); err != nil {
		t.Fatal(err)
	}

	m := &Meta{
		WorkingDir: workdir.NewDir("."),
	}

	ws, err := m.Workspace(t.Context())
	if ws != workspace {
		t.Errorf("Unexpected workspace\n got: %s\nwant: %s\n", ws, workspace)
	}
	if err != nil {
		t.Errorf("Unexpected error: %s", err)
	}
}

func TestCommand_checkRequiredVersion(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("command-check-required-version"), td)
	t.Chdir(td)

	view, done := testView(t)
	meta := Meta{
		WorkingDir: workdir.NewDir("."),
		View:       view,
	}

	diags := meta.checkRequiredVersion(t.Context())
	if diags == nil {
		t.Fatalf("diagnostics should contain unmet version constraint, but is nil")
	}

	view.Diagnostics(diags)

	// Required version diags are correct
	output := done(t)
	errStr := output.Stderr()
	if !strings.Contains(errStr, `required_version = "~> 0.9.0"`) {
		t.Fatalf("output should point to unmet version constraint, but is:\n\n%s", errStr)
	}
	if strings.Contains(errStr, `required_version = ">= 0.13.0"`) {
		t.Fatalf("output should not point to met version constraint, but is:\n\n%s", errStr)
	}
}
