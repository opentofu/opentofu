// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"context"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type MSIAuthConfig struct {
	UseMsi   bool
	Endpoint string
}

type managedIdentityAuth struct{}

var _ AuthMethod = &managedIdentityAuth{}

func (cred *managedIdentityAuth) Name() string {
	return "Managed Service Identity Auth"
}

// msiTokenCredentialWrapper wraps the ManagedIdentityCredential with a bit of logic
// to manage the MSI_ENDPOINT environment variable. See the reconcileMSIEndpoint documentation
// for details.
type msiTokenCredentialWrapper struct {
	cred *azidentity.ManagedIdentityCredential

	Endpoint string
}

func (cred *managedIdentityAuth) Construct(ctx context.Context, config *Config) (azcore.TokenCredential, error) {
	client := httpclient.New(ctx)
	c, err := azidentity.NewManagedIdentityCredential(
		&azidentity.ManagedIdentityCredentialOptions{
			ClientOptions: clientOptions(client, config.CloudConfig),
		},
	)
	endpoint := reconcileMSIEndpoint(config.Endpoint)
	if endpoint != "" {
		return &msiTokenCredentialWrapper{
			cred:     c,
			Endpoint: endpoint,
		}, err
	}
	return c, err
}

const MSI_ENDPOINT string = "MSI_ENDPOINT"

func (credWrapper *msiTokenCredentialWrapper) GetToken(ctx context.Context, options policy.TokenRequestOptions) (token azcore.AccessToken, err error) {
	os.Setenv(MSI_ENDPOINT, credWrapper.Endpoint)
	token, err = credWrapper.cred.GetToken(ctx, options)
	os.Unsetenv(MSI_ENDPOINT)
	return
}

// reconcileMSIEndpoint helps to set MSI_ENDPOINT, if it has not already set.
// This is a bit of a hack, but we do this to ensure backwards compatibility.
// The microsoft-authentication-library-for-go uses the MSI_ENDPOINT environment variable
// to automatically set the endpoint, in the case OpenTofu is running in
// Cloud Shell or AzureML. There isn't another way to get the endpoint information
// to the library...
func reconcileMSIEndpoint(msiEndpointFromConfig string) string {
	_, ok := os.LookupEnv(MSI_ENDPOINT)
	if ok {
		return ""
	}
	return msiEndpointFromConfig
}

func (cred *managedIdentityAuth) Validate(_ context.Context, config *Config) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if !config.MSIAuthConfig.UseMsi {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Managed Service Identity Auth: use_msi set to false",
			"The Managed Service Identity (MSI) needs to have \"use_msi\" (or ARM_USE_MSI) set to true in order to be used.",
		))
	}
	return diags
}

func (cred *managedIdentityAuth) AugmentConfig(_ context.Context, config *Config) error {
	return checkNamesForAccessKeyCredentials(config.StorageAddresses)
}
