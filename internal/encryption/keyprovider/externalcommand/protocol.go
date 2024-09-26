// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package externalcommand

import (
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type ExternalCommandMeta map[string]any

type ExternalCommandOutput struct {
	Key  keyprovider.Output  `json:"key"`
	Meta ExternalCommandMeta `json:"meta"`
}
