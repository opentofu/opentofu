// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package openbao

import (
	"context"

	openbao "github.com/openbao/openbao/api/v2"
)

type mockClientFunc func(ctx context.Context, path string, data map[string]any) (*openbao.Secret, error)

func (f mockClientFunc) WriteWithContext(ctx context.Context, path string, data map[string]any) (*openbao.Secret, error) {
	return f(ctx, path, data)
}

func injectMock(m mockClientFunc) {
	newClient = func(_ *openbao.Config, _ string) (client, error) {
		return m, nil
	}
}

func injectDefaultClient() {
	newClient = newOpenBaoClient
}
