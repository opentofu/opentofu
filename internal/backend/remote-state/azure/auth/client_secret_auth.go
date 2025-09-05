// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type ClientSecretCredentialAuthConfig struct {
	ClientID             string
	ClientIDFilePath     string
	ClientSecret         string
	ClientSecretFilePath string
}

type clientSecretCredentialAuth struct{}

var _ AuthMethod = &clientSecretCredentialAuth{}

func (cred *clientSecretCredentialAuth) Name() string {
	return "Client Secret Auth"
}

func (cred *clientSecretCredentialAuth) Construct(ctx context.Context, config *Config) (azcore.TokenCredential, error) {
	client := httpclient.New(ctx)
	clientId, err := consolidateClientId(config)
	if err != nil {
		// This should never happen; this is checked in the Validate function
		return nil, err
	}
	clientSecret, err := consolidateClientSecret(config)
	if err != nil {
		// This should never happen; this is checked in the Validate function
		return nil, err
	}

	return azidentity.NewClientSecretCredential(
		config.StorageAddresses.TenantID,
		clientId,
		clientSecret,
		&azidentity.ClientSecretCredentialOptions{
			ClientOptions: clientOptions(client, config.CloudConfig),
		},
	)
}

func (cred *clientSecretCredentialAuth) Validate(_ context.Context, config *Config) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if config.StorageAddresses.TenantID == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Client Secret Auth: missing Tenant ID",
			"Tenant ID is required",
		))
	}
	clientId, err := consolidateClientId(config)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Client Secret Auth: error in Client ID configuration",
			fmt.Sprintf("The following error was encountered: %s", err.Error()),
		))
	}
	if clientId == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Client Secret Auth: missing Client ID",
			"Client ID is required",
		))
	}
	clientSecret, err := consolidateClientSecret(config)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Client Secret Auth: error in Client Secret configuration",
			fmt.Sprintf("The following error was encountered: %s", err.Error()),
		))
	}
	if clientSecret == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Client Secret Auth: missing Client Secret",
			"Client Secret is required",
		))
	}
	return diags
}

func (cred *clientSecretCredentialAuth) AugmentConfig(_ context.Context, config *Config) error {
	return checkNamesForAccessKeyCredentials(config.StorageAddresses)
}

func consolidateClientId(config *Config) (string, error) {
	return consolidateFileAndValue(config.ClientID, config.ClientIDFilePath, "client ID")
}

func consolidateClientSecret(config *Config) (string, error) {
	return consolidateFileAndValue(config.ClientSecret, config.ClientSecretFilePath, "client secret")
}
