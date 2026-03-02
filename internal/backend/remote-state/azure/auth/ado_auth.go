// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

const (
	adoApiVersion = "7.1"
)

type ADOAuthConfig struct {
	ADOServiceConnectionId string
}

type adoAuth struct{}

var _ AuthMethod = &adoAuth{}

func (cred *adoAuth) Name() string {
	return "Azure DevOps Auth"
}

func (cred *adoAuth) Construct(ctx context.Context, config *Config) (azcore.TokenCredential, error) {
	clientId, err := consolidateClientId(config)
	if err != nil {
		return nil, err
	}

	return azidentity.NewClientAssertionCredential(
		config.TenantID,
		clientId,
		// The azure sdk calls this callback whenever it needs client assertion
		//
		// Previously, the OIDC token was fetched once and returned statically,
		// which caused failures when using short-lived tokens in azure devops
		// pipelines during long-running operations.
		//
		// By resolving the token dynamically here, we allow the sdk to obtain
		// a fresh OIDC token as needed, enabling proper token refresh behavior.
		func(ctx context.Context) (string, error) {
			client := httpclient.New(ctx)

			if config.OIDCToken == "" && config.OIDCTokenFilePath == "" {
				return getAdoTokenFromRemote(client, config)
			}
			return consolidateToken(config)
		},
		&azidentity.ClientAssertionCredentialOptions{
			ClientOptions: clientOptions(httpclient.New(ctx), config.CloudConfig),
		},
	)
}

type ADOTokenResponse struct {
	Token string `json:"oidcToken"`
}

func getAdoTokenFromRemote(client *http.Client, config *Config) (string, error) {
	// GET from the request URL, using the bearer token
	req, err := http.NewRequest(http.MethodPost, config.OIDCRequestURL, nil)
	if err != nil {
		return "", fmt.Errorf("malformed token request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+config.OIDCRequestToken)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	// Prevent redirection on invalid token
	// https://github.com/Azure/azure-sdk-for-cpp/pull/6019
	req.Header.Add("X-TFS-FedAuthRedirect", "Suppress")

	query := req.URL.Query()
	query.Set("api-version", adoApiVersion)
	query.Set("serviceConnectionId", config.ADOServiceConnectionId)
	req.URL.RawQuery = query.Encode()

	// Read the response
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error obtaining token: %w", err)
	}
	defer resp.Body.Close()
	rawToken, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("io error reading token response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("non-OK response: status code %d, body: %s", resp.StatusCode, rawToken)
	}
	var token ADOTokenResponse
	// Provide that response as the access token.
	err = json.Unmarshal(rawToken, &token)
	if err != nil {
		return "", fmt.Errorf("error parsing json of token response body: %w", err)
	}
	return token.Token, nil
}

func (cred *adoAuth) Validate(ctx context.Context, config *Config) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if !config.UseOIDC {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid Azure DevOps Auth",
			"Azure DevOps Auth is disabled when use_oidc or the environment variable ARM_USE_OIDC are unset or set explicitly to false.",
		))
		return diags
	}
	// Validate serviceConnectionId early, as it serves as the discriminator for using ADO auth.
	if config.ADOServiceConnectionId == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid Azure DevOps Auth",
			"ADO Service Connection ID is missing.",
		))
	}
	if config.TenantID == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid Azure DevOps Auth",
			"Tenant ID is missing.",
		))
	}
	_, err := consolidateClientId(config)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid Azure DevOps Auth",
			fmt.Sprintf("The Client ID is misconfigured: %s.", tfdiags.FormatError(err)),
		))
	}
	directTokenUnset := config.OIDCToken == "" && config.OIDCTokenFilePath == ""
	indirectTokenUnset := config.OIDCRequestURL == "" || config.OIDCRequestToken == ""
	if directTokenUnset && indirectTokenUnset {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid Azure DevOps Auth",
			"An access token must be provided, either directly with a variable or through a file, or indirectly through a request URL and request token (as in GitHub Actions).",
		))
	}
	if directTokenUnset {
		// check request URL and token
		_, err := getAdoTokenFromRemote(httpclient.New(ctx), config)
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid Azure DevOps Auth",
				fmt.Sprintf("Tried to test fetching the token, but received this error instead: %s.", tfdiags.FormatError(err)),
			))
		}
	}
	// This will work, even if both token and file path are empty
	if _, err := consolidateToken(config); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid Azure DevOps Auth",
			fmt.Sprintf("The token is misconfigured: %s", err.Error()),
		))
	}
	return diags
}

func (cred *adoAuth) AugmentConfig(_ context.Context, config *Config) error {
	return checkNamesForAccessKeyCredentials(config.StorageAddresses)
}
