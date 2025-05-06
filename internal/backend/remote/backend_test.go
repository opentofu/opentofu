// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package remote

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tfe "github.com/hashicorp/go-tfe"
	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-svchost/disco"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/tfdiags"
	tfversion "github.com/opentofu/opentofu/version"
	"github.com/zclconf/go-cty/cty"

	backendLocal "github.com/opentofu/opentofu/internal/backend/local"
)

func TestRemote(t *testing.T) {
	var _ backend.Enhanced = New(nil, encryption.StateEncryptionDisabled())
	var _ backend.CLI = New(nil, encryption.StateEncryptionDisabled())
}

func TestRemote_backendDefault(t *testing.T) {
	b, bCleanup := testBackendDefault(t)
	defer bCleanup()

	backend.TestBackendStates(t, b)
	backend.TestBackendStateLocks(t, b, b)
	backend.TestBackendStateForceUnlock(t, b, b)
}

func TestRemote_backendNoDefault(t *testing.T) {
	b, bCleanup := testBackendNoDefault(t)
	defer bCleanup()

	backend.TestBackendStates(t, b)
}

func TestRemote_config(t *testing.T) {
	cases := map[string]struct {
		config  cty.Value
		confErr string
		valErr  string
	}{
		"with_a_nonexisting_organization": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.StringVal(mockedBackendHost),
				"organization": cty.StringVal("nonexisting"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":   cty.StringVal("prod"),
					"prefix": cty.NullVal(cty.String),
				}),
			}),
			confErr: "organization \"nonexisting\" at host " + mockedBackendHost + " not found",
		},
		"with_a_missing_hostname": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.NullVal(cty.String),
				"organization": cty.StringVal("oracle"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":   cty.StringVal("prod"),
					"prefix": cty.NullVal(cty.String),
				}),
			}),
			confErr: `Hostname is required for the remote backend`,
		},
		"with_an_unknown_host": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.StringVal("nonexisting.local"),
				"organization": cty.StringVal("hashicorp"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":   cty.StringVal("prod"),
					"prefix": cty.NullVal(cty.String),
				}),
			}),
			confErr: "Failed to request discovery document",
		},
		// localhost advertises TFE services, but has no token in the credentials
		"without_a_token": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.StringVal("localhost"),
				"organization": cty.StringVal("hashicorp"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":   cty.StringVal("prod"),
					"prefix": cty.NullVal(cty.String),
				}),
			}),
			confErr: "tofu login localhost",
		},
		"with_a_name": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.NullVal(cty.String),
				"organization": cty.StringVal("hashicorp"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":   cty.StringVal("prod"),
					"prefix": cty.NullVal(cty.String),
				}),
			}),
		},
		"with_a_prefix": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.NullVal(cty.String),
				"organization": cty.StringVal("hashicorp"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":   cty.NullVal(cty.String),
					"prefix": cty.StringVal("my-app-"),
				}),
			}),
		},
		"without_either_a_name_and_a_prefix": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.NullVal(cty.String),
				"organization": cty.StringVal("hashicorp"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":   cty.NullVal(cty.String),
					"prefix": cty.NullVal(cty.String),
				}),
			}),
			valErr: `Either workspace "name" or "prefix" is required`,
		},
		"with_both_a_name_and_a_prefix": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.NullVal(cty.String),
				"organization": cty.StringVal("hashicorp"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":   cty.StringVal("prod"),
					"prefix": cty.StringVal("my-app-"),
				}),
			}),
			valErr: `Only one of workspace "name" or "prefix" is allowed`,
		},
		"null config": {
			config: cty.NullVal(cty.EmptyObject),
		},
	}

	for name, tc := range cases {
		s := testServer(t)
		b := New(testDisco(s), encryption.StateEncryptionDisabled())

		// Validate
		_, valDiags := b.PrepareConfig(tc.config)
		if (valDiags.Err() != nil || tc.valErr != "") &&
			(valDiags.Err() == nil || !strings.Contains(valDiags.Err().Error(), tc.valErr)) {
			t.Fatalf("%s: unexpected validation result: %v", name, valDiags.Err())
		}

		// Configure
		confDiags := b.Configure(t.Context(), tc.config)
		if (confDiags.Err() != nil || tc.confErr != "") &&
			(confDiags.Err() == nil || !strings.Contains(confDiags.Err().Error(), tc.confErr)) {
			t.Fatalf("%s: unexpected configure result: %v", name, confDiags.Err())
		}
	}
}

