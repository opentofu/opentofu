package auth

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type ClientBasicAuthConfig struct {
	ClientID     string
	ClientSecret string
}

type clientBasicAuth struct{}

func (cred *clientBasicAuth) Construct(ctx context.Context, config *Config) (azcore.TokenCredential, error) {
	client := httpclient.New(ctx)

	return azidentity.NewClientSecretCredential(
		config.StorageAddresses.TenantID,
		config.ClientBasicAuthConfig.ClientID,
		config.ClientBasicAuthConfig.ClientSecret,
		&azidentity.ClientSecretCredentialOptions{
			ClientOptions: clientOptions(client),
		},
	)
}

func (cred *clientBasicAuth) Validate(config *Config) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if config.StorageAddresses.TenantID == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Tenant ID is empty",
			"In order to use Client Secret credentials, a tenant ID is necessary",
		))
	}
	if config.ClientBasicAuthConfig.ClientID == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Client ID is empty",
			"In order to use Client Secret credentials, a client ID is necessary",
		))
	}
	if config.ClientBasicAuthConfig.ClientSecret == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Client Secret is empty",
			"In order to use Client Secret credentials, a client secret is necessary",
		))
	}
	return diags
}

func (cred *clientBasicAuth) AugmentConfig(config *Config) error {
	return checkNamesForAccessKeyCredentials(*config.StorageAddresses)
}
