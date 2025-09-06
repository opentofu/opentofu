// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"golang.org/x/crypto/pkcs12"
)

type ClientCertificateAuthConfig struct {
	ClientCertificate         string
	ClientCertificatePassword string
	ClientCertificatePath     string
}

type clientCertAuth struct{}

var _ AuthMethod = &clientCertAuth{}

func (cred *clientCertAuth) Name() string {
	return "Client Certificate Auth"
}

func (cred *clientCertAuth) Construct(ctx context.Context, config *Config) (azcore.TokenCredential, error) {
	client := httpclient.New(ctx)

	clientCertificate, err := consolidateCertificate(config.ClientCertificate, config.ClientCertificatePath)
	if err != nil {
		// This should never happen; this is checked in the Validate function
		return nil, err
	}

	privateKey, certificate, err := pkcs12.Decode(
		clientCertificate,
		config.ClientCertificatePassword,
	)
	if err != nil {
		return nil, err
	}

	clientId, err := consolidateClientId(config)
	if err != nil {
		// This should never happen; this is checked in the Validate function
		return nil, err
	}

	return azidentity.NewClientCertificateCredential(
		config.TenantID,
		clientId,
		[]*x509.Certificate{certificate},
		privateKey,
		&azidentity.ClientCertificateCredentialOptions{
			ClientOptions: clientOptions(client, config.CloudConfig),
		},
	)
}

func (cred *clientCertAuth) Validate(_ context.Context, config *Config) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if config.TenantID == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Client Certificate Auth: missing Tenant ID",
			"Tenant ID is required",
		))
	}

	clientId, err := consolidateClientId(config)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Client Certificate Auth: error in Client ID configuration",
			fmt.Sprintf("The following error was encountered: %s", err.Error()),
		))
	}
	if clientId == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Client Certificate Auth: missing Client ID",
			"Client ID is required",
		))
	}
	clientCertificate, err := consolidateCertificate(config.ClientCertificate, config.ClientCertificatePath)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Client Certificate Auth: error in client certificate configuration",
			fmt.Sprintf("The following error was encountered: %s", err.Error()),
		))
	}
	if len(clientCertificate) == 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Client Certificate Auth: missing certificate",
			"The path to the client certificate is required",
		))
	} else {
		_, _, err := pkcs12.Decode(
			clientCertificate,
			config.ClientCertificatePassword,
		)
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Azure Client Certificate Auth: certificate credential error",
				fmt.Sprintf("The following error was encountered processing the certificate credentials: %s", err.Error()),
			))
		}
	}
	return diags
}

func (cred *clientCertAuth) AugmentConfig(_ context.Context, config *Config) error {
	return checkNamesForAccessKeyCredentials(config.StorageAddresses)
}

func consolidateCertificate(base64EncodedCertificate, certificateFilename string) ([]byte, error) {
	certBytes, err := base64.RawStdEncoding.DecodeString(base64EncodedCertificate)
	if err != nil {
		return certBytes, err
	}
	if certificateFilename == "" {
		return certBytes, nil
	}

	fileBytes, err := os.ReadFile(certificateFilename)
	if err != nil {
		return nil, fmt.Errorf("error reading client certificate file: %w", err)
	}
	if len(certBytes) != 0 && !bytes.Equal(certBytes, fileBytes) {
		return nil, errors.New("client certificate provided directly and through file do not match; either make them the same value or only provide one")
	}
	return fileBytes, nil
}
