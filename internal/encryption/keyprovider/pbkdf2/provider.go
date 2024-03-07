// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package pbkdf2 contains a key provider that takes a passphrase and emits a PBKDF2 hash of the configured length.
package pbkdf2

import (
	"fmt"
	"io"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"

	goPBKDF2 "golang.org/x/crypto/pbkdf2"
)

type pbkdf2KeyProvider struct {
	Config
}

func (p pbkdf2KeyProvider) Provide(rawMeta keyprovider.KeyMeta) (keyprovider.Output, keyprovider.KeyMeta, error) {
	inMeta := rawMeta.(*Metadata)

	// Build outMeta based on current configuration
	outMeta := Metadata{
		Iterations:   p.Iterations,
		HashFunction: p.HashFunction,
		Salt:         make([]byte, p.SaltLength),
	}
	// Generate new salt
	if _, err := io.ReadFull(p.randomSource, outMeta.Salt); err != nil {
		return keyprovider.Output{}, nil, &keyprovider.ErrKeyProviderFailure{
			Message: fmt.Sprintf("failed to obtain %d bytes of random data", p.SaltLength),
			Cause:   err,
		}
	}

	if len(inMeta.Salt) == 0 {
		// No previous metadata
		inMeta.Salt = outMeta.Salt
		inMeta.Iterations = outMeta.Iterations
		inMeta.HashFunction = outMeta.HashFunction
	} else {
		// Make sure previous metadata is supported
		if err := inMeta.HashFunction.Validate(); err != nil {
			return keyprovider.Output{}, nil, err
		}
	}

	return keyprovider.Output{
		goPBKDF2.Key([]byte(p.Passphrase), outMeta.Salt, outMeta.Iterations, p.KeyLength, outMeta.HashFunction.Function()),
		goPBKDF2.Key([]byte(p.Passphrase), inMeta.Salt, inMeta.Iterations, p.KeyLength, inMeta.HashFunction.Function()),
	}, outMeta, nil
}