func TestRemote_versionConstraints(t *testing.T) {
	cases := map[string]struct {
		config     cty.Value
		prerelease string
		version    string
		result     string
	}{
		"compatible version": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.StringVal(mockedBackendHost),
				"organization": cty.StringVal("hashicorp"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":   cty.StringVal("prod"),
					"prefix": cty.NullVal(cty.String),
				}),
			}),
			version: "0.11.1",
		},
		"version too old": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.StringVal(mockedBackendHost),
				"organization": cty.StringVal("hashicorp"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":   cty.StringVal("prod"),
					"prefix": cty.NullVal(cty.String),
				}),
			}),
			version: "0.0.1",
			result:  "upgrade OpenTofu to >= 0.1.0",
		},
		"version too new": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.StringVal(mockedBackendHost),
				"organization": cty.StringVal("hashicorp"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":   cty.StringVal("prod"),
					"prefix": cty.NullVal(cty.String),
				}),
			}),
			version: "10.0.1",
			result:  "downgrade OpenTofu to <= 10.0.0",
		},
	}

	// Save and restore the actual version.
	p := tfversion.Prerelease
	v := tfversion.Version
	defer func() {
		tfversion.Prerelease = p
		tfversion.Version = v
	}()

	for name, tc := range cases {
		s := testServer(t)
		b := New(testDisco(s), encryption.StateEncryptionDisabled())

		// Set the version for this test.
		tfversion.Prerelease = tc.prerelease
		tfversion.Version = tc.version

		// Validate
		_, valDiags := b.PrepareConfig(tc.config)
		if valDiags.HasErrors() {
			t.Fatalf("%s: unexpected validation result: %v", name, valDiags.Err())
		}

		// Configure
		confDiags := b.Configure(t.Context(), tc.config)
		if (confDiags.Err() != nil || tc.result != "") &&
			(confDiags.Err() == nil || !strings.Contains(confDiags.Err().Error(), tc.result)) {
			t.Fatalf("%s: unexpected configure result: %v", name, confDiags.Err())
		}
	}
}

func TestRemote_localBackend(t *testing.T) {
	b, bCleanup := testBackendDefault(t)
	defer bCleanup()

	local, ok := b.local.(*backendLocal.Local)
	if !ok {
		t.Fatalf("expected b.local to be \"*local.Local\", got: %T", b.local)
	}

	remote, ok := local.Backend.(*Remote)
	if !ok {
		t.Fatalf("expected local.Backend to be *remote.Remote, got: %T", remote)
	}
}

func TestRemote_addAndRemoveWorkspacesDefault(t *testing.T) {
	b, bCleanup := testBackendDefault(t)
	defer bCleanup()

	if _, err := b.Workspaces(t.Context()); err != backend.ErrWorkspacesNotSupported {
		t.Fatalf("expected error %v, got %v", backend.ErrWorkspacesNotSupported, err)
	}

	if _, err := b.StateMgr(t.Context(), backend.DefaultStateName); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if _, err := b.StateMgr(t.Context(), "prod"); err != backend.ErrWorkspacesNotSupported {
		t.Fatalf("expected error %v, got %v", backend.ErrWorkspacesNotSupported, err)
	}

	if err := b.DeleteWorkspace(t.Context(), backend.DefaultStateName, true); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if err := b.DeleteWorkspace(t.Context(), "prod", true); err != backend.ErrWorkspacesNotSupported {
		t.Fatalf("expected error %v, got %v", backend.ErrWorkspacesNotSupported, err)
	}
}

