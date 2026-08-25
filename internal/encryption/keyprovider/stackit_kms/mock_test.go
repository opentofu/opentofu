// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package stackit_kms

import (
	"context"

	stackitconfig "github.com/stackitcloud/stackit-sdk-go/core/config"
	"github.com/stackitcloud/stackit-sdk-go/services/kms/v1api"
)

// mockKMC implements keyManagementClient directly; see provider.go for why.
type mockKMC struct {
	encrypt      func(version int64, plaintext []byte) ([]byte, error)
	decrypt      func(version int64, ciphertext []byte) ([]byte, error)
	listVersions func() ([]v1api.Version, error)
}

func (m *mockKMC) Encrypt(_ context.Context, version int64, plaintext []byte) ([]byte, error) {
	return m.encrypt(version, plaintext)
}

func (m *mockKMC) Decrypt(_ context.Context, version int64, ciphertext []byte) ([]byte, error) {
	return m.decrypt(version, ciphertext)
}

func (m *mockKMC) ListVersions(_ context.Context) ([]v1api.Version, error) {
	return m.listVersions()
}

func injectMock(m *mockKMC) {
	newKeyManagementClient = func(_, _, _, _ string, _ ...stackitconfig.ConfigurationOption) (keyManagementClient, error) {
		return m, nil
	}
}
