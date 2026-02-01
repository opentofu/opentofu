// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testhelpers

import (
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
)

func ConstructProviderSchemaForTesting(attrs map[string]*configschema.Attribute) providers.Schema {
	return providers.Schema{
		Block: &configschema.Block{Attributes: attrs},
	}
}
