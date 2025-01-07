// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package xor contains a key provider that combines two other keys.
package xor

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type xorKeyProvider struct {
	key keyprovider.Output
}

func (p xorKeyProvider) Provide(meta keyprovider.KeyMeta) (keyprovider.Output, keyprovider.KeyMeta, error) {
	if meta != nil {
		return keyprovider.Output{}, nil, &keyprovider.ErrInvalidMetadata{
			Message: fmt.Sprintf("bug: metadata provider despite none being required: %T", meta),
		}
	}

	return p.key, nil, nil
}
