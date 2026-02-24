// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package keyprovider

import (
	"github.com/hashicorp/hcl/v2"
)

// Config is a struct annotated with HCL (and preferably JSON) tags that OpenTofu reads the user-provided configuration
// into. The Build function assembles the configuration into a usable key provider.
type Config interface {
	// Build provides a key provider and an empty JSON-tagged struct to read the decryption metadata into. If the
	// configuration is invalid, it returns an error.
	//
	// If a key provider does not need metadata, it may return nil.
	Build() (KeyProvider, KeyMeta, error)
}

// SelfDecodingConfig can be implemented by the [Config] types that has special rules for decoding
// references to other `encryption` contained blocks.
type SelfDecodingConfig interface {
	DecodeConfig(body hcl.Body, evalCtx *hcl.EvalContext) (diags hcl.Diagnostics)
	ConfigSchema() *hcl.BodySchema
}
