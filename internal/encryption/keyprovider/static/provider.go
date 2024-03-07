// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package static contains a key provider that emits a static key.
package static

import "github.com/opentofu/opentofu/internal/encryption/keyprovider"

type staticKeyProvider struct {
	key []byte
}

func (p staticKeyProvider) Provide() (keyprovider.Output, error) {
	return keyprovider.Output{
		EncryptionKey: p.key,
		DecryptionKey: p.key,
	}, nil
}
