package auth

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type MSIAuthConfig struct {
	UseMsi      bool
	MsiEndpoint string
}

type managedIdentityAuth struct{}

func (cred *managedIdentityAuth) Construct(ctx context.Context, config *Config) (azcore.TokenCredential, error) {
	client := httpclient.New(ctx)

	return azidentity.NewManagedIdentityCredential(
		&azidentity.ManagedIdentityCredentialOptions{
			ClientOptions: clientOptions(client),
		},
	)
}

func (cred *managedIdentityAuth) Validate(config *Config) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if !config.MSIAuthConfig.UseMsi {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Managed Identity Auth is not set",
			"The Managed Service Identity (MSI) needs to have \"use_msi\" (or ARM_USE_MSI) set to true in order to be used.",
		))
	}
	return diags
}

func (cred *managedIdentityAuth) AugmentConfig(config *Config) error {
	return checkNamesForAccessKeyCredentials(*config.StorageAddresses)
}
