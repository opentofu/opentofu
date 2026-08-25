// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package stackit_kms

import (
	"context"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/version"
	stackitconfig "github.com/stackitcloud/stackit-sdk-go/core/config"
	"github.com/stackitcloud/stackit-sdk-go/services/kms/v1api"
)

type keyManagementClientInit func(projectID, region, keyRingID, keyID string, opts ...stackitconfig.ConfigurationOption) (keyManagementClient, error)

// Can be overridden for test mocking.
var newKeyManagementClient keyManagementClientInit = func(projectID, region, keyRingID, keyID string, opts ...stackitconfig.ConfigurationOption) (keyManagementClient, error) {
	client, err := v1api.NewAPIClient(opts...)
	if err != nil {
		return nil, err
	}
	return &apiClient{
		svc:       client.DefaultAPI,
		projectID: projectID,
		region:    region,
		keyRingID: keyRingID,
		keyID:     keyID,
	}, nil
}

type Config struct {
	// Auth: optional; unset falls back to STACKIT_* env vars and the credentials file.
	ServiceAccountKey     string `hcl:"service_account_key,optional"`
	ServiceAccountKeyPath string `hcl:"service_account_key_path,optional"`
	ServiceAccountToken   string `hcl:"service_account_token,optional"`
	PrivateKey            string `hcl:"private_key,optional"`
	PrivateKeyPath        string `hcl:"private_key_path,optional"`
	Endpoint              string `hcl:"endpoint,optional"`

	ProjectID string `hcl:"project_id"`
	Region    string `hcl:"region"`
	KeyRingID string `hcl:"key_ring_id"`
	KeyID     string `hcl:"key_id"`
	// KeyVersion pins the wrapping version; 0 resolves the latest active version.
	KeyVersion int64 `hcl:"key_version,optional"`

	// KeyLength is the size, in bytes, of the generated data encryption key.
	KeyLength int `hcl:"key_length"`
}

func (c Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	if c.ProjectID == "" {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{Message: "project_id must be provided"}
	}
	if c.Region == "" {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{Message: "region must be provided"}
	}
	if c.KeyRingID == "" {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{Message: "key_ring_id must be provided"}
	}
	if c.KeyID == "" {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{Message: "key_id must be provided"}
	}
	if c.KeyLength < 1 {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{Message: "key_length must be at least 1"}
	}
	if c.KeyLength > 1024 {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{Message: "key_length must be 1024 or less"}
	}

	var opts []stackitconfig.ConfigurationOption
	if c.ServiceAccountKey != "" {
		opts = append(opts, stackitconfig.WithServiceAccountKey(c.ServiceAccountKey))
	}
	if c.ServiceAccountKeyPath != "" {
		opts = append(opts, stackitconfig.WithServiceAccountKeyPath(c.ServiceAccountKeyPath))
	}
	if c.ServiceAccountToken != "" {
		opts = append(opts, stackitconfig.WithToken(c.ServiceAccountToken))
	}
	if c.PrivateKey != "" {
		opts = append(opts, stackitconfig.WithPrivateKey(c.PrivateKey))
	}
	if c.PrivateKeyPath != "" {
		opts = append(opts, stackitconfig.WithPrivateKeyPath(c.PrivateKeyPath))
	}
	if c.Endpoint != "" {
		opts = append(opts, stackitconfig.WithEndpoint(c.Endpoint))
	}
	// No WithRegion: KMS is global-endpoint, so region goes per-call as regionId instead.
	opts = append(opts, stackitconfig.WithUserAgent(httpclient.OpenTofuUserAgent(version.Version)))

	svc, err := newKeyManagementClient(c.ProjectID, c.Region, c.KeyRingID, c.KeyID, opts...)
	if err != nil {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{Message: "failed to create STACKIT KMS client", Cause: err}
	}

	ctx := context.Background()

	resolvedVersion := c.KeyVersion
	if resolvedVersion == 0 {
		resolvedVersion, err = resolveLatestActiveVersion(ctx, svc)
		if err != nil {
			return nil, nil, err
		}
	}

	return &keyProvider{
		svc:       svc,
		ctx:       ctx,
		version:   resolvedVersion,
		keyLength: c.KeyLength,
	}, new(keyMeta), nil
}

// resolveLatestActiveVersion picks the highest-numbered active, non-disabled version.
func resolveLatestActiveVersion(ctx context.Context, svc keyManagementClient) (int64, error) {
	versions, err := svc.ListVersions(ctx)
	if err != nil {
		return 0, &keyprovider.ErrInvalidConfiguration{Message: "failed to list key versions", Cause: err}
	}
	var best int64
	for _, v := range versions {
		if v.GetState() != v1api.VERSIONSTATE_ACTIVE || v.GetDisabled() {
			continue
		}
		if n := v.GetNumber(); n > best {
			best = n
		}
	}
	if best == 0 {
		return 0, &keyprovider.ErrInvalidConfiguration{Message: "no active key version found"}
	}
	return best, nil
}
