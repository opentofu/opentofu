// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package rpcproviders

import (
	"context"

	"github.com/opentofu/opentofu/internal/providers"
)

// ReadDataSource implements providers.Interface.
func (r rpcProvider) ReadDataSource(ctx context.Context, req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
	panic("unimplemented")
}

// ValidateDataResourceConfig implements providers.Interface.
func (r rpcProvider) ValidateDataResourceConfig(ctx context.Context, req providers.ValidateDataResourceConfigRequest) providers.ValidateDataResourceConfigResponse {
	panic("unimplemented")
}
