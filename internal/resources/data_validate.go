// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ValidateConfig asks the provider whether the given value is valid.
//
// The given value should already conform to the schema of the resource type.
func (rt *DataResourceType) ValidateConfig(ctx context.Context, configVal cty.Value) tfdiags.Diagnostics {
	configValUnmarked, _ := configVal.UnmarkDeep()
	resp := rt.client.ValidateDataResourceConfig(ctx, providers.ValidateDataResourceConfigRequest{
		TypeName: rt.typeName,
		Config:   configValUnmarked,
	})
	return resp.Diagnostics
}
