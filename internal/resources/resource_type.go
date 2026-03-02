// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ResourceType represents a resource type belonging to a specific provider
// client.
//
// This interface represents the general operations that are relevant to all
// resource types regardless of mode, but most callers will want to use a
// specific implementation of this interface, such as [ManagedResourceType].
type ResourceType interface {
	ResourceMode() addrs.ResourceMode
	ResourceTypeName() string
	LoadSchema(ctx context.Context) (providers.Schema, tfdiags.Diagnostics)
}
