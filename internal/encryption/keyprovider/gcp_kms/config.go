// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package gcp_kms

import (
	"context"
	"encoding/json"
	"os"

	"github.com/mitchellh/go-homedir"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/version"
	"golang.org/x/oauth2"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"

	kms "cloud.google.com/go/kms/apiv1"
)

type keyManagementClientInit func(ctx context.Context, opts ...option.ClientOption) (keyManagementClient, error)

// Can be overridden for test mocking
var newKeyManagementClient keyManagementClientInit = func(ctx context.Context, opts ...option.ClientOption) (keyManagementClient, error) {
	return kms.NewKeyManagementClient(ctx, opts...)
}

type Config struct {
	Credentials string `hcl:"credentials,optional"`
	AccessToken string `hcl:"access_token,optional"`

	ImpersonateServiceAccount          string   `hcl:"impersonate_service_account,optional"`
	ImpersonateServiceAccountDelegates []string `hcl:"impersonate_service_account_delegates,optional"`

	KMSKeyName string `hcl:"kms_encryption_key"`
	KeyLength  int    `hcl:"key_length"`
}

func stringAttrEnvFallback(val string, env string) string {
	if val != "" {
		return val
	}
	return os.Getenv(env)
}

// TODO This is copied in from the backend packge to prevent a circular dependency loop
// If the argument is a path, ReadPathOrContents loads it and returns the contents,
// otherwise the argument is assumed to be the desired contents and is simply
// returned.
func ReadPathOrContents(poc string) (string, error) {
	if len(poc) == 0 {
		return poc, nil
	}

	path := poc
	if path[0] == '~' {
		var err error
		path, err = homedir.Expand(path)
		if err != nil {
			return path, err
		}
	}

	if _, err := os.Stat(path); err == nil {
		contents, err := os.ReadFile(path)
		if err != nil {
			return string(contents), err
		}
		return string(contents), nil
	}

	return poc, nil
}

func (c Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	// This mirrors the gcp remote state backend

	// Apply env defaults if nessesary
	c.Credentials = stringAttrEnvFallback(c.Credentials, "GOOGLE_CREDENTIALS")
	c.AccessToken = stringAttrEnvFallback(c.AccessToken, "GOOGLE_OAUTH_ACCESS_TOKEN")
	c.ImpersonateServiceAccount = stringAttrEnvFallback(c.ImpersonateServiceAccount, "GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT")
	c.ImpersonateServiceAccount = stringAttrEnvFallback(c.ImpersonateServiceAccount, "GOOGLE_IMPERSONATE_SERVICE_ACCOUNT")

	ctx := context.Background()

	var opts []option.ClientOption
	var credOptions []option.ClientOption

	if c.AccessToken != "" {
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: c.AccessToken,
		})
		credOptions = append(credOptions, option.WithTokenSource(tokenSource))
	} else if c.Credentials != "" {
		// to mirror how the provider works, we accept the file path or the contents
		contents, err := ReadPathOrContents(c.Credentials)
		if err != nil {
			return nil, nil, &keyprovider.ErrInvalidConfiguration{Message: "Error loading credentials", Cause: err}
		}

		if !json.Valid([]byte(contents)) {
			return nil, nil, &keyprovider.ErrInvalidConfiguration{Message: "the string provided in credentials is neither valid json nor a valid file path"}
		}

		credOptions = append(credOptions, option.WithCredentialsJSON([]byte(contents)))
	}

	// Service Account Impersonation
	if c.ImpersonateServiceAccount != "" {
		ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: c.ImpersonateServiceAccount,
			Scopes:          []string{"https://www.googleapis.com/auth/cloudkms"}, // I can't find a smaller scope than this...
			Delegates:       c.ImpersonateServiceAccountDelegates,
		}, credOptions...)

		if err != nil {
			return nil, nil, &keyprovider.ErrInvalidConfiguration{Cause: err}
		}

		opts = append(opts, option.WithTokenSource(ts))

	} else {
		opts = append(opts, credOptions...)
	}

	opts = append(opts, option.WithUserAgent(httpclient.OpenTofuUserAgent(version.Version)))

	svc, err := newKeyManagementClient(ctx, opts...)
	if err != nil {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{Cause: err}
	}

	if c.KMSKeyName == "" {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{Message: "kms_key_name must be provided"}
	}

	if c.KeyLength < 1 {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{Message: "key_length must be at least 1"}
	}
	if c.KeyLength > 1024 {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{Message: "key_length must be less than the GCP limit of 1024"}
	}

	return &keyProvider{
		svc:       svc,
		ctx:       ctx,
		keyName:   c.KMSKeyName,
		keyLength: c.KeyLength,
	}, new(keyMeta), nil
}
