// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cloud

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	tfe "github.com/hashicorp/go-tfe"
	version "github.com/hashicorp/go-version"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/tfdiags"
	tfversion "github.com/opentofu/opentofu/version"
	"github.com/zclconf/go-cty/cty"

	backendLocal "github.com/opentofu/opentofu/internal/backend/local"
)

func TestCloud(t *testing.T) {
	var _ backend.Enhanced = New(nil, encryption.StateEncryptionDisabled())
	var _ backend.CLI = New(nil, encryption.StateEncryptionDisabled())
}

func TestCloud_backendWithName(t *testing.T) {
	b, bCleanup := testBackendWithName(t)
	defer bCleanup()

	workspaces, err := b.Workspaces()
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(workspaces) != 1 || workspaces[0] != testBackendSingleWorkspaceName {
		t.Fatalf("should only have a single configured workspace matching the configured 'name' strategy, but got: %#v", workspaces)
	}

	if _, err := b.StateMgr("foo"); err != backend.ErrWorkspacesNotSupported {
		t.Fatalf("expected fetching a state which is NOT the single configured workspace to have an ErrWorkspacesNotSupported error, but got: %v", err)
	}

	if err := b.DeleteWorkspace(testBackendSingleWorkspaceName, true); err != backend.ErrWorkspacesNotSupported {
		t.Fatalf("expected deleting the single configured workspace name to result in an error, but got: %v", err)
	}

	if err := b.DeleteWorkspace("foo", true); err != backend.ErrWorkspacesNotSupported {
		t.Fatalf("expected deleting a workspace which is NOT the configured workspace name to result in an error, but got: %v", err)
	}
}

func TestCloud_backendWithoutHost(t *testing.T) {
	s := testServer(t)
	b := New(testDisco(s), encryption.StateEncryptionDisabled())

	obj := cty.ObjectVal(map[string]cty.Value{
		"hostname":     cty.NullVal(cty.String),
		"organization": cty.StringVal("hashicorp"),
		"token":        cty.NullVal(cty.String),
		"workspaces": cty.ObjectVal(map[string]cty.Value{
			"name":    cty.StringVal(testBackendSingleWorkspaceName),
			"tags":    cty.NullVal(cty.Set(cty.String)),
			"project": cty.NullVal(cty.String),
		}),
	})

	// Configure the backend so the client is created.
	newObj, valDiags := b.PrepareConfig(obj)
	if len(valDiags) != 0 {
		t.Fatalf("testBackend: backend.PrepareConfig() failed: %s", valDiags.ErrWithWarnings())
	}
	obj = newObj

	confDiags := b.Configure(obj)

	if !confDiags.HasErrors() {
		t.Fatalf("testBackend: backend.Configure() should have failed")
	}

	if !strings.Contains(confDiags.Err().Error(), "Hostname is required for the cloud backend") {
		t.Fatalf("testBackend: backend.Configure() should have failed with missing hostname error")
	}
}

func TestCloud_backendWithTags(t *testing.T) {
	b, bCleanup := testBackendWithTags(t)
	defer bCleanup()

	backend.TestBackendStates(t, b)

	// Test pagination works
	for i := 0; i < 25; i++ {
		_, err := b.StateMgr(fmt.Sprintf("foo-%d", i+1))
		if err != nil {
			t.Fatalf("error: %s", err)
		}
	}

	workspaces, err := b.Workspaces()
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	actual := len(workspaces)
	if actual != 26 {
		t.Errorf("expected 26 workspaces (over one standard paginated response), got %d", actual)
	}
}

func TestCloud_PrepareConfig(t *testing.T) {
	cases := map[string]struct {
		config      cty.Value
		expectedErr string
	}{
		"null organization": {
			config: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.StringVal("prod"),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			expectedErr: `Invalid or missing required argument: "organization" must be set in the cloud configuration or as an environment variable: TF_CLOUD_ORGANIZATION.`,
		},
		"null workspace": {
			config: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.StringVal("org"),
				"workspaces":   cty.NullVal(cty.String),
			}),
			expectedErr: `Invalid workspaces configuration: Missing workspace mapping strategy. Either workspace "tags" or "name" is required.`,
		},
		"workspace: empty tags, name": {
			config: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.StringVal("org"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.NullVal(cty.String),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			expectedErr: `Invalid workspaces configuration: Missing workspace mapping strategy. Either workspace "tags" or "name" is required.`,
		},
		"workspace: name present": {
			config: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.StringVal("org"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.StringVal("prod"),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			expectedErr: `Invalid workspaces configuration: Only one of workspace "tags" or "name" is allowed.`,
		},
		"workspace: name and tags present": {
			config: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.StringVal("org"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("prod"),
					"tags": cty.SetVal(
						[]cty.Value{
							cty.StringVal("billing"),
						},
					),
					"project": cty.NullVal(cty.String),
				}),
			}),
			expectedErr: `Invalid workspaces configuration: Only one of workspace "tags" or "name" is allowed.`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := testServer(t)
			b := New(testDisco(s), encryption.StateEncryptionDisabled())

			// Validate
			_, valDiags := b.PrepareConfig(tc.config)
			if valDiags.Err() != nil && tc.expectedErr != "" {
				actualErr := valDiags.Err().Error()
				if !strings.Contains(actualErr, tc.expectedErr) {
					t.Fatalf("%s: unexpected validation result: %v", name, valDiags.Err())
				}
			}
		})
	}
}

