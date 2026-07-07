// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"iter"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type Provisioner struct {
	// Fields from configuration

	Type      string
	When      configs.ProvisionerWhen
	OnFailure configs.ProvisionerOnFailure

	// Config is the current instance value specific configuration function
	Config func(context.Context, cty.Value) (ProvisionerConfig, tfdiags.Diagnostics)

	// Dependencies of the configuration (excluding the self value)
	Dependencies iter.Seq[*ResourceInstance]
}

type ProvisionerConfig struct {
	Value      cty.Value
	Connection cty.Value
}
