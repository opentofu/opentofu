// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

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

func (cred *oidcAuth) Name() string {
	return "OpenID Connect Auth"
}

func (cred *oidcAuth) Construct(ctx context.Context, config *Config) (azcore.TokenCredential, error) {
	client := httpclient.New(ctx)
	var token string
	var err error
	if config.OIDCToken == "" && config.OIDCTokenFilePath == "" {
		token, err = getTokenFromRemote(client, config.OIDCAuthConfig)
		if err != nil {
			return nil, err
		}
	} else {
		token, err = consolidateToken(config.OIDCAuthConfig)
		if err != nil {
			return nil, err
		}
	}
	return azidentity.NewClientAssertionCredential(
		config.TenantID,
		config.ClientID,
		func(_ context.Context) (string, error) {
			return token, nil
		},
		&azidentity.ClientAssertionCredentialOptions{
			ClientOptions: clientOptions(client, config.CloudConfig),
		},
	)
}

type tokenResp struct {
	Count *int    `json:"count"`
	Value *string `json:"value"`
}

/*
// TODO do I need this disclaimer? In a future state, we will not be using this package in the code, so the license will not necessarily still be provided
Legal Disclaimer: the function getTokenFromRemote was copied almost wholesale from https://github.com/manicminer/hamilton, an Apache 2.0-licensed repository
*/

func getTokenFromRemote(client *http.Client, config OIDCAuthConfig) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, config.OIDCRequestURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("error forming request: %w", err)
	}
	queryParams := req.URL.Query()
	queryParams.Set("audience", "api://AzureADTokenExchange")
	req.URL.RawQuery = queryParams.Encode()

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.OIDCRequestToken))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error processing OIDC token request request: %w", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || 300 <= resp.StatusCode {
		if err != nil {
			return "", fmt.Errorf("error processing OIDC request: received status code %d", resp.StatusCode)
		}
		return "", fmt.Errorf("error processing OIDC request: %s", string(body))
	}

	if err != nil {
		return "", fmt.Errorf("error reading body of OIDC response: %w", err)
	}

	var tokenRes tokenResp

	if err := json.Unmarshal(body, &tokenRes); err != nil {
		return "", fmt.Errorf("error decoding OIDC json: %w", err)
	}

	if tokenRes.Value == nil || *tokenRes.Value == "" {
		return "", errors.New("no token value found in the OIDC response body")
	}

	return *tokenRes.Value, nil
}

func consolidateToken(config OIDCAuthConfig) (string, error) {
	token := config.OIDCToken
	if config.OIDCTokenFilePath != "" {
		// read token from file. Use as token if provided is empty, or check that they're the same
		b, err := os.ReadFile(config.OIDCTokenFilePath)
		if err != nil {
			return "", fmt.Errorf("error reading token file: %w", err)
		}
		file_token := string(b)
		if token != "" && token != file_token {
			return "", errors.New("token provided directly and through file do not match; either make them the same value or only provide one")
		}
		token = file_token
	}
	return token, nil
}

func (cred *oidcAuth) Validate(config *Config) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if !config.UseOIDC {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Use OIDC is not set",
			"In order to use OpenID Connect credentials, use_oidc or the environment variable ARM_USE_OIDC must be set to true",
		))
		return diags
	}
	if config.TenantID == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Tenant ID is empty",
			"In order to use OpenID Connect credentials, a tenant ID is necessary",
		))
	}
	if config.ClientID == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Client ID is empty",
			"In order to use OpenID Connect credentials, a client ID is necessary",
		))
	}
	directTokenUnset := config.OIDCToken == "" && config.OIDCTokenFilePath == ""
	indirectTokenUnset := config.OIDCRequestURL == "" || config.OIDCRequestToken == ""
	if directTokenUnset && indirectTokenUnset {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Both OIDC Token and OIDC Token File Path are empty, and the request url or request token are empty",
			"In order to use OpenID Connect credentials, the access token must be provided. It is either directly provided in a variable or through a file, or indirectly through a request URL and request token (as in GitHub Actions)",
		))
	}
	if directTokenUnset {
		// check request URL and token
		_, err := getTokenFromRemote(httpclient.New(context.Background()), config.OIDCAuthConfig)
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Could not get token from request URL",
				fmt.Sprintf("In order to use OpenID Connect credentials, an access token should be obtainable, but the following error was encountered while fetching the token: %s", err.Error()),
			))
		}
	}
	// This will work, even if both token and file path are empty
	if _, err := consolidateToken(config.OIDCAuthConfig); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"There was a problem reconciling tokens",
			fmt.Sprintf("In order to use OpenID Connect credentials, the access token provided must be readable and consistent, but the following error was encountered: %s", err.Error()),
		))
	}
	return diags
}

func (cred *oidcAuth) AugmentConfig(config *Config) error {
	return checkNamesForAccessKeyCredentials(config.StorageAddresses)
}