func TestCloud_PrepareConfigWithEnvVars(t *testing.T) {
	cases := map[string]struct {
		config      cty.Value
		vars        map[string]string
		expectedErr string
	}{
		"with no organization": {
			config: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.StringVal("prod"),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			vars: map[string]string{
				"TF_CLOUD_ORGANIZATION": "example-org",
			},
		},
		"with no organization attribute or env var": {
			config: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.StringVal("prod"),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			vars:        map[string]string{},
			expectedErr: `Invalid or missing required argument: "organization" must be set in the cloud configuration or as an environment variable: TF_CLOUD_ORGANIZATION.`,
		},
		"null workspace": {
			config: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.StringVal("hashicorp"),
				"workspaces":   cty.NullVal(cty.String),
			}),
			vars: map[string]string{
				"TF_WORKSPACE": "my-workspace",
			},
		},
		"organization and workspace and project env var": {
			config: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.NullVal(cty.String),
				"workspaces":   cty.NullVal(cty.String),
			}),
			vars: map[string]string{
				"TF_CLOUD_ORGANIZATION": "hashicorp",
				"TF_WORKSPACE":          "my-workspace",
				"TF_CLOUD_PROJECT":      "example-project",
			},
		},
		"with no project": {
			config: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.StringVal("organization"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.StringVal("prod"),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
		},
		"with null project": {
			config: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.StringVal("organization"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.StringVal("prod"),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			vars: map[string]string{
				"TF_CLOUD_PROJECT": "example-project",
			},
		},
		"with project env var overwrite config value": {
			config: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.StringVal("organization"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.StringVal("prod"),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.StringVal("project-name"),
				}),
			}),
			vars: map[string]string{
				"TF_CLOUD_PROJECT": "example-project",
			},
		},
		"with workspace defined by tags overwritten by TF_WORKSPACE": {
			// see https://github.com/opentofu/opentofu/issues/814 for context
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.StringVal("foo"),
				"organization": cty.StringVal("bar"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.NullVal(cty.String),
					"project": cty.NullVal(cty.String),
					"tags":    cty.SetVal([]cty.Value{cty.StringVal("baz"), cty.StringVal("qux")}),
				}),
			}),
			vars: map[string]string{
				"TF_WORKSPACE": "qux",
			},
		},
		"with TF_WORKSPACE value outside of the tags set": {
			// see https://github.com/opentofu/opentofu/issues/814 for context
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.StringVal("foo"),
				"organization": cty.StringVal("bar"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.NullVal(cty.String),
					"project": cty.NullVal(cty.String),
					"tags":    cty.SetVal([]cty.Value{cty.StringVal("baz"), cty.StringVal("qux")}),
				}),
			}),
			vars: map[string]string{
				"TF_WORKSPACE": "quxx",
			},
			expectedErr: `Invalid workspaces configuration: The workspace defined using the environment variable "TF_WORKSPACE" does not belong to "tags".`,
		},
		"with workspace block w/o attributes, TF_WORKSPACE defined": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.StringVal("foo"),
				"organization": cty.StringVal("bar"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.NullVal(cty.String),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			vars: map[string]string{
				"TF_WORKSPACE": "qux",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := testServer(t)
			b := New(testDisco(s), encryption.StateEncryptionDisabled())

			for k, v := range tc.vars {
				t.Setenv(k, v)
			}

			_, valDiags := b.PrepareConfig(tc.config)
			if (valDiags.Err() == nil) != (tc.expectedErr == "") {
				t.Fatalf("%s: unexpected validation result: %v", name, valDiags.Err())
			}
			if valDiags.Err() != nil {
				if !strings.Contains(valDiags.Err().Error(), tc.expectedErr) {
					t.Fatalf("%s: unexpected validation result: %v", name, valDiags.Err())
				}
			}
		})
	}
}

