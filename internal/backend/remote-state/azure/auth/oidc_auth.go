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

type OIDCAuthConfig struct {
	UseOIDC           bool
	OIDCToken         string
	OIDCTokenFilePath string
	OIDCRequestURL    string
	OIDCRequestToken  string
}

type oidcAuth struct{}

var _ AuthMethod = &oidcAuth{}

func (cred *oidcAuth) Name() string {
	return "OpenID Connect Auth"
}

func (cred *oidcAuth) Construct(ctx context.Context, config *Config) (azcore.TokenCredential, error) {
	client := httpclient.New(ctx)

	clientId, err := consolidateClientId(config)
	if err != nil {
		// This should never happen; this is checked in the Validate function
		return nil, err
	}
	var token string
	if config.OIDCToken == "" && config.OIDCTokenFilePath == "" {
		token, err = getTokenFromRemote(client, config.OIDCAuthConfig)
		if err != nil {
			return nil, err
		}
	} else {
		token, err = consolidateToken(config)
		if err != nil {
			return nil, err
		}
	}
	return azidentity.NewClientAssertionCredential(
		config.TenantID,
		clientId,
		func(_ context.Context) (string, error) {
			return token, nil
		},
		&azidentity.ClientAssertionCredentialOptions{
			ClientOptions: clientOptions(client, config.CloudConfig),
		},
	)
}

type TokenResponse struct {
	Value string `json:"value"`
}

func getTokenFromRemote(client *http.Client, config OIDCAuthConfig) (string, error) {
	// GET from the request URL, using the bearer token
	req, err := http.NewRequest(http.MethodGet, config.OIDCRequestURL, nil)
	if err != nil {
		return "", fmt.Errorf("malformed token request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+config.OIDCRequestToken)
	req.Header.Add("Accept", "application/json; api-version=2.0")
	req.Header.Add("Content-Type", "application/json")

	query := req.URL.Query()
	query.Set("audience", "api://AzureADTokenExchange")
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("non-2xx response: status code %d, body: %s", resp.StatusCode, rawToken)
	}
	var token TokenResponse
	// Provide that response as the access token.
	err = json.Unmarshal(rawToken, &token)
	if err != nil {
		return "", fmt.Errorf("error parsing json of token response body: %w", err)
	}
	return token.Value, nil
}

func consolidateToken(config *Config) (string, error) {
	return consolidateFileAndValue(config.OIDCToken, config.OIDCTokenFilePath, "token")
}

func (cred *oidcAuth) Validate(ctx context.Context, config *Config) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if !config.UseOIDC {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure OpenID Connect Auth: use_oidc set to false",
			"use_oidc or the environment variable ARM_USE_OIDC must be set to true",
		))
		return diags
	}
	if config.TenantID == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure OpenID Connect Auth: missing Tenant ID",
			"Tenant ID is required",
		))
	}
	clientId, err := consolidateClientId(config)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure OpenID Connect Auth: error in Client ID configuration",
			fmt.Sprintf("The following error was encountered: %s", err.Error()),
		))
	}
	if clientId == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure OpenID Connect Auth: missing Client ID",
			"Client ID is required",
		))
	}
	directTokenUnset := config.OIDCToken == "" && config.OIDCTokenFilePath == ""
	indirectTokenUnset := config.OIDCRequestURL == "" || config.OIDCRequestToken == ""
	if directTokenUnset && indirectTokenUnset {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure OpenID Connect Auth: missing access token",
			"An access token must be provided, either directly with a variable or through a file, or indirectly through a request URL and request token (as in GitHub Actions)",
		))
	}
	if directTokenUnset {
		// check request URL and token
		_, err := getTokenFromRemote(httpclient.New(ctx), config.OIDCAuthConfig)
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Azure OpenID Connect Auth: error fetching token",
				fmt.Sprintf("The following error was encountered while fetching the token: %s", err.Error()),
			))
		}
	}
	// This will work, even if both token and file path are empty
	if _, err := consolidateToken(config); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure OpenID Connect Auth: error in token configuration",
			fmt.Sprintf("The following error was encountered: %s", err.Error()),
		))
	}
	return diags
}

func (cred *oidcAuth) AugmentConfig(_ context.Context, config *Config) error {
	return checkNamesForAccessKeyCredentials(config.StorageAddresses)
}
