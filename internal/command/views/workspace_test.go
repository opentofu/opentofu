// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestWorkspaceViews(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(workspace Workspace)
		wantJson   []map[string]any
		wantStdout string
		wantStderr string
	}{
		"workspace_already_exists": {
			viewCall: func(workspace Workspace) {
				workspace.WorkspaceAlreadyExists("test-workspace")
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": `Workspace "test-workspace" already exists`,
					"@module":  "tofu.ui",
				},
			},
			wantStderr: withNewline(`Workspace "test-workspace" already exists`),
		},
		"workspace_does_not_exist": {
			viewCall: func(workspace Workspace) {
				workspace.WorkspaceDoesNotExist("missing-workspace")
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": `Workspace "missing-workspace" doesn't exist. You can create this workspace with the "new" subcommand or include the "-or-create" flag with the "select" subcommand`,
					"@module":  "tofu.ui",
				},
			},
			wantStderr: withNewline("\nWorkspace \"missing-workspace\" doesn't exist.\n\nYou can create this workspace with the \"new\" subcommand \nor include the \"-or-create\" flag with the \"select\" subcommand."),
		},
		"workspace_invalid_name": {
			viewCall: func(workspace Workspace) {
				workspace.WorkspaceInvalidName("invalid/name")
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": `The workspace name "invalid/name" is not allowed. The name must contain only URL safe characters, and no path separators`,
					"@module":  "tofu.ui",
				},
			},
			wantStderr: withNewline("\nThe workspace name \"invalid/name\" is not allowed. The name must contain only URL safe\ncharacters, and no path separators.\n"),
		},
		"list_workspaces": {
			viewCall: func(workspace Workspace) {
				workspace.ListWorkspaces([]string{"default", "dev", "prod"}, "dev")
			},
			wantJson: []map[string]any{
				{
					"@level":     "info",
					"@message":   "workspace listing",
					"@module":    "tofu.ui",
					"workspace":  "default",
					"is_current": false,
				},
				{
					"@level":     "info",
					"@message":   "workspace listing",
					"@module":    "tofu.ui",
					"workspace":  "dev",
					"is_current": true,
				},
				{
					"@level":     "info",
					"@message":   "workspace listing",
					"@module":    "tofu.ui",
					"workspace":  "prod",
					"is_current": false,
				},
			},
			wantStdout: withNewline("  default\n* dev\n  prod\n"),
		},
		"workspace_overwritten_by_env_var_warn": {
			viewCall: func(workspace Workspace) {
				workspace.WorkspaceOverwrittenByEnvVarWarn()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "The active workspace is being overridden using the TF_WORKSPACE environment variable",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("\n\nThe active workspace is being overridden using the TF_WORKSPACE environment\nvariable.\n"),
		},
		"workspace_created": {
			viewCall: func(workspace Workspace) {
				workspace.WorkspaceCreated("new-workspace")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": `Created and switched to workspace "new-workspace". You're now on a new, empty workspace. Workspaces isolate their state, so if you run "tofu plan" OpenTofu will not see any existing state for this configuration`,
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("Created and switched to workspace \"new-workspace\"!\n\nYou're now on a new, empty workspace. Workspaces isolate their state,\nso if you run \"tofu plan\" OpenTofu will not see any existing state\nfor this configuration."),
		},
		"workspace_changed": {
			viewCall: func(workspace Workspace) {
				workspace.WorkspaceChanged("other-workspace")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": `Switched to workspace "other-workspace"`,
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline(`Switched to workspace "other-workspace".`),
		},
		"workspace_is_overridden_select_error": {
			viewCall: func(workspace Workspace) {
				workspace.WorkspaceIsOverriddenSelectError()
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "The selected workspace is currently overridden using the TF_WORKSPACE environment variable. To select a new workspace, either update this environment variable or unset it and then run this command again",
					"@module":  "tofu.ui",
				},
			},
			wantStderr: withNewline("\nThe selected workspace is currently overridden using the TF_WORKSPACE\nenvironment variable.\n\nTo select a new workspace, either update this environment variable or unset\nit and then run this command again.\n"),
		},
		"workspace_is_overridden_new_error": {
			viewCall: func(workspace Workspace) {
				workspace.WorkspaceIsOverriddenNewError()
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "The workspace is currently overridden using the TF_WORKSPACE environment variable. You cannot create a new workspace when using this setting. To create a new workspace, either unset this environment variable or update it to match the workspace name you are trying to create, and then run this command again",
					"@module":  "tofu.ui",
				},
			},
			wantStderr: withNewline("\nThe workspace is currently overridden using the TF_WORKSPACE environment\nvariable. You cannot create a new workspace when using this setting.\n\nTo create a new workspace, either unset this environment variable or update it\nto match the workspace name you are trying to create, and then run this command\nagain.\n"),
		},
		"workspace_deleted": {
			viewCall: func(workspace Workspace) {
				workspace.WorkspaceDeleted("old-workspace")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": `Deleted workspace "old-workspace"`,
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline(`Deleted workspace "old-workspace"!`),
		},
		"deleted_workspace_not_empty": {
			viewCall: func(workspace Workspace) {
				workspace.DeletedWorkspaceNotEmpty("workspace-with-resources")
			},
			wantJson: []map[string]any{
				{
					"@level":   "warn",
					"@message": `WARNING: "workspace-with-resources" was non-empty. The resources managed by the deleted workspace may still exist, but are no longer manageable by OpenTofu since the state has been deleted`,
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("WARNING: \"workspace-with-resources\" was non-empty.\nThe resources managed by the deleted workspace may still exist,\nbut are no longer manageable by OpenTofu since the state has\nbeen deleted."),
		},
		"cannot_delete_current_workspace": {
			viewCall: func(workspace Workspace) {
				workspace.CannotDeleteCurrentWorkspace("current-workspace")
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": `Workspace "current-workspace" is your active workspace. You cannot delete the currently active workspace. Please switch to another workspace and try again`,
					"@module":  "tofu.ui",
				},
			},
			wantStderr: withNewline("\nWorkspace \"current-workspace\" is your active workspace.\n\nYou cannot delete the currently active workspace. Please switch\nto another workspace and try again."),
		},
		"workspace_show": {
			viewCall: func(workspace Workspace) {
				workspace.WorkspaceShow("my-workspace")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "my-workspace",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("my-workspace"),
		},
		"warn_when_used_as_env_cmd_true": {
			viewCall: func(workspace Workspace) {
				workspace.WarnWhenUsedAsEnvCmd(true)
			},
			wantJson: []map[string]any{
				{
					"@level":   "warn",
					"@message": `The "tofu env" family of commands is deprecated. Use "tofu workspace" instead`,
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("Warning: the \"tofu env\" family of commands is deprecated.\n\n\"Workspace\" is now the preferred term for what earlier OpenTofu versions\ncalled \"environment\", to reduce ambiguity caused by the latter term colliding\nwith other concepts.\n\nThe \"tofu workspace\" commands should be used instead. \"tofu env\"\nwill be removed in a future OpenTofu version.\n"),
		},
		"warn_when_used_as_env_cmd_false": {
			viewCall: func(workspace Workspace) {
				workspace.WarnWhenUsedAsEnvCmd(false)
			},
			wantJson:   []map[string]any{{}},
			wantStdout: "",
		},
		// Diagnostics
		"diagnostics_warning": {
			viewCall: func(workspace Workspace) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A workspace warning", "workspace warning detail"),
				}
				workspace.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A workspace warning\n\nworkspace warning detail"),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "warn",
					"@message": "Warning: A workspace warning",
					"@module":  "tofu.ui",
					"diagnostic": map[string]any{
						"detail":   "workspace warning detail",
						"severity": "warning",
						"summary":  "A workspace warning",
					},
					"type": "diagnostic",
				},
			},
		},
		"diagnostics_error": {
			viewCall: func(workspace Workspace) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "A workspace error", "workspace error detail"),
				}
				workspace.Diagnostics(diags)
			},
			wantStdout: "",
			wantStderr: withNewline("\nError: A workspace error\n\nworkspace error detail"),
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "Error: A workspace error",
					"@module":  "tofu.ui",
					"diagnostic": map[string]any{
						"detail":   "workspace error detail",
						"severity": "error",
						"summary":  "A workspace error",
					},
					"type": "diagnostic",
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testWorkspaceHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
			testWorkspaceJson(t, tc.viewCall, tc.wantJson)
			testWorkspaceMulti(t, tc.viewCall, tc.wantStdout, tc.wantStderr, tc.wantJson)
		})
	}
}