func TestRemote_addAndRemoveWorkspacesNoDefault(t *testing.T) {
	b, bCleanup := testBackendNoDefault(t)
	defer bCleanup()

	states, err := b.Workspaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	expectedWorkspaces := []string(nil)
	if !reflect.DeepEqual(states, expectedWorkspaces) {
		t.Fatalf("expected states %#+v, got %#+v", expectedWorkspaces, states)
	}

	if _, err := b.StateMgr(t.Context(), backend.DefaultStateName); err != backend.ErrDefaultWorkspaceNotSupported {
		t.Fatalf("expected error %v, got %v", backend.ErrDefaultWorkspaceNotSupported, err)
	}

	expectedA := "test_A"
	if _, err := b.StateMgr(t.Context(), expectedA); err != nil {
		t.Fatal(err)
	}

	states, err = b.Workspaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	expectedWorkspaces = append(expectedWorkspaces, expectedA)
	if !reflect.DeepEqual(states, expectedWorkspaces) {
		t.Fatalf("expected %#+v, got %#+v", expectedWorkspaces, states)
	}

	expectedB := "test_B"
	if _, err := b.StateMgr(t.Context(), expectedB); err != nil {
		t.Fatal(err)
	}

	states, err = b.Workspaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	expectedWorkspaces = append(expectedWorkspaces, expectedB)
	if !reflect.DeepEqual(states, expectedWorkspaces) {
		t.Fatalf("expected %#+v, got %#+v", expectedWorkspaces, states)
	}

	if err := b.DeleteWorkspace(t.Context(), backend.DefaultStateName, true); err != backend.ErrDefaultWorkspaceNotSupported {
		t.Fatalf("expected error %v, got %v", backend.ErrDefaultWorkspaceNotSupported, err)
	}

	if err := b.DeleteWorkspace(t.Context(), expectedA, true); err != nil {
		t.Fatal(err)
	}

	states, err = b.Workspaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	expectedWorkspaces = []string{expectedB}
	if !reflect.DeepEqual(states, expectedWorkspaces) {
		t.Fatalf("expected %#+v got %#+v", expectedWorkspaces, states)
	}

	if err := b.DeleteWorkspace(t.Context(), expectedB, true); err != nil {
		t.Fatal(err)
	}

	states, err = b.Workspaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	expectedWorkspaces = []string(nil)
	if !reflect.DeepEqual(states, expectedWorkspaces) {
		t.Fatalf("expected %#+v, got %#+v", expectedWorkspaces, states)
	}
}

func TestRemote_checkConstraints(t *testing.T) {
	b, bCleanup := testBackendDefault(t)
	defer bCleanup()

	cases := map[string]struct {
		constraints *disco.Constraints
		prerelease  string
		version     string
		result      string
	}{
		"compatible version": {
			constraints: &disco.Constraints{
				Minimum: "0.11.0",
				Maximum: "0.11.11",
			},
			version: "0.11.1",
			result:  "",
		},
		"version too old": {
			constraints: &disco.Constraints{
				Minimum: "0.11.0",
				Maximum: "0.11.11",
			},
			version: "0.10.1",
			result:  "upgrade OpenTofu to >= 0.11.0",
		},
		"version too new": {
			constraints: &disco.Constraints{
				Minimum: "0.11.0",
				Maximum: "0.11.11",
			},
			version: "0.12.0",
			result:  "downgrade OpenTofu to <= 0.11.11",
		},
		"version excluded - ordered": {
			constraints: &disco.Constraints{
				Minimum:   "0.11.0",
				Excluding: []string{"0.11.7", "0.11.8"},
				Maximum:   "0.11.11",
			},
			version: "0.11.7",
			result:  "upgrade OpenTofu to > 0.11.8",
		},
		"version excluded - unordered": {
			constraints: &disco.Constraints{
				Minimum:   "0.11.0",
				Excluding: []string{"0.11.8", "0.11.6"},
				Maximum:   "0.11.11",
			},
			version: "0.11.6",
			result:  "upgrade OpenTofu to > 0.11.8",
		},
		"list versions": {
			constraints: &disco.Constraints{
				Minimum: "0.11.0",
				Maximum: "0.11.11",
			},
			version: "0.10.1",
			result:  "versions >= 0.11.0, <= 0.11.11.",
		},
		"list exclusion": {
			constraints: &disco.Constraints{
				Minimum:   "0.11.0",
				Excluding: []string{"0.11.6"},
				Maximum:   "0.11.11",
			},
			version: "0.11.6",
			result:  "excluding version 0.11.6.",
		},
		"list exclusions": {
			constraints: &disco.Constraints{
				Minimum:   "0.11.0",
				Excluding: []string{"0.11.8", "0.11.6"},
				Maximum:   "0.11.11",
			},
			version: "0.11.6",
			result:  "excluding versions 0.11.6, 0.11.8.",
		},
	}

	// Save and restore the actual version.
	p := tfversion.Prerelease
	v := tfversion.Version
	defer func() {
		tfversion.Prerelease = p
		tfversion.Version = v
	}()

	for name, tc := range cases {
		// Set the version for this test.
		tfversion.Prerelease = tc.prerelease
		tfversion.Version = tc.version

		// Check the constraints.
		diags := b.checkConstraints(tc.constraints)
		if (diags.Err() != nil || tc.result != "") &&
			(diags.Err() == nil || !strings.Contains(diags.Err().Error(), tc.result)) {
			t.Fatalf("%s: unexpected constraints result: %v", name, diags.Err())
		}
	}
}

