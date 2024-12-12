// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package externalcommand

import (
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

// Input describes the input datastructure passed in over stdin.
type Input *Metadata

// Output describes the output datastructure written to stdout by the external program.
type Output struct {
	Key  keyprovider.Output `json:"key"`
	Meta *Metadata          `json:"meta,omitempty"`
}
