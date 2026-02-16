// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseWorkspaceNew_viewOptions(t *testing.T) {
	tempDir := t.TempDir()
	testCases := map[string]struct {
		args         []string
		wantViewType ViewType
	}{
		"default view type": {
			args:         []string{"new-workspace"},
			wantViewType: ViewHuman,
		},
		"json view type": {
			args:         []string{"-json", "new-workspace"},
			wantViewType: ViewJSON,
		},
		"json-into": {
			args:         []string{fmt.Sprintf("-json-into=%s", filepath.Join(tempDir, "json-into")), "new-workspace"},
			wantViewType: ViewHuman,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseWorkspaceNew(tc.args)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if got.ViewOptions.ViewType != tc.wantViewType {
				t.Errorf("ViewOptions.ViewType = %v, want %v", got.ViewOptions.ViewType, tc.wantViewType)
			}
		})
	}
}

func TestParseWorkspaceNew_workspaceName(t *testing.T) {
	testCases := map[string]struct {
		args              []string
		wantName          string
		expectDiagnostics bool
	}{
		"valid workspace name": {
			args:              []string{"new-workspace"},
			wantName:          "new-workspace",
			expectDiagnostics: false,
		},
		"no workspace name": {
			args:              []string{},
			wantName:          "",
			expectDiagnostics: true,
		},
		"too many arguments": {
			args:              []string{"workspace1", "workspace2"},
			wantName:          "",
			expectDiagnostics: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseWorkspaceNew(tc.args)
			defer closer()

			if tc.expectDiagnostics && len(diags) == 0 {
				t.Fatal("expected diagnostics but got none")
			}
			if !tc.expectDiagnostics && len(diags) > 0 {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if !tc.expectDiagnostics && got.WorkspaceName != tc.wantName {
				t.Errorf("WorkspaceName = %q, want %q", got.WorkspaceName, tc.wantName)
			}
		})
	}
}

func TestParseWorkspaceNew_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args []string
		want *WorkspaceNew
	}{
		"defaults": {
			[]string{"target-ws"},
			workspaceNewArgsWithDefaults(func(in *WorkspaceNew) {
				in.WorkspaceName = "target-ws"
			}),
		},
		"state path": {
			[]string{"-state=/path/to/state.tfstate", "target-ws"},
			workspaceNewArgsWithDefaults(func(in *WorkspaceNew) {
				in.WorkspaceName = "target-ws"
				in.StatePath = "/path/to/state.tfstate"
			}),
		},
		"lock flag": {
			[]string{"-lock=false", "target-ws"},
			workspaceNewArgsWithDefaults(func(in *WorkspaceNew) {
				in.WorkspaceName = "target-ws"
				in.StateLock = false
			}),
		},
		"lock timeout flag": {
			[]string{"-lock-timeout=2s", "target-ws"},
			workspaceNewArgsWithDefaults(func(in *WorkspaceNew) {
				in.WorkspaceName = "target-ws"
				in.StateLockTimeout = 2 * time.Second
			}),
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Vars{}, ViewOptions{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseWorkspaceNew(tc.args)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if diff := cmp.Diff(tc.want, got, cmpOpts); diff != "" {
				t.Errorf("unexpected result\n%s", diff)
			}
		})
	}
}

func TestParseWorkspaceNew_vars(t *testing.T) {
	got, closer, diags := ParseWorkspaceNew([]string{"test-workspace"})
	defer closer()

	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got.Vars == nil {
		t.Fatal("Vars should never be nil")
	}

	if !got.Vars.Empty() {
		t.Errorf("expected Vars to be empty but got vars")
	}
}

func workspaceNewArgsWithDefaults(mutate func(in *WorkspaceNew)) *WorkspaceNew {
	ret := &WorkspaceNew{
		StatePath:        "",
		StateLock:        true,
		StateLockTimeout: 0,
		Vars:             &Vars{},
		ViewOptions: ViewOptions{
			ViewType: ViewHuman,
		},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
