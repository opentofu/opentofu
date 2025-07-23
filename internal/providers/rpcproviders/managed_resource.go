// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package rpcproviders

import (
	"context"

	"github.com/opentofu/opentofu/internal/providers"
)

// ApplyResourceChange implements providers.Interface.
func (r rpcProvider) ApplyResourceChange(ctx context.Context, req providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
	panic("unimplemented")
}

// ImportResourceState implements providers.Interface.
func (r rpcProvider) ImportResourceState(ctx context.Context, req providers.ImportResourceStateRequest) providers.ImportResourceStateResponse {
	panic("unimplemented")
}

// MoveResourceState implements providers.Interface.
func (r rpcProvider) MoveResourceState(ctx context.Context, req providers.MoveResourceStateRequest) providers.MoveResourceStateResponse {
	panic("unimplemented")
}

// PlanResourceChange implements providers.Interface.
func (r rpcProvider) PlanResourceChange(ctx context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
	panic("unimplemented")
}

// ReadResource implements providers.Interface.
func (r rpcProvider) ReadResource(ctx context.Context, req providers.ReadResourceRequest) providers.ReadResourceResponse {
	panic("unimplemented")
}

// UpgradeResourceState implements providers.Interface.
func (r rpcProvider) UpgradeResourceState(ctx context.Context, req providers.UpgradeResourceStateRequest) providers.UpgradeResourceStateResponse {
	panic("unimplemented")
}

// ValidateResourceConfig implements providers.Interface.
func (r rpcProvider) ValidateResourceConfig(crx context.Context, req providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
	panic("unimplemented")
}
