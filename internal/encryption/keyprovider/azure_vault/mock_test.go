// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package azure_vault

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"github.com/opentofu/opentofu/internal/backend/remote-state/azure/auth"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type mockKMC struct {
	encrypt func(azkeys.KeyOperationParameters) (azkeys.EncryptResponse, error)
	decrypt func(azkeys.KeyOperationParameters) (azkeys.DecryptResponse, error)
}

func (m *mockKMC) Decrypt(_ context.Context, _ string, _ string, parameters azkeys.KeyOperationParameters, _ *azkeys.DecryptOptions) (azkeys.DecryptResponse, error) {
	return m.decrypt(parameters)
}

func (m *mockKMC) Encrypt(_ context.Context, _ string, _ string, parameters azkeys.KeyOperationParameters, _ *azkeys.EncryptOptions) (azkeys.EncryptResponse, error) {
	return m.encrypt(parameters)
}

func (m *mockKMC) Construct(_ context.Context, _ *auth.Config) (azcore.TokenCredential, error) {
	return nil, nil
}

func (m *mockKMC) Validate(_ context.Context, _ *auth.Config) tfdiags.Diagnostics {
	return nil
}

func (m *mockKMC) AugmentConfig(_ context.Context, _ *auth.Config) error {
	return nil
}

func (m *mockKMC) Name() string {
	return "Mock key manager"
}

func injectMock(m *mockKMC) {
	newKeyManagementClient = func(_ string, _ azcore.TokenCredential) (keyManagementClient, error) {
		return m, nil
	}
	getAuthMethod = func(_ context.Context, _ *auth.Config) (auth.AuthMethod, error) {
		return m, nil
	}
}
