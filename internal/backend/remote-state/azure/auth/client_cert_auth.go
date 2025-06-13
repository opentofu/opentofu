// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"context"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"golang.org/x/crypto/pkcs12"
)

type ClientCertificateAuthConfig struct {
	ClientCertificatePassword string
	ClientCertificatePath     string
}

type clientCertAuth struct{}

func (cred *clientCertAuth) Name() string {
	return "Client Certificate Auth"
}

func (cred *clientCertAuth) Construct(ctx context.Context, config *Config) (azcore.TokenCredential, error) {
	client := httpclient.New(ctx)

	privateKey, certificate, err := decodePFXCertificate(
		config.ClientCertificateAuthConfig.ClientCertificatePath,
		config.ClientCertificateAuthConfig.ClientCertificatePassword,
	)
	if err != nil {
		return nil, err
	}

	return azidentity.NewClientCertificateCredential(
		config.StorageAddresses.TenantID,
		config.ClientBasicAuthConfig.ClientID,
		[]*x509.Certificate{certificate},
		privateKey,
		&azidentity.ClientCertificateCredentialOptions{
			ClientOptions: clientOptions(client, config.CloudConfig),
		},
	)
}

func (cred *clientCertAuth) Validate(config *Config) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if config.StorageAddresses.TenantID == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Backend: Client Certificate credentials",
			"Tenant ID is required",
		))
	}
	if config.ClientBasicAuthConfig.ClientID == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Backend: Client Certificate credentials",
			"Client ID is required",
		))
	}
	if config.ClientCertificateAuthConfig.ClientCertificatePath == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Backend: Client Certificate credentials",
			"The path to the client certificate is required",
		))
	} else {
		_, _, err := decodePFXCertificate(
			config.ClientCertificateAuthConfig.ClientCertificatePath,
			config.ClientCertificateAuthConfig.ClientCertificatePassword,
		)
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Azure Backend: Client Certificate credentials",
				fmt.Sprintf("The following error was encountered processing the certificate credentials: %s", err.Error()),
			))
		}
	}
	return diags
}

func (cred *clientCertAuth) AugmentConfig(config *Config) error {
	return checkNamesForAccessKeyCredentials(config.StorageAddresses)
}

func decodePFXCertificate(pfxFileName string, password string) (privateKey interface{}, certificate *x509.Certificate, err error) {
	// read file contents, decode cert
	contents, err := os.ReadFile(pfxFileName)
	if err != nil {
		err = fmt.Errorf("problem reading file at %s: %w", pfxFileName, err)
		return
	}
	return pkcs12.Decode(contents, password)
}