func TestCloud_config(t *testing.T) {
	cases := map[string]struct {
		config  cty.Value
		confErr string
		valErr  string
		envVars map[string]string
	}{
		"with_a_non_tfe_host": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.StringVal("nontfe.local"),
				"organization": cty.StringVal("hashicorp"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.StringVal("prod"),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			confErr: "Host nontfe.local does not provide a tfe service",
		},
		// localhost advertises TFE services, but has no token in the credentials
		"without_a_token": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.StringVal("localhost"),
				"organization": cty.StringVal("hashicorp"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.StringVal("prod"),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			confErr: "tofu login localhost",
		},
		"with_tags": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.NullVal(cty.String),
				"organization": cty.StringVal("hashicorp"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name": cty.NullVal(cty.String),
					"tags": cty.SetVal(
						[]cty.Value{
							cty.StringVal("billing"),
						},
					),
					"project": cty.NullVal(cty.String),
				}),
			}),
		},
		"with_a_name": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.NullVal(cty.String),
				"organization": cty.StringVal("hashicorp"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.StringVal("prod"),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
		},
		"without_a_name_tags": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.NullVal(cty.String),
				"organization": cty.StringVal("hashicorp"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.NullVal(cty.String),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			valErr: `Missing workspace mapping strategy.`,
		},
		"with_both_a_name_and_tags": {
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.NullVal(cty.String),
				"organization": cty.StringVal("hashicorp"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("prod"),
					"tags": cty.SetVal(
						[]cty.Value{
							cty.StringVal("billing"),
						},
					),
					"project": cty.NullVal(cty.String),
				}),
			}),
			valErr: `Only one of workspace "tags" or "name" is allowed.`,
		},
		"null config": {
			config: cty.NullVal(cty.EmptyObject),
		},
		"with_tags_and_TF_WORKSPACE_env_var_not_matching_tags": { //TODO: once we have proper e2e backend testing we should also add the opposite test - with_tags_and_TF_WORKSPACE_env_var_matching_tags
			config: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.NullVal(cty.String),
				"organization": cty.StringVal("opentofu"),
				"token":        cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"tags": cty.SetVal(
						[]cty.Value{
							cty.StringVal("billing"),
						},
					),
					"project": cty.NullVal(cty.String),
				}),
			}),
			envVars: map[string]string{
				"TF_WORKSPACE": "my-workspace",
			},
			confErr: `OpenTofu failed to find workspace my-workspace with the tags specified in your configuration`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}

			b, cleanup := testUnconfiguredBackend(t)
			t.Cleanup(cleanup)

			// Validate
			_, valDiags := b.PrepareConfig(tc.config)
			if (valDiags.Err() != nil || tc.valErr != "") &&
				(valDiags.Err() == nil || !strings.Contains(valDiags.Err().Error(), tc.valErr)) {
				t.Fatalf("unexpected validation result: %v", valDiags.Err())
			}

			// Configure
			confDiags := b.Configure(tc.config)
			if (confDiags.Err() != nil || tc.confErr != "") &&
				(confDiags.Err() == nil || !strings.Contains(confDiags.Err().Error(), tc.confErr)) {
				t.Fatalf("unexpected configure result: %v", confDiags.Err())
			}
		})
	}
}

func TestCloud_configVerifyMinimumTFEVersion(t *testing.T) {
	config := cty.ObjectVal(map[string]cty.Value{
		"hostname":     cty.StringVal(tfeHost),
		"organization": cty.StringVal("hashicorp"),
		"token":        cty.NullVal(cty.String),
		"workspaces": cty.ObjectVal(map[string]cty.Value{
			"name": cty.NullVal(cty.String),
			"tags": cty.SetVal(
				[]cty.Value{
					cty.StringVal("billing"),
				},
			),
			"project": cty.NullVal(cty.String),
		}),
	})

	handlers := map[string]func(http.ResponseWriter, *http.Request){
		"/api/v2/ping": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("TFP-API-Version", "2.4")
		},
	}
	s := testServerWithHandlers(handlers)

	b := New(testDisco(s), encryption.StateEncryptionDisabled())

	confDiags := b.Configure(config)
	if confDiags.Err() == nil {
		t.Fatalf("expected configure to error")
	}

	expected := `The 'cloud' option is not supported with this version of the cloud backend.`
	if !strings.Contains(confDiags.Err().Error(), expected) {
		t.Fatalf("expected configure to error with %q, got %q", expected, confDiags.Err().Error())
	}
}

