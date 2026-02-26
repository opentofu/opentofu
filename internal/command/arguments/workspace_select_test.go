// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestParseWorkspaceSelect_viewOptions(t *testing.T) {
	tempDir := t.TempDir()
	testCases := map[string]struct {
		args         []string
		wantViewType ViewType
	}{
		"default view type": {
			args:         []string{"workspace-name"},
			wantViewType: ViewHuman,
		},
		"json view type": {
			args:         []string{"-json", "workspace-name"},
			wantViewType: ViewJSON,
		},
		"json-into": {
			args:         []string{fmt.Sprintf("-json-into=%s", filepath.Join(tempDir, "json-into")), "workspace-name"},
			wantViewType: ViewHuman,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseWorkspaceSelect(tc.args)
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

func TestParseWorkspaceSelect_workspaceName(t *testing.T) {
	testCases := map[string]struct {
		args              []string
		wantName          string
		expectDiagnostics bool
	}{
		"valid workspace name": {
			args:              []string{"my-workspace"},
			wantName:          "my-workspace",
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
			got, closer, diags := ParseWorkspaceSelect(tc.args)
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

func TestParseWorkspaceSelect_createIfMissing(t *testing.T) {
	testCases := map[string]struct {
		args                []string
		wantCreateIfMissing bool
	}{
		"default": {
			args:                []string{"my-workspace"},
			wantCreateIfMissing: false,
		},
		"with -or-create flag": {
			args:                []string{"-or-create", "my-workspace"},
			wantCreateIfMissing: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseWorkspaceSelect(tc.args)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if got.CreateIfMissing != tc.wantCreateIfMissing {
				t.Errorf("CreateIfMissing = %v, want %v", got.CreateIfMissing, tc.wantCreateIfMissing)
			}
		})
	}
}

func TestParseWorkspaceSelect_vars(t *testing.T) {
	got, closer, diags := ParseWorkspaceSelect([]string{"test-workspace"})
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