func TestRemote_StateMgr_versionCheck(t *testing.T) {
	b, bCleanup := testBackendDefault(t)
	defer bCleanup()

	// Some fixed versions for testing with. This logic is a simple string
	// comparison, so we don't need many test cases.
	v0135 := version.Must(version.NewSemver("0.13.5"))
	v0140 := version.Must(version.NewSemver("0.14.0"))

	// Save original local version state and restore afterwards
	p := tfversion.Prerelease
	v := tfversion.Version
	s := tfversion.SemVer
	defer func() {
		tfversion.Prerelease = p
		tfversion.Version = v
		tfversion.SemVer = s
	}()

	// For this test, the local Terraform version is set to 0.14.0
	tfversion.Prerelease = ""
	tfversion.Version = v0140.String()
	tfversion.SemVer = v0140

	// Update the mock remote workspace OpenTofu version to match the local
	// Terraform version
	if _, err := b.client.Workspaces.Update(
		context.Background(),
		b.organization,
		b.workspace,
		tfe.WorkspaceUpdateOptions{
			TerraformVersion: tfe.String(v0140.String()),
		},
	); err != nil {
		t.Fatalf("error: %v", err)
	}

	// This should succeed
	if _, err := b.StateMgr(t.Context(), backend.DefaultStateName); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Now change the remote workspace to a different Terraform version
	if _, err := b.client.Workspaces.Update(
		context.Background(),
		b.organization,
		b.workspace,
		tfe.WorkspaceUpdateOptions{
			TerraformVersion: tfe.String(v0135.String()),
		},
	); err != nil {
		t.Fatalf("error: %v", err)
	}

	// This should fail
	want := `Remote workspace OpenTofu version "0.13.5" does not match local OpenTofu version "0.14.0"`
	if _, err := b.StateMgr(t.Context(), backend.DefaultStateName); err.Error() != want {
		t.Fatalf("wrong error\n got: %v\nwant: %v", err.Error(), want)
	}
}

func TestRemote_Unlock_ignoreVersion(t *testing.T) {
	b, bCleanup := testBackendDefault(t)
	defer bCleanup()

	// this is set by the unlock command
	b.IgnoreVersionConflict()

	v111 := version.Must(version.NewSemver("1.1.1"))

	// Save original local version state and restore afterwards
	p := tfversion.Prerelease
	v := tfversion.Version
	s := tfversion.SemVer
	defer func() {
		tfversion.Prerelease = p
		tfversion.Version = v
		tfversion.SemVer = s
	}()

	// For this test, the local Terraform version is set to 1.1.1
	tfversion.Prerelease = ""
	tfversion.Version = v111.String()
	tfversion.SemVer = v111

	state, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	lockID, err := state.Lock(statemgr.NewLockInfo())
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	// this should succeed since the version conflict is ignored
	if err = state.Unlock(lockID); err != nil {
		t.Fatalf("error: %v", err)
	}
}