func TestCloud_configVerifyMinimumTFEVersionInAutomation(t *testing.T) {
	config := cty.ObjectVal(map[string]cty.Value{
		"hostname":     cty.StringVal(tfeHost),
		"organization": cty.StringVal("hashicorp"),
		"token":        cty.NullVal(cty.String),
		"workspaces": cty.ObjectVal(map[string]cty.Value{
			"name": cty.NullVal(cty.String),
			"tags": cty.SetVal(
				[]cty.Value{
					cty.StringVal("billing"),
				},
			),
			"project": cty.NullVal(cty.String),
		}),
	})

	handlers := map[string]func(http.ResponseWriter, *http.Request){
		"/api/v2/ping": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("TFP-API-Version", "2.4")
		},
	}
	s := testServerWithHandlers(handlers)

	b := New(testDisco(s), encryption.StateEncryptionDisabled())
	b.runningInAutomation = true

	confDiags := b.Configure(config)
	if confDiags.Err() == nil {
		t.Fatalf("expected configure to error")
	}

	expected := `This version of cloud backend does not support the state mechanism
attempting to be used by the platform. This should never happen.`
	if !strings.Contains(confDiags.Err().Error(), expected) {
		t.Fatalf("expected configure to error with %q, got %q", expected, confDiags.Err().Error())
	}
}

func TestCloud_setUnavailableTerraformVersion(t *testing.T) {
	// go-tfe returns an error IRL if you try to set a Terraform version that's
	// not available in your TFC instance. To test this, tfe_client_mock errors if
	// you try to set any Terraform version for this specific workspace name.
	workspaceName := "unavailable-terraform-version"

	config := cty.ObjectVal(map[string]cty.Value{
		"hostname":     cty.StringVal(tfeHost),
		"organization": cty.StringVal("hashicorp"),
		"token":        cty.NullVal(cty.String),
		"workspaces": cty.ObjectVal(map[string]cty.Value{
			"name": cty.NullVal(cty.String),
			"tags": cty.SetVal(
				[]cty.Value{
					cty.StringVal("sometag"),
				},
			),
			"project": cty.NullVal(cty.String),
		}),
	})

	b, _, bCleanup := testBackend(t, config, nil)
	defer bCleanup()

	// Make sure the workspace doesn't exist yet -- otherwise, we can't test what
	// happens when a workspace gets created. This is why we can't use "name" in
	// the backend config above, btw: if you do, testBackend() creates the default
	// workspace before we get a chance to do anything.
	_, err := b.client.Workspaces.Read(context.Background(), b.organization, workspaceName)
	if err != tfe.ErrResourceNotFound {
		t.Fatalf("the workspace we were about to try and create (%s/%s) already exists in the mocks somehow, so this test isn't trustworthy anymore", b.organization, workspaceName)
	}

	_, err = b.StateMgr(workspaceName)
	if err != nil {
		t.Fatalf("expected no error from StateMgr, despite not being able to set remote TF version: %#v", err)
	}
	// Make sure the workspace was created:
	workspace, err := b.client.Workspaces.Read(context.Background(), b.organization, workspaceName)
	if err != nil {
		t.Fatalf("b.StateMgr() didn't actually create the desired workspace")
	}
	// Make sure our mocks still error as expected, using the same update function b.StateMgr() would call:
	_, err = b.client.Workspaces.UpdateByID(
		context.Background(),
		workspace.ID,
		tfe.WorkspaceUpdateOptions{TerraformVersion: tfe.String("1.1.0")},
	)
	if err == nil {
		t.Fatalf("the mocks aren't emulating a nonexistent remote TF version correctly, so this test isn't trustworthy anymore")
	}
}

