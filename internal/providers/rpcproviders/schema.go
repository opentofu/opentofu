// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package rpcproviders

import (
	"context"

	"github.com/opentofu/opentofu/internal/providers"
)

// GetProviderSchema implements providers.Interface.
func (r rpcProvider) GetProviderSchema(ctx context.Context) providers.GetProviderSchemaResponse {
	panic("unimplemented")
}

// GetFunctions implements providers.Interface.
func (r rpcProvider) GetFunctions(ctx context.Context) providers.GetFunctionsResponse {
	panic("unimplemented")
}
