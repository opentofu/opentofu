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

func TestParseWorkspaceDelete_viewOptions(t *testing.T) {
	tempDir := t.TempDir()
	testCases := map[string]struct {
		args         []string
		wantViewType ViewType
	}{
		"default view type": {
			args:         []string{"old-workspace"},
			wantViewType: ViewHuman,
		},
		"json view type": {
			args:         []string{"-json", "old-workspace"},
			wantViewType: ViewJSON,
		},
		"json-into": {
			args:         []string{fmt.Sprintf("-json-into=%s", filepath.Join(tempDir, "json-into")), "old-workspace"},
			wantViewType: ViewHuman,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseWorkspaceDelete(tc.args)
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

func TestParseWorkspaceDelete_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args []string
		want *WorkspaceDelete
	}{
		"defaults": {
			[]string{"target-ws"},
			workspaceDeleteArgsWithDefaults(func(in *WorkspaceDelete) {
				in.WorkspaceName = "target-ws"
			}),
		},
		"force flag": {
			[]string{"-force", "target-ws"},
			workspaceDeleteArgsWithDefaults(func(in *WorkspaceDelete) {
				in.WorkspaceName = "target-ws"
				in.Force = true
			}),
		},
		"lock flag": {
			[]string{"-lock=false", "target-ws"},
			workspaceDeleteArgsWithDefaults(func(in *WorkspaceDelete) {
				in.WorkspaceName = "target-ws"
				in.StateLock = false
			}),
		},
		"lock timeout flag": {
			[]string{"-lock-timeout=2s", "target-ws"},
			workspaceDeleteArgsWithDefaults(func(in *WorkspaceDelete) {
				in.WorkspaceName = "target-ws"
				in.StateLockTimeout = 2 * time.Second
			}),
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Vars{}, ViewOptions{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseWorkspaceDelete(tc.args)
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

func TestParseWorkspaceDelete_vars(t *testing.T) {
	testCases := map[string]struct {
		args              []string
		expectVarsEmpty   bool
		expectDiagnostics bool
	}{
		"no vars": {
			args:              []string{"target_ws"},
			expectVarsEmpty:   true,
			expectDiagnostics: false,
		},
		"single var": {
			args:              []string{"-var=key=value", "target_ws"},
			expectVarsEmpty:   false,
			expectDiagnostics: false,
		},
		"multiple vars": {
			args:              []string{"-var=key1=value1", "-var=key2=value2", "target_ws"},
			expectVarsEmpty:   false,
			expectDiagnostics: false,
		},
		"var-file": {
			args:              []string{"-var-file=test.tfvars", "target_ws"},
			expectVarsEmpty:   false,
			expectDiagnostics: false,
		},
		"mixed vars and var-files": {
			args:              []string{"-var=key=value", "-var-file=test.tfvars", "-var=another=val", "target_ws"},
			expectVarsEmpty:   false,
			expectDiagnostics: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseWorkspaceDelete(tc.args)
			defer closer()

			if tc.expectDiagnostics && len(diags) == 0 {
				t.Fatal("expected diagnostics but got none")
			}
			if !tc.expectDiagnostics && len(diags) > 0 {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if got.Vars == nil {
				t.Fatal("Vars should never be nil")
			}

			if tc.expectVarsEmpty && !got.Vars.Empty() {
				t.Errorf("expected Vars to be empty but got vars")
			}
			if !tc.expectVarsEmpty && got.Vars.Empty() {
				t.Errorf("expected Vars to be populated but got empty")
			}
		})
	}
}

func workspaceDeleteArgsWithDefaults(mutate func(in *WorkspaceDelete)) *WorkspaceDelete {
	ret := &WorkspaceDelete{
		Force:            false,
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
