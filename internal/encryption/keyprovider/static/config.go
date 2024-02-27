// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package static

import (
	"encoding/hex"
	"fmt"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type Config struct {
	Key string `hcl:"key"`
}

func (c Config) Build() (keyprovider.KeyProvider, error) {
	decodedData, err := hex.DecodeString(c.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to hex-decode the provided key (%w)", err)
	}
	return &staticKeyProvider{decodedData}, nil
}