func TestCloud_setConfigurationFieldsHappyPath(t *testing.T) {
	cases := map[string]struct {
		obj                   cty.Value
		envVars               map[string]string
		expectedHostname      string
		expectedOrganization  string
		expectedWorkspaceName string
		expectedProjectName   string
		expectedWorkspaceTags map[string]struct{}
		expectedForceLocal    bool
	}{
		"with hostname, organization and tags set": {
			obj: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.StringVal("opentofu"),
				"hostname":     cty.StringVal("opentofu.org"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.NullVal(cty.String),
					"tags":    cty.SetVal([]cty.Value{cty.StringVal("foo"), cty.StringVal("bar")}),
					"project": cty.NullVal(cty.String),
				}),
			}),
			expectedHostname:      "opentofu.org",
			expectedOrganization:  "opentofu",
			expectedWorkspaceTags: map[string]struct{}{"foo": {}, "bar": {}},
		},
		"with hostname and workspace name set": {
			obj: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.NullVal(cty.String),
				"hostname":     cty.StringVal("opentofu.org"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.StringVal("prod"),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			expectedHostname:      "opentofu.org",
			expectedWorkspaceName: "prod",
		},
		"with hostname and project name set": {
			obj: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.NullVal(cty.String),
				"hostname":     cty.StringVal("opentofu.org"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.NullVal(cty.String),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.StringVal("my-project"),
				}),
			}),
			expectedHostname:    "opentofu.org",
			expectedProjectName: "my-project",
		},
		"with hostname and force local set (env var)": {
			obj: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.NullVal(cty.String),
				"hostname":     cty.StringVal("opentofu.org"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.NullVal(cty.String),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			expectedHostname: "opentofu.org",
			envVars: map[string]string{
				"TF_FORCE_LOCAL_BACKEND": "1",
			},
			expectedForceLocal: true,
		},
		"with hostname and workspace tags set, then tags should not be overwritten by TF_WORKSPACE": {
			// see: https://github.com/opentofu/opentofu/issues/814
			obj: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.NullVal(cty.String),
				"hostname":     cty.StringVal("opentofu.org"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.NullVal(cty.String),
					"tags":    cty.SetVal([]cty.Value{cty.StringVal("foo"), cty.StringVal("bar")}),
					"project": cty.NullVal(cty.String),
				}),
			}),
			envVars: map[string]string{
				"TF_WORKSPACE": "foo",
			},
			expectedHostname:      "opentofu.org",
			expectedWorkspaceName: "",
			expectedWorkspaceTags: map[string]struct{}{"foo": {}, "bar": {}},
		},
		"with hostname and workspace name set, and workspace name the same as provided TF_WORKSPACE": {
			obj: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.NullVal(cty.String),
				"hostname":     cty.StringVal("opentofu.org"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.StringVal("my-workspace"),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			envVars: map[string]string{
				"TF_WORKSPACE": "my-workspace",
			},
			expectedHostname:      "opentofu.org",
			expectedWorkspaceName: "my-workspace",
		},
		"with hostname and project set, and project overwritten by TF_CLOUD_PROJECT": {
			obj: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.NullVal(cty.String),
				"hostname":     cty.StringVal("opentofu.org"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.NullVal(cty.String),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.StringVal("old"),
				}),
			}),
			envVars: map[string]string{
				"TF_CLOUD_PROJECT": "new",
			},
			expectedHostname:    "opentofu.org",
			expectedProjectName: "old",
		},
		"with hostname set, and project specified by TF_CLOUD_PROJECT": {
			obj: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.NullVal(cty.String),
				"hostname":     cty.StringVal("opentofu.org"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.NullVal(cty.String),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			envVars: map[string]string{
				"TF_CLOUD_PROJECT": "new",
			},
			expectedHostname:    "opentofu.org",
			expectedProjectName: "new",
		},
		"with hostname set, and organization specified by TF_CLOUD_ORGANIZATION": {
			obj: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.StringVal("opentofu.org"),
				"token":        cty.NullVal(cty.String),
				"organization": cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.NullVal(cty.String),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			envVars: map[string]string{
				"TF_CLOUD_ORGANIZATION": "my-org",
			},
			expectedHostname:     "opentofu.org",
			expectedOrganization: "my-org",
		},
		"with hostname set, and TF_CLOUD_HOSTNAME defined": {
			obj: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.StringVal("opentofu.org"),
				"token":        cty.NullVal(cty.String),
				"organization": cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.NullVal(cty.String),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			envVars: map[string]string{
				"TF_CLOUD_HOSTNAME": "new",
			},
			expectedHostname: "opentofu.org",
		},
		"with hostname specified by TF_CLOUD_HOSTNAME": {
			obj: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.NullVal(cty.String),
				"token":        cty.NullVal(cty.String),
				"organization": cty.NullVal(cty.String),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.NullVal(cty.String),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			envVars: map[string]string{
				"TF_CLOUD_HOSTNAME": "new",
			},
			expectedHostname: "new",
		},
		"with nothing set, all configured using env vars": {
			obj: cty.ObjectVal(map[string]cty.Value{
				"hostname":     cty.NullVal(cty.String),
				"organization": cty.NullVal(cty.String),
				"workspaces":   cty.NullVal(cty.String),
			}),
			envVars: map[string]string{
				"TF_CLOUD_HOSTNAME":     "opentofu.org",
				"TF_CLOUD_ORGANIZATION": "opentofu",
				"TF_WORKSPACE":          "foo",
				"TF_CLOUD_PROJECT":      "bar",
			},
			expectedHostname:      "opentofu.org",
			expectedOrganization:  "opentofu",
			expectedWorkspaceName: "foo",
			expectedProjectName:   "bar",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}

			b := &Cloud{}
			errDiags := b.setConfigurationFields(tc.obj)

			if errDiags.HasErrors() {
				t.Fatalf("%s: unexpected validation result: %v", name, errDiags.Err())
			}
			if b.hostname != tc.expectedHostname {
				t.Fatalf("%s: expected hostname %s to match configured hostname %s", name, b.hostname, tc.expectedHostname)
			}
			if b.organization != tc.expectedOrganization {
				t.Fatalf("%s: expected organization (%s) to match configured organization (%s)", name, b.organization, tc.expectedOrganization)
			}
			if b.WorkspaceMapping.Name != tc.expectedWorkspaceName {
				t.Fatalf("%s: expected workspace name mapping (%s) to match configured workspace name (%s)", name, b.WorkspaceMapping.Name, tc.expectedWorkspaceName)
			}
			if b.forceLocal != tc.expectedForceLocal {
				t.Fatalf("%s: expected force local backend to be set to %v", name, tc.expectedForceLocal)
			}
			if b.WorkspaceMapping.Project != tc.expectedProjectName {
				t.Fatalf("%s: expected project name mapping (%s) to match configured project name (%s)", name, b.WorkspaceMapping.Project, tc.expectedProjectName)
			}

			// read map of configured tags
			gotTags := map[string]struct{}{}
			for _, v := range b.WorkspaceMapping.Tags {
				gotTags[v] = struct{}{}
			}

			if len(gotTags) != len(tc.expectedWorkspaceTags) {
				t.Fatalf("%s: unordered workspace tags (%v) don't match configuration (%v)", name, gotTags, tc.expectedWorkspaceTags)
			}

			for k := range tc.expectedWorkspaceTags {
				if _, ok := gotTags[k]; !ok {
					t.Fatalf("%s: unordered workspace tags (%v) don't match configuration (%v)", name, gotTags, tc.expectedWorkspaceTags)
				}
			}
		})
	}
}

