// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package pbkdf2 contains a key provider that takes a passphrase and emits a PBKDF2 hash of the configured length.
package pbkdf2

import (
	"encoding/hex"
	"fmt"
	"hash"
	"io"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"

	goPBKDF2 "golang.org/x/crypto/pbkdf2"
)

type pbkdf2KeyProvider struct {
	randomSource         io.Reader
	passphrase           string
	iterations           int
	hashFunctionName     HashFunctionName
	hashFunctionProvider func() hash.Hash
	saltLength           int
	keyLength            int
}

func (p pbkdf2KeyProvider) Provide(rawMeta keyprovider.KeyMeta) (keyprovider.Output, keyprovider.KeyMeta, error) {
	meta := rawMeta.(*Metadata)

	var decryptSalt []byte
	if len(meta.Salt) > 0 {
		var err error
		decryptSalt, err = hex.DecodeString(meta.Salt)
		if err != nil {
			return keyprovider.Output{}, nil, &keyprovider.ErrInvalidMetadata{
				Message: "failed to hex-decode stored salt, possible data corruption",
				Cause:   err,
			}
		}
	}

	var decryptHashFunction func() hash.Hash
	if meta.HashFunction == "" {
		decryptHashFunction = p.hashFunctionProvider
	} else {
		if err := meta.HashFunction.Validate(); err != nil {
			return keyprovider.Output{}, nil, err
		}
		decryptHashFunction = hashFunctions[meta.HashFunction].functionProvider
	}

	salt := make([]byte, p.saltLength)
	if _, err := io.ReadFull(p.randomSource, salt); err != nil {
		return keyprovider.Output{}, nil, &keyprovider.ErrKeyProviderFailure{
			Message: fmt.Sprintf("failed to obtain %d bytes of random data", p.saltLength),
			Cause:   err,
		}
	}

	if len(decryptSalt) == 0 {
		decryptSalt = salt
	}
	decryptIterations := meta.Iterations
	if decryptIterations == 0 {
		decryptIterations = p.iterations
	}

	return keyprovider.Output{
			goPBKDF2.Key([]byte(p.passphrase), salt, p.iterations, p.keyLength, p.hashFunctionProvider),
			goPBKDF2.Key([]byte(p.passphrase), decryptSalt, decryptIterations, p.keyLength, decryptHashFunction),
		}, Metadata{
			HashFunction: p.hashFunctionName,
			Salt:         fmt.Sprintf("%x", salt),
			Iterations:   p.iterations,
		},
		nil
}
