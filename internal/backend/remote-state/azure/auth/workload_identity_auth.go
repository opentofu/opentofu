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

type WorkloadIdentityAuthConfig struct {
	UseAKSWorkloadIdentity bool
}

type workloadIdentityAuth struct{}

var _ AuthMethod = &workloadIdentityAuth{}

func (cred *workloadIdentityAuth) Name() string {
	return "AKS Workload Identity Auth"
}

func (cred *workloadIdentityAuth) Construct(ctx context.Context, config *Config) (azcore.TokenCredential, error) {
	client := httpclient.New(ctx)
	return azidentity.NewWorkloadIdentityCredential(
		&azidentity.WorkloadIdentityCredentialOptions{
			ClientOptions: clientOptions(client, config.CloudConfig),
		},
	)
}

func (cred *workloadIdentityAuth) Validate(_ context.Context, config *Config) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if !config.UseAKSWorkloadIdentity {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid AKS Workload Identity Auth",
			"The AKS Workload Identity Auth needs to have \"use_aks_workload_identity\" (or ARM_USE_AKS_WORKLOAD_IDENTITY) set to true in order to be used.",
		))
	}
	return diags
}

func (cred *workloadIdentityAuth) AugmentConfig(_ context.Context, config *Config) error {
	return checkNamesForAccessKeyCredentials(config.StorageAddresses)
}
