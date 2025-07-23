// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package rpcproviders

import (
	"context"

	"github.com/opentofu/opentofu/internal/providers"
)

// CallFunction implements providers.Interface.
func (r rpcProvider) CallFunction(ctx context.Context, req providers.CallFunctionRequest) providers.CallFunctionResponse {
	panic("unimplemented")
}