func TestCloud_setConfigurationFieldsUnhappyPath(t *testing.T) {
	cases := map[string]struct {
		obj         cty.Value
		envVars     map[string]string
		wantSummary string
		wantDetail  string
	}{
		"cloud block is not configured": {
			obj: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.NullVal(cty.String),
				"hostname":     cty.NullVal(cty.String),
				"workspaces":   cty.NullVal(cty.String),
			}),
			wantSummary: "Hostname is required for the cloud backend",
			wantDetail:  `OpenTofu does not provide a default "hostname" attribute, so it must be set to the hostname of the cloud backend.`,
		},
		"with hostname and workspace name set, and workspace name is not the same as provided TF_WORKSPACE": {
			obj: cty.ObjectVal(map[string]cty.Value{
				"organization": cty.NullVal(cty.String),
				"hostname":     cty.StringVal("opentofu.org"),
				"workspaces": cty.ObjectVal(map[string]cty.Value{
					"name":    cty.StringVal("my-workspace"),
					"tags":    cty.NullVal(cty.Set(cty.String)),
					"project": cty.NullVal(cty.String),
				}),
			}),
			envVars: map[string]string{
				"TF_WORKSPACE": "qux",
			},
			wantSummary: invalidWorkspaceConfigInconsistentNameAndEnvVar().Description().Summary,
			wantDetail:  invalidWorkspaceConfigInconsistentNameAndEnvVar().Description().Detail,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}

			b := &Cloud{}
			errDiags := b.setConfigurationFields(tc.obj)
			if (tc.wantDetail != "" || tc.wantSummary != "") != errDiags.HasErrors() {
				t.Fatalf("%s error expected", name)
			}

			gotSummary := errDiags[0].Description().Summary
			if gotSummary != tc.wantSummary {
				t.Fatalf("%s diagnostic summary mismatch, want: %s, got: %s", name, tc.wantSummary, gotSummary)
			}

			gotDetail := errDiags[0].Description().Detail
			if gotDetail != tc.wantDetail {
				t.Fatalf("%s diagnostic details mismatch, want: %s, got: %s", name, tc.wantDetail, gotDetail)
			}
		})
	}
}

func TestCloud_localBackend(t *testing.T) {
	b, bCleanup := testBackendWithName(t)
	defer bCleanup()

	local, ok := b.local.(*backendLocal.Local)
	if !ok {
		t.Fatalf("expected b.local to be \"*local.Local\", got: %T", b.local)
	}

	cloud, ok := local.Backend.(*Cloud)
	if !ok {
		t.Fatalf("expected local.Backend to be *cloud.Cloud, got: %T", cloud)
	}
}

