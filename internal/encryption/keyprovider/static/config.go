// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package static

import (
	"encoding/hex"

	"github.com/terramate-io/opentofulib/internal/encryption/keyprovider"
)

// Config contains the configuration for this key provider supplied by the user. This struct must have hcl tags in order
// to function.
type Config struct {
	Key string `hcl:"key"`
}

// Build will create the usable key provider.
func (c Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	if c.Key == "" {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: "Missing key",
		}
	}

	decodedData, err := hex.DecodeString(c.Key)
	if err != nil {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: "failed to hex-decode the provided key",
			Cause:   err,
		}
	}

	return &staticKeyProvider{decodedData}, new(Metadata), nil
}
