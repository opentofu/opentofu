// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package external

import (
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

// HeaderMagic is the magic string that needs to be present in the header to identify
// the external program as an external keyprovider for OpenTofu.
const HeaderMagic = "OpenTofu-External-Key-Provider"

// Header describes the initial header the external program must output as a single line,
// followed by a single newline.
type Header struct {
	// Magic must always be "OpenTofu-External-Key-Provider".
	Magic string `json:"magic"`
	// Version is the protocol version number. This currently must be 1.
	Version int `json:"version"`
}

// InputV1 describes the input datastructure passed in over stdin.
// This structure is valid for protocol version 1.
type InputV1 *MetadataV1

// OutputV1 describes the output datastructure written to stdout by the external program.
// This structure is valid for protocol version 1.
type OutputV1 struct {
	Keys keyprovider.Output `json:"keys"`
	Meta MetadataV1         `json:"meta,omitempty"`
}