func TestCloud_addAndRemoveWorkspacesDefault(t *testing.T) {
	b, bCleanup := testBackendWithName(t)
	defer bCleanup()

	if _, err := b.StateMgr(testBackendSingleWorkspaceName); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if err := b.DeleteWorkspace(testBackendSingleWorkspaceName, true); err != backend.ErrWorkspacesNotSupported {
		t.Fatalf("expected error %v, got %v", backend.ErrWorkspacesNotSupported, err)
	}
}

func TestCloud_StateMgr_versionCheck(t *testing.T) {
	b, bCleanup := testBackendWithName(t)
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

	// Update the mock remote workspace Terraform version to match the local
	// Terraform version
	if _, err := b.client.Workspaces.Update(
		context.Background(),
		b.organization,
		b.WorkspaceMapping.Name,
		tfe.WorkspaceUpdateOptions{
			TerraformVersion: tfe.String(v0140.String()),
		},
	); err != nil {
		t.Fatalf("error: %v", err)
	}

	// This should succeed
	if _, err := b.StateMgr(testBackendSingleWorkspaceName); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Now change the remote workspace to a different Terraform version
	if _, err := b.client.Workspaces.Update(
		context.Background(),
		b.organization,
		b.WorkspaceMapping.Name,
		tfe.WorkspaceUpdateOptions{
			TerraformVersion: tfe.String(v0135.String()),
		},
	); err != nil {
		t.Fatalf("error: %v", err)
	}

	// This should fail
	want := `Remote workspace TF version "0.13.5" does not match local OpenTofu version "0.14.0"`
	if _, err := b.StateMgr(testBackendSingleWorkspaceName); err.Error() != want {
		t.Fatalf("wrong error\n got: %v\nwant: %v", err.Error(), want)
	}
}

