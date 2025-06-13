// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

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

func (cred *clientBasicAuth) Name() string {
	return "Client Secret Auth"
}

func (cred *clientBasicAuth) Construct(ctx context.Context, config *Config) (azcore.TokenCredential, error) {
	client := httpclient.New(ctx)

	return azidentity.NewClientSecretCredential(
		config.StorageAddresses.TenantID,
		config.ClientBasicAuthConfig.ClientID,
		config.ClientBasicAuthConfig.ClientSecret,
		&azidentity.ClientSecretCredentialOptions{
			ClientOptions: clientOptions(client, config.CloudConfig),
		},
	)
}

func (cred *clientBasicAuth) Validate(config *Config) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if config.StorageAddresses.TenantID == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Backend: Client Secret credentials",
			"Tenant ID is required",
		))
	}
	if config.ClientBasicAuthConfig.ClientID == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Backend: Client Secret credentials",
			"Client ID is required",
		))
	}
	if config.ClientBasicAuthConfig.ClientSecret == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Backend: Client Secret credentials",
			"Client Secret is required",
		))
	}
	return diags
}

func (cred *clientBasicAuth) AugmentConfig(config *Config) error {
	return checkNamesForAccessKeyCredentials(config.StorageAddresses)
}