func TestRemote_StateMgr_versionCheckLatest(t *testing.T) {
	b, bCleanup := testBackendDefault(t)
	defer bCleanup()

	v0140 := version.Must(version.NewSemver("0.14.0"))

	// Save original local version state and restore afterwards
	p := tfversion.Prerelease
	v := tfversion.Version
	s := tfversion.SemVer
	defer func() {
		tfversion.Prerelease = p
		tfversion.Version = v
		tfversion.SemVer = s
	}()

	// For this test, the local Terraform version is set to 0.14.0
	tfversion.Prerelease = ""
	tfversion.Version = v0140.String()
	tfversion.SemVer = v0140

	// Update the remote workspace to the pseudo-version "latest"
	if _, err := b.client.Workspaces.Update(
		context.Background(),
		b.organization,
		b.workspace,
		tfe.WorkspaceUpdateOptions{
			TerraformVersion: tfe.String("latest"),
		},
	); err != nil {
		t.Fatalf("error: %v", err)
	}

	// This should succeed despite not being a string match
	if _, err := b.StateMgr(t.Context(), backend.DefaultStateName); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRemote_VerifyWorkspaceTerraformVersion(t *testing.T) {
	testCases := []struct {
		local         string
		remote        string
		executionMode string
		wantErr       bool
	}{
		{"0.13.5", "0.13.5", "remote", false},
		{"0.14.0", "0.13.5", "remote", true},
		{"0.14.0", "0.13.5", "local", false},
		{"0.14.0", "0.14.1", "remote", false},
		{"0.14.0", "1.0.99", "remote", false},
		{"0.14.0", "1.1.0", "remote", false},
		{"0.14.0", "1.3.0", "remote", true},
		{"1.2.0", "1.2.99", "remote", false},
		{"1.2.0", "1.3.0", "remote", true},
		{"0.15.0", "latest", "remote", false},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("local %s, remote %s", tc.local, tc.remote), func(t *testing.T) {
			b, bCleanup := testBackendDefault(t)
			defer bCleanup()

			local := version.Must(version.NewSemver(tc.local))

			// Save original local version state and restore afterwards
			p := tfversion.Prerelease
			v := tfversion.Version
			s := tfversion.SemVer
			defer func() {
				tfversion.Prerelease = p
				tfversion.Version = v
				tfversion.SemVer = s
			}()

			// Override local version as specified
			tfversion.Prerelease = ""
			tfversion.Version = local.String()
			tfversion.SemVer = local

			// Update the mock remote workspace OpenTofu version to the
			// specified remote version
			if _, err := b.client.Workspaces.Update(
				context.Background(),
				b.organization,
				b.workspace,
				tfe.WorkspaceUpdateOptions{
					ExecutionMode:    &tc.executionMode,
					TerraformVersion: tfe.String(tc.remote),
				},
			); err != nil {
				t.Fatalf("error: %v", err)
			}

			diags := b.VerifyWorkspaceTerraformVersion(backend.DefaultStateName)
			if tc.wantErr {
				if len(diags) != 1 {
					t.Fatal("expected diag, but none returned")
				}
				if got := diags.Err().Error(); !strings.Contains(got, "OpenTofu version mismatch") {
					t.Fatalf("unexpected error: %s", got)
				}
			} else {
				if len(diags) != 0 {
					t.Fatalf("unexpected diags: %s", diags.Err())
				}
			}
		})
	}
}

