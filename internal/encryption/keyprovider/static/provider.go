// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package static contains a key provider that emits a static key.
package static

type staticKeyProvider struct {
	key []byte
}

func (p staticKeyProvider) Provide(metadata []byte) ([]byte, []byte, error) {
	if metadata == nil {
		metadata = []byte("magic")
	}

	return p.key, metadata, nil
}