func testWorkspaceHuman(t *testing.T, call func(workspace Workspace), wantStdout, wantStderr string) {
	view, done := testView(t)
	workspaceView := NewWorkspace(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
	call(workspaceView)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}

func testWorkspaceJson(t *testing.T, call func(workspace Workspace), want []map[string]interface{}) {
	view, done := testView(t)
	workspaceView := NewWorkspace(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view)
	call(workspaceView)
	output := done(t)
	if output.Stderr() != "" {
		t.Errorf("expected no stderr but got:\n%s", output.Stderr())
	}

	testJSONViewOutputEquals(t, output.Stdout(), want)
}

func testWorkspaceMulti(t *testing.T, call func(workspace Workspace), wantStdout string, wantStderr string, want []map[string]interface{}) {
	jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
	if err != nil {
		t.Fatalf("failed to create the file to write json content into: %s", err)
	}
	view, done := testView(t)
	workspaceView := NewWorkspace(arguments.ViewOptions{ViewType: arguments.ViewHuman, JSONInto: jsonInto}, view)
	call(workspaceView)
	{
		if err := jsonInto.Close(); err != nil {
			t.Fatalf("failed to close the jsonInto file: %s", err)
		}
		// check the fileInto content
		fileContent, err := os.ReadFile(jsonInto.Name())
		if err != nil {
			t.Fatalf("failed to read the file content with the json output: %s", err)
		}
		testJSONViewOutputEquals(t, string(fileContent), want)
	}
	{
		// check the human output
		output := done(t)
		if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
			t.Errorf("invalid stderr (-want, +got):\n%s", diff)
		}
		if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
			t.Errorf("invalid stdout (-want, +got):\n%s", diff)
		}
	}
}
