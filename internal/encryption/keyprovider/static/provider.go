// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package static contains a key provider that emits a static key.
package static

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type staticKeyProvider struct {
	key []byte
}

const magic = "Hello world!"

func (p staticKeyProvider) Provide(meta keyprovider.KeyMeta) (keyprovider.Output, keyprovider.KeyMeta, error) {
	// Note: this is a demonstration how you can handle metadata. Using a magic string does not make any sense,
	// but it illustrates well how you can store and retrieve metadata. We wish we could use generics to
	// save you the trouble of doing a type assertion, but Go does not have sufficiently advanced enough generics
	// to do that.
	typedMeta, ok := meta.(*Metadata)
	if !ok {
		return keyprovider.Output{}, nil, fmt.Errorf("bug: invalid metadata type received: %T", meta)
	}
	// Note: the Magic may be empty if OpenTofu isn't decrypting anything, make sure to account for that possibility.
	if typedMeta.Magic != "" && typedMeta.Magic != magic {
		return keyprovider.Output{}, nil, fmt.Errorf("corrupted data received, no or invalid magic string: %s", typedMeta.Magic)
	}

	return keyprovider.Output{
		EncryptionKey: p.key,
		DecryptionKey: p.key,
	}, &Metadata{Magic: magic}, nil
}