func TestRemote_VerifyWorkspaceTerraformVersion_workspaceErrors(t *testing.T) {
	b, bCleanup := testBackendDefault(t)
	defer bCleanup()

	// Attempting to check the version against a workspace which doesn't exist
	// should result in no errors
	diags := b.VerifyWorkspaceTerraformVersion("invalid-workspace")
	if len(diags) != 0 {
		t.Fatalf("unexpected error: %s", diags.Err())
	}

	// Use a special workspace ID to trigger a 500 error, which should result
	// in a failed check
	diags = b.VerifyWorkspaceTerraformVersion("network-error")
	if len(diags) != 1 {
		t.Fatal("expected diag, but none returned")
	}
	if got := diags.Err().Error(); !strings.Contains(got, "Error looking up workspace: Workspace read failed") {
		t.Fatalf("unexpected error: %s", got)
	}

	// Update the mock remote workspace OpenTofu version to an invalid version
	if _, err := b.client.Workspaces.Update(
		context.Background(),
		b.organization,
		b.workspace,
		tfe.WorkspaceUpdateOptions{
			TerraformVersion: tfe.String("1.0.cheetarah"),
		},
	); err != nil {
		t.Fatalf("error: %v", err)
	}
	diags = b.VerifyWorkspaceTerraformVersion(backend.DefaultStateName)

	if len(diags) != 1 {
		t.Fatal("expected diag, but none returned")
	}
	if got := diags.Err().Error(); !strings.Contains(got, "Error looking up workspace: Invalid OpenTofu version") {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestRemote_VerifyWorkspaceTerraformVersion_ignoreFlagSet(t *testing.T) {
	b, bCleanup := testBackendDefault(t)
	defer bCleanup()

	// If the ignore flag is set, the behaviour changes
	b.IgnoreVersionConflict()

	// Different local & remote versions to cause an error
	local := version.Must(version.NewSemver("0.14.0"))
	remote := version.Must(version.NewSemver("0.13.5"))

	// Save original local version state and restore afterwards
	p := tfversion.Prerelease
	v := tfversion.Version
	s := tfversion.SemVer
	defer func() {
		tfversion.Prerelease = p
		tfversion.Version = v
		tfversion.SemVer = s
	}()

	// Override local version as specified
	tfversion.Prerelease = ""
	tfversion.Version = local.String()
	tfversion.SemVer = local

	// Update the mock remote workspace OpenTofu version to the
	// specified remote version
	if _, err := b.client.Workspaces.Update(
		context.Background(),
		b.organization,
		b.workspace,
		tfe.WorkspaceUpdateOptions{
			TerraformVersion: tfe.String(remote.String()),
		},
	); err != nil {
		t.Fatalf("error: %v", err)
	}

	diags := b.VerifyWorkspaceTerraformVersion(backend.DefaultStateName)
	if len(diags) != 1 {
		t.Fatal("expected diag, but none returned")
	}

	if got, want := diags[0].Severity(), tfdiags.Warning; got != want {
		t.Errorf("wrong severity: got %#v, want %#v", got, want)
	}
	if got, want := diags[0].Description().Summary, "OpenTofu version mismatch"; got != want {
		t.Errorf("wrong summary: got %s, want %s", got, want)
	}
	wantDetail := "The local OpenTofu version (0.14.0) does not match the configured version for remote workspace hashicorp/prod (0.13.5)."
	if got := diags[0].Description().Detail; got != wantDetail {
		t.Errorf("wrong summary: got %s, want %s", got, wantDetail)
	}
}

func TestRemote_ServiceDiscoveryAliases(t *testing.T) {
	s := testServer(t)
	b := New(testDisco(s), encryption.StateEncryptionDisabled())

	diag := b.Configure(t.Context(), cty.ObjectVal(map[string]cty.Value{
		"hostname":     cty.StringVal(mockedBackendHost),
		"organization": cty.StringVal("hashicorp"),
		"token":        cty.NullVal(cty.String),
		"workspaces": cty.ObjectVal(map[string]cty.Value{
			"name":   cty.StringVal("prod"),
			"prefix": cty.NullVal(cty.String),
		}),
	}))
	if diag.HasErrors() {
		t.Fatalf("expected no diagnostic errors, got %s", diag.Err())
	}

	aliases, err := b.ServiceDiscoveryAliases()
	if err != nil {
		t.Fatalf("expected no errors, got %s", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias but got %d", len(aliases))
	}
}