func TestCloud_StateMgr_versionCheckLatest(t *testing.T) {
	b, bCleanup := testBackendWithName(t)
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
		b.WorkspaceMapping.Name,
		tfe.WorkspaceUpdateOptions{
			TerraformVersion: tfe.String("latest"),
		},
	); err != nil {
		t.Fatalf("error: %v", err)
	}

	// This should succeed despite not being a string match
	if _, err := b.StateMgr(testBackendSingleWorkspaceName); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCloud_VerifyWorkspaceTerraformVersion(t *testing.T) {
	testCases := []struct {
		local         string
		remote        string
		executionMode string
		wantErr       bool
	}{
		{"0.13.5", "0.13.5", "agent", false},
		{"0.14.0", "0.13.5", "remote", true},
		{"0.14.0", "0.13.5", "local", false},
		{"0.14.0", "0.14.1", "remote", false},
		{"0.14.0", "1.0.99", "remote", false},
		{"0.14.0", "1.1.0", "remote", false},
		{"0.14.0", "1.3.0", "remote", true},
		{"1.2.0", "1.2.99", "remote", false},
		{"1.2.0", "1.3.0", "remote", true},
		{"0.15.0", "latest", "remote", false},
		{"1.1.5", "~> 1.1.1", "remote", false},
		{"1.1.5", "> 1.1.0, < 1.3.0", "remote", false},
		{"1.1.5", "~> 1.0.1", "remote", true},
		// pre-release versions are comparable within their pre-release stage (dev,
		// alpha, beta), but not comparable to different stages and not comparable
		// to final releases.
		{"1.1.0-beta1", "1.1.0-beta1", "remote", false},
		{"1.1.0-beta1", "~> 1.1.0-beta", "remote", false},
		{"1.1.0", "~> 1.1.0-beta", "remote", true},
		{"1.1.0-beta1", "~> 1.1.0-dev", "remote", true},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("local %s, remote %s", tc.local, tc.remote), func(t *testing.T) {
			b, bCleanup := testBackendWithName(t)
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

			// Update the mock remote workspace Terraform version to the
			// specified remote version
			if _, err := b.client.Workspaces.Update(
				context.Background(),
				b.organization,
				b.WorkspaceMapping.Name,
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
				if got := diags.Err().Error(); !strings.Contains(got, "Incompatible TF version") {
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

func TestCloud_VerifyWorkspaceTerraformVersion_workspaceErrors(t *testing.T) {
	b, bCleanup := testBackendWithName(t)
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

	// Update the mock remote workspace Terraform version to an invalid version
	if _, err := b.client.Workspaces.Update(
		context.Background(),
		b.organization,
		b.WorkspaceMapping.Name,
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
	if got := diags.Err().Error(); !strings.Contains(got, "Incompatible TF version: The remote workspace specified") {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestCloud_VerifyWorkspaceTerraformVersion_ignoreFlagSet(t *testing.T) {
	b, bCleanup := testBackendWithName(t)
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

	// Update the mock remote workspace Terraform version to the
	// specified remote version
	if _, err := b.client.Workspaces.Update(
		context.Background(),
		b.organization,
		b.WorkspaceMapping.Name,
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
	if got, want := diags[0].Description().Summary, "Incompatible TF version"; got != want {
		t.Errorf("wrong summary: got %s, want %s", got, want)
	}
	wantDetail := "The local OpenTofu version (0.14.0) does not meet the version requirements for remote workspace hashicorp/app-prod (0.13.5)."
	if got := diags[0].Description().Detail; got != wantDetail {
		t.Errorf("wrong summary: got %s, want %s", got, wantDetail)
	}
}

func TestCloudBackend_DeleteWorkspace_SafeAndForce(t *testing.T) {
	b, bCleanup := testBackendWithTags(t)
	defer bCleanup()
	safeDeleteWorkspaceName := "safe-delete-workspace"
	forceDeleteWorkspaceName := "force-delete-workspace"

	_, err := b.StateMgr(safeDeleteWorkspaceName)
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	_, err = b.StateMgr(forceDeleteWorkspaceName)
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	// sanity check that the mock now contains two workspaces
	wl, err := b.Workspaces()
	if err != nil {
		t.Fatalf("error fetching workspace names: %v", err)
	}
	if len(wl) != 2 {
		t.Fatalf("expected 2 workspaced but got %d", len(wl))
	}

	c := context.Background()
	safeDeleteWorkspace, err := b.client.Workspaces.Read(c, b.organization, safeDeleteWorkspaceName)
	if err != nil {
		t.Fatalf("error fetching workspace: %v", err)
	}

	// Lock a workspace so that it should fail to be safe deleted
	_, err = b.client.Workspaces.Lock(context.Background(), safeDeleteWorkspace.ID, tfe.WorkspaceLockOptions{Reason: tfe.String("test")})
	if err != nil {
		t.Fatalf("error locking workspace: %v", err)
	}
	err = b.DeleteWorkspace(safeDeleteWorkspaceName, false)
	if err == nil {
		t.Fatalf("workspace should have failed to safe delete")
	}

	// unlock the workspace and confirm that safe-delete now works
	_, err = b.client.Workspaces.Unlock(context.Background(), safeDeleteWorkspace.ID)
	if err != nil {
		t.Fatalf("error unlocking workspace: %v", err)
	}
	err = b.DeleteWorkspace(safeDeleteWorkspaceName, false)
	if err != nil {
		t.Fatalf("error safe deleting workspace: %v", err)
	}

	// lock a workspace and then confirm that force deleting it works
	forceDeleteWorkspace, err := b.client.Workspaces.Read(c, b.organization, forceDeleteWorkspaceName)
	if err != nil {
		t.Fatalf("error fetching workspace: %v", err)
	}
	_, err = b.client.Workspaces.Lock(context.Background(), forceDeleteWorkspace.ID, tfe.WorkspaceLockOptions{Reason: tfe.String("test")})
	if err != nil {
		t.Fatalf("error locking workspace: %v", err)
	}
	err = b.DeleteWorkspace(forceDeleteWorkspaceName, true)
	if err != nil {
		t.Fatalf("error force deleting workspace: %v", err)
	}
}

func TestCloudBackend_DeleteWorkspace_DoesNotExist(t *testing.T) {
	b, bCleanup := testBackendWithTags(t)
	defer bCleanup()

	err := b.DeleteWorkspace("non-existent-workspace", false)
	if err != nil {
		t.Fatalf("expected deleting a workspace which does not exist to succeed")
	}
}

func TestCloud_ServiceDiscoveryAliases(t *testing.T) {
	s := testServer(t)
	b := New(testDisco(s), encryption.StateEncryptionDisabled())

	diag := b.Configure(cty.ObjectVal(map[string]cty.Value{
		"hostname":     cty.StringVal(tfeHost),
		"organization": cty.StringVal("hashicorp"),
		"token":        cty.NullVal(cty.String),
		"workspaces": cty.ObjectVal(map[string]cty.Value{
			"name":    cty.StringVal("prod"),
			"tags":    cty.NullVal(cty.Set(cty.String)),
			"project": cty.NullVal(cty.String),
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
