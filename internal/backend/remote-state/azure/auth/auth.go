// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"context"
	"log"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Config struct {
	AzureCLIAuthConfig
	ClientSecretCredentialAuthConfig
	ClientCertificateAuthConfig
	OIDCAuthConfig
	MSIAuthConfig
	StorageAddresses
	WorkloadIdentityAuthConfig
}

type AuthMethod interface {
	// Construct takes the configuration and obtains an Azure-native
	// authentication method, appropriate for the Azure sdk's various clients.
	Construct(ctx context.Context, config *Config) (azcore.TokenCredential, error)

	// Validate ensures this authentication method has the configuration variables and is
	// the appropriate method to use. A nil return for diagnostics implies that there is no
	// need to look further for authentication methods.
	Validate(ctx context.Context, config *Config) tfdiags.Diagnostics

	// AugmentConfig should be called to ensure the config has all proper storage names
	// when attempting to get the storage account's access keys. It will return an error if
	// the expected storage names, IDs, and addresses are not present.
	//
	// Note: only the CLI is really able to actually *change* the config, by obtaining information
	// out of the azure profile saved on the filesystem.
	AugmentConfig(ctx context.Context, config *Config) error

	// Name provides a simple english name for the auth method; used for debugging
	Name() string
}

func GetAuthMethod(ctx context.Context, config *Config) (AuthMethod, error) {
	authMethods := []AuthMethod{
		&clientCertAuth{},
		&clientSecretCredentialAuth{},
		&oidcAuth{},
		&managedIdentityAuth{},
		&workloadIdentityAuth{},
		&azureCLICredentialAuth{},
	}
	var diags tfdiags.Diagnostics
	for _, authMethod := range authMethods {
		if d := authMethod.Validate(ctx, config); d.HasErrors() {
			diags = diags.Append(d)
			continue
		}
		log.Printf("[DEBUG] Selected Azure auth method: %s", authMethod.Name())
		return authMethod, nil
	}
	diags = diags.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"No valid azure auth methods found",
		"Please see above warnings for details about what each auth method needs to properly work.",
	))
	return nil, diags.ErrWithWarnings()
}
