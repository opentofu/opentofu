// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package azure_vault

import (
	"context"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/backend/remote-state/azure/auth"
)

func getParsedConfig(c **auth.Config) authMethodGetter {
	return func(ctx context.Context, authConfig *auth.Config) (auth.AuthMethod, error) {
		*c = authConfig
		return auth.GetAuthMethod(ctx, authConfig)
	}
}

// This does not test the validity of the configuration.
// It only checks that configuration variables passed in the key_provider block are recognized and used
func TestConfig_Build(t *testing.T) {
	cloudConfig, _, err := auth.CloudConfigFromAddresses(t.Context(), "public", "")
	if err != nil {
		t.Fatal(err)
	}
	testCases := map[string]struct {
		input    string
		expected *auth.Config
	}{
		"AuthConfigurationVariables": {
			input: `
				use_cli = false
				vault_uri = "https://example-keys.vault.azure.net"
				vault_key_name = "my-rsa-key"
				key_length = 32
				subscription_id = "xxxxxxxx-xxxx-xxxx-xxxx-subscriptionID"
				tenant_id = "xxxxxxxx-xxxx-xxxx-xxxx-tenantID"
				client_id = "xxxxxxxx-xxxx-xxxx-xxxx-clientID"
				client_id_file_path = "./client-id-file-path"
				client_secret = "client-secret-string"
				client_secret_file_path = "./client-secret-file-path"
				client_certificate = "client-certificate-string"
				client_certificate_password = "client-certificate-password"
				client_certificate_path = "./client-certificate-path"
				use_oidc = false
				oidc_token = "oidc_token"
				oidc_token_file_path = "./oidc-token-file-path"
				oidc_request_url = "https://oidc-request-url"
				oidc_request_token = "oidc-request-token"
				use_msi = false
				msi_endpoint = "https://msi-endpoint"
				use_aks_workload_identity = false
				`,
			expected: &auth.Config{
				AzureCLIAuthConfig: auth.AzureCLIAuthConfig{
					CLIAuthEnabled: false,
				},
				ClientSecretCredentialAuthConfig: auth.ClientSecretCredentialAuthConfig{
					ClientID:             "xxxxxxxx-xxxx-xxxx-xxxx-clientID",
					ClientIDFilePath:     "./client-id-file-path",
					ClientSecret:         "client-secret-string",
					ClientSecretFilePath: "./client-secret-file-path",
				},
				ClientCertificateAuthConfig: auth.ClientCertificateAuthConfig{
					ClientCertificate:         "client-certificate-string",
					ClientCertificatePassword: "client-certificate-password",
					ClientCertificatePath:     "./client-certificate-path",
				},
				OIDCAuthConfig: auth.OIDCAuthConfig{
					UseOIDC:           false,
					OIDCToken:         "oidc_token",
					OIDCTokenFilePath: "./oidc-token-file-path",
					OIDCRequestURL:    "https://oidc-request-url",
					OIDCRequestToken:  "oidc-request-token",
				},
				MSIAuthConfig: auth.MSIAuthConfig{
					UseMsi:   false,
					Endpoint: "https://msi-endpoint",
				},
				StorageAddresses: auth.StorageAddresses{
					CloudConfig:    cloudConfig,
					SubscriptionID: "xxxxxxxx-xxxx-xxxx-xxxx-subscriptionID",
					TenantID:       "xxxxxxxx-xxxx-xxxx-xxxx-tenantID",
				},
				WorkloadIdentityAuthConfig: auth.WorkloadIdentityAuthConfig{
					UseAKSWorkloadIdentity: false,
				},
			},
		},
	}
	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			input, diags := hclsyntax.ParseConfig([]byte(testCase.input), "test_config", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatal(diags.Error())
			}
			config := new(Config)
			diags = gohcl.DecodeBody(input.Body, nil, config)
			if diags.HasErrors() {
				t.Fatal(diags.Error())
			}
			var authConfig *auth.Config
			getAuthMethod = getParsedConfig(&authConfig)
			_, _, err := config.Build()
			if authConfig == nil && err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(testCase.expected, authConfig); diff != "" {
				t.Errorf("mismatch (-expected +received):\n%s", diff)
			}
		})
	}
}
